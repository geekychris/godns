package server

import (
	"fmt"
	"net"
	"testing"
	"time"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/godns/internal/config"
)

// getTestAddr returns a free port for testing
func getTestAddr() (string, int) {
	addr := "127.0.0.1"
	listener, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP(addr),
		Port: 0,
	})
	if err != nil {
		return addr, 10053 // fallback to default test port
	}
	port := listener.LocalAddr().(*net.UDPAddr).Port
	listener.Close()
	return addr, port
}

// waitForServer waits for the server to start
func waitForServer(addr string) error {
	deadline := time.Now().Add(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			conn, err := net.DialTimeout("udp", addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				// Add extra delay to ensure the server is fully ready
				time.Sleep(200 * time.Millisecond)
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("server failed to start within timeout")
			}
		}
	}
}

func TestCompleteSystemFlow(t *testing.T) {
	// Create test configuration
	addr, port := getTestAddr()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address:  addr,
			Port:     port,
			CacheTTL: time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
		},
	}

	// Create and start server
	server := NewDNSServer(cfg, createTestLogger())

	// Create test resolver
	resolver := &CustomRoundRobinResolver{
		hostname: "test.example.com",
		ips: []net.IP{
			net.ParseIP("192.168.1.1"),
			net.ParseIP("192.168.1.2"),
		},
	}
	server.RegisterResolver(resolver)

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// Wait for server to start
	serverAddr := fmt.Sprintf("%s:%d", addr, port)
	err := waitForServer(serverAddr)
	require.NoError(t, err, "Server failed to start")
	// Test DNS queries
	testCases := []struct {
		name     string
		hostname string
		qtype    uint16
		wantCode int
		wantIP   string
	}{
		{
			name:     "Valid A record query",
			hostname: "test.example.com",
			qtype:    dns.TypeA,
			wantCode: dns.RcodeSuccess,
			wantIP:   "192.168.1.1",
		},
		{
			name:     "Unknown hostname",
			hostname: "unknown.example.com",
			qtype:    dns.TypeA,
			wantCode: dns.RcodeNameError,
		},
		{
			name:     "Unsupported record type",
			hostname: "test.example.com",
			qtype:    dns.TypeAAAA,
			wantCode: dns.RcodeSuccess,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(tc.hostname), tc.qtype)

			// Send query
			c := new(dns.Client)
			r, _, err := c.Exchange(m, server.Addr())
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, r.Rcode)

			if tc.wantIP != "" {
				require.Len(t, r.Answer, 1)
				a, ok := r.Answer[0].(*dns.A)
				require.True(t, ok)
				assert.Equal(t, tc.wantIP, a.A.String())
			}
		})
	}

	// Clean up
	server.Stop()
}

