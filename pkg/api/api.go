package api

import "net"

// Resolver is the interface that must be implemented by DNS resolvers
type Resolver interface {
	// Name returns the resolver's name
	Name() string
	
	// Init initializes the resolver with configuration
	Init(config map[string]interface{}) error
	
	// CanResolve checks if this resolver can handle the given hostname
	CanResolve(hostname string) bool
	
	// Resolve returns IP addresses for the given hostname
	Resolve(hostname string) ([]net.IP, error)
}

// MemoryPlugin extends Resolver with memory-specific operations
type MemoryPlugin interface {
	Resolver
	
	// GetEntry retrieves IP addresses for a hostname
	GetEntry(hostname string) ([]net.IP, error)
	
	// AddEntry adds or updates a hostname entry
	AddEntry(hostname string, ips []net.IP) error
	
	// DeleteEntry removes a hostname entry
	DeleteEntry(hostname string) error
	
	// ListEntries returns all hostnames in the plugin
	ListEntries() []string
}

