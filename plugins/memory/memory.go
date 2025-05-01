package memory

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"

	"github.com/yourusername/godns/pkg/api"
)

// MemoryPlugin implements the Resolver interface for in-memory DNS resolution
type MemoryPlugin struct {
	entries      map[string][]net.IP
	entriesMutex sync.RWMutex
	counters     map[string]*uint64
	logger       *logrus.Logger
}

// NewMemoryPlugin creates a new in-memory resolver
func NewMemoryPlugin(config map[string]interface{}) (api.Resolver, error) {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	})

	mp := &MemoryPlugin{
		entries:  make(map[string][]net.IP),
		counters: make(map[string]*uint64),
		logger:   logger,
	}
	// Load default entries if provided
	if entriesConfig, exists := config["entries"]; exists {
		entries, ok := entriesConfig.([]map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("entries must be an array of maps")
		}
		for _, entry := range entries {
			hostname, ok := entry["hostname"].(string)
			if !ok || hostname == "" {
				continue
			}

			ipStrings, ok := entry["ips"].([]string)
			if !ok {
				// Try to handle as []interface{}
				if ipInterfaces, ok := entry["ips"].([]interface{}); ok {
					ipStrings = make([]string, len(ipInterfaces))
					for i, ipInterface := range ipInterfaces {
						if ipString, ok := ipInterface.(string); ok {
							ipStrings[i] = ipString
						}
					}
				} else {
					continue
				}
			}

			var ips []net.IP
			for _, ipString := range ipStrings {
				ip := net.ParseIP(ipString)
				if ip == nil {
					mp.logger.Warnf("Invalid IP address: %s for hostname: %s", ipString, hostname)
					continue
				}
				ips = append(ips, ip)
			}

			if len(ips) > 0 {
				if err := mp.AddEntry(hostname, ips); err != nil {
					return nil, err
				}
			}
		}
	}

	return mp, nil
}

// Name returns the plugin name
func (mp *MemoryPlugin) Name() string {
	return "memory"
}

// Init initializes the plugin with the given configuration
func (mp *MemoryPlugin) Init(config map[string]interface{}) error {
	// No-op as initialization is done in constructor
	return nil
}

// CanResolve determines if this plugin can resolve the given hostname
func (mp *MemoryPlugin) CanResolve(hostname string) bool {
	mp.entriesMutex.RLock()
	defer mp.entriesMutex.RUnlock()

	hostname = strings.ToLower(hostname)
	_, exists := mp.entries[hostname]
	return exists
}

// Resolve returns IP addresses for the given hostname
func (mp *MemoryPlugin) Resolve(hostname string) ([]net.IP, error) {
	mp.entriesMutex.RLock()
	defer mp.entriesMutex.RUnlock()

	hostname = strings.ToLower(hostname)
	ips, exists := mp.entries[hostname]
	if !exists {
		return nil, fmt.Errorf("hostname %s not found in memory plugin", hostname)
	}

	// If only one IP is defined, return it directly
	if len(ips) == 1 {
		return ips, nil
	}

	// Otherwise, use round-robin to balance between IPs
	counter, exists := mp.counters[hostname]
	if !exists {
		var c uint64 = 0
		counter = &c
		mp.counters[hostname] = counter
	}

	// Atomically increment the counter and get the next IP
	idx := int(atomic.AddUint64(counter, 1) - 1) % len(ips)
	mp.logger.Debugf("Round-robin for %s: selected IP %s (index %d of %d)", 
		hostname, ips[idx].String(), idx, len(ips))
	return []net.IP{ips[idx]}, nil
}

// AddEntry adds or updates a hostname entry
func (mp *MemoryPlugin) AddEntry(hostname string, ips []net.IP) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	
	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses provided for hostname %s", hostname)
	}

	// Validate each IP
	for _, ip := range ips {
		if ip == nil {
			return fmt.Errorf("invalid IP address (nil) provided for hostname %s", hostname)
		}
	}

	hostname = strings.ToLower(hostname)

	mp.entriesMutex.Lock()
	defer mp.entriesMutex.Unlock()

	mp.entries[hostname] = ips
	// Reset counter when entry is updated
	var c uint64 = 0
	mp.counters[hostname] = &c

	mp.logger.Infof("Added entry for hostname %s with %d IPs", hostname, len(ips))
	return nil
}

// GetEntry retrieves the IPs for a hostname
func (mp *MemoryPlugin) GetEntry(hostname string) ([]net.IP, error) {
	mp.entriesMutex.RLock()
	defer mp.entriesMutex.RUnlock()

	hostname = strings.ToLower(hostname)
	ips, exists := mp.entries[hostname]
	if !exists {
		return nil, fmt.Errorf("hostname %s not found", hostname)
	}
	
	return ips, nil
}

// DeleteEntry removes a hostname entry
func (mp *MemoryPlugin) DeleteEntry(hostname string) error {
	hostname = strings.ToLower(hostname)
	
	mp.entriesMutex.Lock()
	defer mp.entriesMutex.Unlock()
	
	if _, exists := mp.entries[hostname]; !exists {
		return fmt.Errorf("hostname %s not found", hostname)
	}
	
	delete(mp.entries, hostname)
	delete(mp.counters, hostname)
	
	mp.logger.Infof("Deleted entry for hostname %s", hostname)
	return nil
}

// ListEntries returns all hostname entries
func (mp *MemoryPlugin) ListEntries() []string {
	mp.entriesMutex.RLock()
	defer mp.entriesMutex.RUnlock()
	
	hostnames := make([]string, 0, len(mp.entries))
	for hostname := range mp.entries {
		hostnames = append(hostnames, hostname)
	}
	
	return hostnames
}