// TestCacheAndRoundRobin tests both caching and round-robin behavior
func TestCacheAndRoundRobin(t *testing.T) {
	// Create test configuration with short cache TTL
	addr, port := getTestAddr()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address:  addr,
			Port:     port,
			CacheTTL: 500 * time.Millisecond, // Short TTL for testing
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
		},
	}

	// Create server and resolver
	server := NewDNSServer(cfg, createTestLogger())
	resolver := &CustomRoundRobinResolver{
		hostname: "roundrobin.example.com",
		ips: []net.IP{
			net.ParseIP("192.168.1.10"),
			net.ParseIP("192.168.1.20"),
			net.ParseIP("192.168.1.30"),
		},
	}
	server.RegisterResolver(resolver)

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			errChan <- err
		}
	}()

	// Setup server shutdown to happen when test completes
	defer func() {
		server.Stop()
		// Wait for any error from server.Start()
		select {
		case err := <-errChan:
			require.NoError(t, err, "Server error")
		case <-time.After(2 * time.Second):
			// Timeout is okay - means server was stopped normally
		}
	}()

	// Wait for server to start
	serverAddr := fmt.Sprintf("%s:%d", addr, port)
	err := waitForServer(serverAddr)
	require.NoError(t, err, "Server failed to start")
	
	// Add an additional delay to ensure the server is fully initialized
	time.Sleep(500 * time.Millisecond)
	// Create DNS client
	c := new(dns.Client)
	hostname := "roundrobin.example.com"
	
	// First query - should get first IP and cache it
	m1 := new(dns.Msg)
	m1.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	r1, _, err := c.Exchange(m1, server.Addr())
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, r1.Rcode)
	require.Len(t, r1.Answer, 1)
	a1, ok := r1.Answer[0].(*dns.A)
	require.True(t, ok)
	firstIP := a1.A.String()
	require.Contains(t, []string{"192.168.1.10", "192.168.1.20", "192.168.1.30"}, firstIP)
	
	// Second query - should get cached result (same IP)
	m2 := new(dns.Msg)
	m2.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	r2, _, err := c.Exchange(m2, server.Addr())
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, r2.Rcode)
	require.Len(t, r2.Answer, 1)
	a2, ok := r2.Answer[0].(*dns.A)
	require.True(t, ok)
	assert.Equal(t, firstIP, a2.A.String(), "Cached result should match first query")
	
	// Wait for cache to expire
	time.Sleep(cfg.Server.CacheTTL + 100*time.Millisecond)
	
	// Third query - should get next IP in round-robin and cache it
	m3 := new(dns.Msg)
	m3.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	r3, _, err := c.Exchange(m3, server.Addr())
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, r3.Rcode)
	require.Len(t, r3.Answer, 1)
	a3, ok := r3.Answer[0].(*dns.A)
	require.True(t, ok)
	secondIP := a3.A.String()
	require.Contains(t, []string{"192.168.1.10", "192.168.1.20", "192.168.1.30"}, secondIP)
	require.NotEqual(t, firstIP, secondIP, "Round-robin should return different IP")
	
	// Fourth query - should get cached result (same as third query)
	m4 := new(dns.Msg)
	m4.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	r4, _, err := c.Exchange(m4, server.Addr())
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, r4.Rcode)
	require.Len(t, r4.Answer, 1)
	a4, ok := r4.Answer[0].(*dns.A)
	require.True(t, ok)
	assert.Equal(t, secondIP, a4.A.String(), "Cached result should match third query")
	
	// Wait for cache to expire again
	time.Sleep(cfg.Server.CacheTTL + 100*time.Millisecond)
	
	// Fifth query - should get third IP and cache it
	m5 := new(dns.Msg)
	m5.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	r5, _, err := c.Exchange(m5, server.Addr())
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, r5.Rcode)
	require.Len(t, r5.Answer, 1)
	a5, ok := r5.Answer[0].(*dns.A)
	require.True(t, ok)
	thirdIP := a5.A.String()
	require.Contains(t, []string{"192.168.1.10", "192.168.1.20", "192.168.1.30"}, thirdIP)
	require.NotEqual(t, firstIP, thirdIP, "Third IP should be different from first")
	require.NotEqual(t, secondIP, thirdIP, "Third IP should be different from second")
	
	// Wait for cache to expire once more
	time.Sleep(cfg.Server.CacheTTL + 100*time.Millisecond)
	
	// Sixth query - should cycle back to first IP
	m6 := new(dns.Msg)
	m6.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	r6, _, err := c.Exchange(m6, server.Addr())
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, r6.Rcode)
	require.Len(t, r6.Answer, 1)
	a6, ok := r6.Answer[0].(*dns.A)
	require.True(t, ok)
	fourthIP := a6.A.String()
	require.Contains(t, []string{"192.168.1.10", "192.168.1.20", "192.168.1.30"}, fourthIP)
	// With 3 IPs, on the fourth request we should get back to the first IP
	assert.Equal(t, firstIP, fourthIP, "Should cycle back to first IP")

	// Clean up happens via defer
}

// TestConcurrentQueries tests the server's behavior under concurrent load
func TestConcurrentQueries(t *testing.T) {
	// Create test configuration
	addr, port := getTestAddr()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address:  addr,
			Port:     port,
			CacheTTL: time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
		},
	}

	// Create and start server
	server := NewDNSServer(cfg, createTestLogger())

	// Create test resolver with multiple hostnames
	resolver := &CustomMultiHostResolver{
		entries: map[string][]net.IP{
			"host1.example.com": {net.ParseIP("192.168.1.1")},
			"host2.example.com": {net.ParseIP("192.168.1.2")},
			"host3.example.com": {net.ParseIP("192.168.1.3")},
		},
	}
	server.RegisterResolver(resolver)

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// Wait for server to start
	serverAddr := fmt.Sprintf("%s:%d", addr, port)
	err := waitForServer(serverAddr)
	require.NoError(t, err, "Server failed to start")
	// Run concurrent queries
	const numConcurrent = 10
	const numQueries = 3
	
	hostnames := []string{
		"host1.example.com",
		"host2.example.com",
		"host3.example.com",
	}
	
	expectedIPs := map[string]string{
		"host1.example.com": "192.168.1.1",
		"host2.example.com": "192.168.1.2",
		"host3.example.com": "192.168.1.3",
	}
	
	results := make(chan error, numConcurrent*numQueries)
	
	for i := 0; i < numConcurrent; i++ {
		go func(id int) {
			c := new(dns.Client)
			
			for j := 0; j < numQueries; j++ {
				hostname := hostnames[j%len(hostnames)]
				
				m := new(dns.Msg)
				m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
				
				r, _, err := c.Exchange(m, server.Addr())
				if err != nil {
					results <- err
					continue
				}
				
				if r.Rcode != dns.RcodeSuccess {
					results <- fmt.Errorf("expected success, got code %d", r.Rcode)
					continue
				}
				
				if len(r.Answer) != 1 {
					results <- fmt.Errorf("expected 1 answer, got %d", len(r.Answer))
					continue
				}
				
				a, ok := r.Answer[0].(*dns.A)
				if !ok {
					results <- fmt.Errorf("expected A record, got %T", r.Answer[0])
					continue
				}
				
				if a.A.String() != expectedIPs[hostname] {
					results <- fmt.Errorf("expected IP %s, got %s", expectedIPs[hostname], a.A.String())
					continue
				}
				
				// Query succeeded
				results <- nil
			}
		}(i)
	}
	
	// Collect and check results
	var failures int
	for i := 0; i < numConcurrent*numQueries; i++ {
		if err := <-results; err != nil {
			failures++
			t.Logf("Query failed: %v", err)
		}
	}
	
	assert.Zero(t, failures, "Expected no failures in concurrent queries")

	// Clean up
	server.Stop()
}

