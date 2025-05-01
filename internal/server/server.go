package server

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"

	"github.com/yourusername/godns/internal/config"
	"github.com/yourusername/godns/internal/rest"
	"github.com/yourusername/godns/pkg/api"
)

// DNSServer represents the DNS server
type DNSServer struct {
	config       *config.Config
	server       *dns.Server
	cache        map[string]*cacheEntry
	cacheMutex   sync.RWMutex
	resolvers    []api.Resolver
	resolverLock sync.RWMutex
	logger       *logrus.Logger
	metrics      *metrics
	restServer   rest.Server
}

// cacheEntry represents a cached DNS response
type cacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
}

// metrics tracks server statistics
type metrics struct {
	totalQueries      uint64
	cacheHits         uint64
	pluginResolutions uint64
	forwardedQueries  uint64
	failedQueries     uint64
	mutex             sync.Mutex
}

// NewDNSServer creates a new DNS server instance
func NewDNSServer(cfg *config.Config, logger *logrus.Logger) *DNSServer {
	return &DNSServer{
		config:    cfg,
		cache:     make(map[string]*cacheEntry),
		resolvers: make([]api.Resolver, 0),
		logger:    logger,
		metrics:   &metrics{},
	}
}

// RegisterResolver registers a plugin resolver with the server
func (s *DNSServer) RegisterResolver(resolver api.Resolver) {
	s.resolverLock.Lock()
	defer s.resolverLock.Unlock()

	// Check if the resolver is already registered
	for _, r := range s.resolvers {
		if r.Name() == resolver.Name() {
			s.logger.Warnf("Resolver with name %s already registered", resolver.Name())
			return
		}
	}

	s.resolvers = append(s.resolvers, resolver)
	s.logger.Infof("Registered resolver: %s", resolver.Name())
}


// SetRESTServer sets the REST API server
func (s *DNSServer) SetRESTServer(server rest.Server) {
	s.restServer = server
	s.logger.Debug("REST server connected to DNS server")
}

// Start starts the DNS server
func (s *DNSServer) Start() error {
	// Set up the DNS server
	addr := fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)
	s.server = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: dns.HandlerFunc(s.handleDNSRequest),
	}

	s.logger.Infof("Starting DNS server on %s (UDP)", addr)
	
	// Start periodic cache cleanup
	go s.periodicCacheCleanup()
	
	// Start the DNS server (this will block until the server is stopped)
	return s.server.ListenAndServe()
}

// Stop stops the DNS server
func (s *DNSServer) Stop() error {
	var dnsErr, restErr error

	// Stop DNS server
	if s.server != nil {
		s.logger.Info("Stopping DNS server")
		dnsErr = s.server.Shutdown()
	}

	// Stop REST API server if set
	if s.restServer != nil {
		s.logger.Info("Stopping REST API server")
		restErr = s.restServer.Stop()
	}

	// Return the first error encountered
	if dnsErr != nil {
		return dnsErr
	}
	return restErr
}

// handleDNSRequest handles incoming DNS requests
func (s *DNSServer) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	s.metrics.mutex.Lock()
	s.metrics.totalQueries++
	s.metrics.mutex.Unlock()

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		s.logger.Debugf("Received query for %s (type: %d)", q.Name, q.Qtype)

		// Only handle A record queries for now
		if q.Qtype == dns.TypeA {
			hostname := q.Name
			// Remove trailing dot from hostname if present
			if len(hostname) > 0 && hostname[len(hostname)-1] == '.' {
				hostname = hostname[:len(hostname)-1]
			}
			
			hostname = strings.ToLower(hostname)

			// Try to resolve using cache first
			if ips := s.resolveFromCache(hostname); ips != nil {
				s.logger.Debugf("Cache hit for %s", hostname)
				for _, ip := range ips {
					rr := &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    uint32(s.config.Server.CacheTTL.Seconds()),
						},
						A: ip,
					}
					m.Answer = append(m.Answer, rr)
				}
				
				s.metrics.mutex.Lock()
				s.metrics.cacheHits++
				s.metrics.mutex.Unlock()
				
				continue
			}

			// Try to resolve using plugins
			resolved := false
			ips, err := s.resolveWithPlugins(hostname)
			if err == nil && len(ips) > 0 {
				s.logger.Debugf("Resolved %s using plugins: %v", hostname, ips)
				
				// Cache the result
				s.cacheResult(hostname, ips)
				
				for _, ip := range ips {
					rr := &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    uint32(s.config.Server.CacheTTL.Seconds()),
						},
						A: ip,
					}
					m.Answer = append(m.Answer, rr)
				}
				
				s.metrics.mutex.Lock()
				s.metrics.pluginResolutions++
				s.metrics.mutex.Unlock()
				
				resolved = true
			}

			// Fall back to upstream DNS server if plugins couldn't resolve
			if !resolved {
				ips, err := s.forwardQuery(hostname)
				if err == nil && len(ips) > 0 {
					s.logger.Debugf("Resolved %s using upstream: %v", hostname, ips)
					
					// Cache the result
					s.cacheResult(hostname, ips)
					
					for _, ip := range ips {
						rr := &dns.A{
							Hdr: dns.RR_Header{
								Name:   q.Name,
								Rrtype: dns.TypeA,
								Class:  dns.ClassINET,
								Ttl:    uint32(s.config.Server.CacheTTL.Seconds()),
							},
							A: ip,
						}
						m.Answer = append(m.Answer, rr)
					}
					
					s.metrics.mutex.Lock()
					s.metrics.forwardedQueries++
					s.metrics.mutex.Unlock()
					
					resolved = true
				}
			}

			if !resolved {
				s.logger.Warnf("Failed to resolve %s", hostname)
				
				// Set NXDOMAIN (Name Error) response code for unresolvable hostnames
				m.SetRcode(r, dns.RcodeNameError)
				
				s.metrics.mutex.Lock()
				s.metrics.failedQueries++
				s.metrics.mutex.Unlock()
			}
		} else {
			// For non-A record types, forward directly to upstream
			response, err := s.forwardQueryRaw(r)
			if err == nil {
				// Replace our empty reply with the forwarded response
				*m = *response
				break  // We've replaced the entire message, so we're done
			} else {
				s.logger.Warnf("Failed to forward non-A query: %v", err)
				
				// For non-A records, the response should be NOERROR even if we don't have records
				// This indicates the domain exists but we don't have that record type
				m.SetRcode(r, dns.RcodeSuccess)
			}
		}
	}

	if err := w.WriteMsg(m); err != nil {
		s.logger.Errorf("Error writing DNS response: %v", err)
	}
}