// CustomMultiHostResolver is a simple resolver for testing multiple hostnames
type CustomMultiHostResolver struct {
	entries map[string][]net.IP
}

func (r *CustomMultiHostResolver) CanResolve(hostname string) bool {
	_, ok := r.entries[hostname]
	return ok
}

func (r *CustomMultiHostResolver) Resolve(hostname string) ([]net.IP, error) {
	ips, ok := r.entries[hostname]
	if !ok {
		return nil, fmt.Errorf("hostname %s not found", hostname)
	}
	return ips, nil
}

func (r *CustomMultiHostResolver) Name() string {
	return "custom-multi-host"
}

func (r *CustomMultiHostResolver) Init(config map[string]interface{}) error {
	// No initialization needed for test resolver
	return nil
}

// This duplicate method is removed
// customNameResolver is a wrapper that provides a custom name
type customNameResolver struct {
	*CustomMultiHostResolver
	customName string
}

func (r *customNameResolver) Name() string {
	return r.customName
}

func (r *customNameResolver) Init(config map[string]interface{}) error {
	return r.CustomMultiHostResolver.Init(config)
}

// TestMultipleResolvers tests the server's ability to use multiple resolvers
func TestMultipleResolvers(t *testing.T) {
	// Create test configuration
	addr, port := getTestAddr()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address:  addr,
			Port:     port,
			CacheTTL: time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
		},
	}

	// Create and start server
	server := NewDNSServer(cfg, createTestLogger())

	// Create first resolver
	resolver1 := &CustomMultiHostResolver{
		entries: map[string][]net.IP{
			"host1.example.com": {net.ParseIP("192.168.1.1")},
		},
	}
	server.RegisterResolver(resolver1)
	// Create second resolver with a unique name
	resolver2 := &CustomMultiHostResolver{
		entries: map[string][]net.IP{
			"host2.example.com": {net.ParseIP("192.168.2.1")},
		},
	}
	
	// Give it a custom name to avoid duplicate registration
	customResolver2 := &customNameResolver{
		CustomMultiHostResolver: resolver2,
		customName: "custom-multi-host-2",
	}
	server.RegisterResolver(customResolver2)
	server.RegisterResolver(customResolver2)
	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// Wait for server to start
	serverAddr := fmt.Sprintf("%s:%d", addr, port)
	err := waitForServer(serverAddr)
	require.NoError(t, err, "Server failed to start")
	// Test cases for different resolvers
	testCases := []struct {
		name     string
		hostname string
		wantIP   string
		wantCode int
	}{
		{
			name:     "First resolver",
			hostname: "host1.example.com",
			wantIP:   "192.168.1.1",
			wantCode: dns.RcodeSuccess,
		},
		{
			name:     "Second resolver",
			hostname: "host2.example.com",
			wantIP:   "192.168.2.1",
			wantCode: dns.RcodeSuccess,
		},
		{
			name:     "Unknown hostname",
			hostname: "unknown.example.com",
			wantCode: dns.RcodeNameError,
		},
	}

	// Create DNS client
	c := new(dns.Client)

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(tc.hostname), dns.TypeA)

			r, _, err := c.Exchange(m, server.Addr())
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, r.Rcode)

			if tc.wantIP != "" {
				require.Len(t, r.Answer, 1)
				a, ok := r.Answer[0].(*dns.A)
				require.True(t, ok)
				assert.Equal(t, tc.wantIP, a.A.String())
			}
		})
	}

	// Clean up
	server.Stop()
}