// resolveFromCache tries to resolve a hostname from the cache
func (s *DNSServer) resolveFromCache(hostname string) []net.IP {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	entry, found := s.cache[hostname]
	if !found {
		return nil
	}

	// Check if cache entry has expired
	if time.Now().After(entry.expiresAt) {
		s.logger.Debugf("Cache entry for %s has expired", hostname)
		return nil
	}

	return entry.ips
}

// cacheResult caches a DNS resolution result
func (s *DNSServer) cacheResult(hostname string, ips []net.IP) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	ttl := s.config.Server.CacheTTL
	s.cache[hostname] = &cacheEntry{
		ips:       ips,
		expiresAt: time.Now().Add(ttl),
	}
	s.logger.Debugf("Cached %s with TTL %v", hostname, ttl)
}

// resolveWithPlugins tries to resolve a hostname using registered plugins
func (s *DNSServer) resolveWithPlugins(hostname string) ([]net.IP, error) {
	s.resolverLock.RLock()
	defer s.resolverLock.RUnlock()

	for _, resolver := range s.resolvers {
		if resolver.CanResolve(hostname) {
			s.logger.Debugf("Resolver %s can resolve %s", resolver.Name(), hostname)
			return resolver.Resolve(hostname)
		}
	}

	return nil, fmt.Errorf("no plugin can resolve %s", hostname)
}

// forwardQuery forwards a query to upstream DNS servers
func (s *DNSServer) forwardQuery(hostname string) ([]net.IP, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	m.RecursionDesired = true

	var ips []net.IP
	var lastErr error

	for _, server := range s.config.Upstream.Servers {
		c := &dns.Client{
			Timeout: s.config.Upstream.Timeout * time.Second,
		}
		r, _, err := c.Exchange(m, server)
		if err != nil {
			s.logger.Warnf("Error querying upstream DNS %s: %v", server, err)
			lastErr = err
			continue
		}

		if r.Rcode != dns.RcodeSuccess {
			s.logger.Warnf("Upstream DNS %s returned non-success code: %d", server, r.Rcode)
			continue
		}

		for _, answer := range r.Answer {
			if a, ok := answer.(*dns.A); ok {
				ips = append(ips, a.A)
			}
		}

		if len(ips) > 0 {
			return ips, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no A records found for %s", hostname)
}

// forwardQueryRaw forwards a raw DNS query to upstream DNS servers
func (s *DNSServer) forwardQueryRaw(r *dns.Msg) (*dns.Msg, error) {
	var lastErr error

	for _, server := range s.config.Upstream.Servers {
		c := &dns.Client{
			Timeout: s.config.Upstream.Timeout * time.Second,
		}
		resp, _, err := c.Exchange(r, server)
		if err != nil {
			s.logger.Warnf("Error forwarding query to upstream DNS %s: %v", server, err)
			lastErr = err
			continue
		}

		if resp.Rcode != dns.RcodeSuccess {
			s.logger.Warnf("Upstream DNS %s returned non-success code: %d", server, resp.Rcode)
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no response from any upstream DNS server")
}

// periodicCacheCleanup periodically removes expired entries from the cache
func (s *DNSServer) periodicCacheCleanup() {
	// Use half the cache TTL as the cleanup interval, but at least every 60 seconds
	interval := s.config.Server.CacheTTL / 2
	if interval < 60*time.Second {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Infof("Starting cache cleanup routine with interval %v", interval)

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredCache()
		}
	}
}

// cleanupExpiredCache removes expired entries from the cache
func (s *DNSServer) cleanupExpiredCache() {
	now := time.Now()
	expiredCount := 0

	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	for hostname, entry := range s.cache {
		if now.After(entry.expiresAt) {
			delete(s.cache, hostname)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		s.logger.Debugf("Cleaned up %d expired cache entries", expiredCount)
	}
}

// Addr returns the server's address in the format "ip:port"
func (s *DNSServer) Addr() string {
	if s.server == nil {
		return fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)
	}
	// Return the actual server address if it's running
	if s.server.PacketConn != nil {
		return s.server.PacketConn.LocalAddr().String()
	}
	return s.server.Addr
}
