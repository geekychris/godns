package server

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/godns/internal/config"
)

// MockResolver is a mock implementation of the Resolver interface
type MockResolver struct {
	mock.Mock
}

func (m *MockResolver) CanResolve(hostname string) bool {
	args := m.Called(hostname)
	return args.Bool(0)
}

func (m *MockResolver) Resolve(hostname string) ([]net.IP, error) {
	args := m.Called(hostname)
	return args.Get(0).([]net.IP), args.Error(1)
}

func (m *MockResolver) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockResolver) Init(config map[string]interface{}) error {
	args := m.Called(config)
	return args.Error(0)
}

// MockRESTServer is a mock implementation of the REST server
type MockRESTServer struct {
	mock.Mock
}

func (m *MockRESTServer) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRESTServer) Stop() error {
	args := m.Called()
	return args.Error(0)
}

// createTestConfig creates a configuration for testing
func createTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Address:  "127.0.0.1",
			Port:     0, // Use random port
			CacheTTL: 1 * time.Second,
		},
		Upstream: config.UpstreamConfig{
			Servers: []string{"8.8.8.8:53", "1.1.1.1:53"},
			Timeout: 5 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
		},
	}
}

// createTestLogger creates a logger for testing
func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return logger
}

// Test server initialization
func TestNewDNSServer(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()

	server := NewDNSServer(cfg, logger)
	assert.NotNil(t, server)
	assert.Equal(t, cfg, server.config)
	assert.NotNil(t, server.cache)
	assert.Empty(t, server.resolvers)
}

// Test resolver registration
func TestRegisterResolver(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	// Create a mock resolver
	mockResolver := new(MockResolver)
	mockResolver.On("Name").Return("mock")

	// Register the resolver
	server.RegisterResolver(mockResolver)

	// Verify it was registered
	assert.Len(t, server.resolvers, 1)
	assert.Equal(t, mockResolver, server.resolvers[0])

	// Register the same resolver again (should be a no-op)
	server.RegisterResolver(mockResolver)
	assert.Len(t, server.resolvers, 1)

	mockResolver.AssertExpectations(t)
}

// Test cache operations
func TestCacheOperations(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	// Test caching result
	hostname := "test.example.com"
	ips := []net.IP{net.ParseIP("192.168.1.1")}
	server.cacheResult(hostname, ips)

	// Test resolving from cache
	cachedIPs := server.resolveFromCache(hostname)
	assert.Equal(t, ips, cachedIPs)

	// Test cache expiration
	time.Sleep(cfg.Server.CacheTTL + 100*time.Millisecond) // Wait for cache to expire
	expiredIPs := server.resolveFromCache(hostname)
	assert.Nil(t, expiredIPs)
}

// Test plugin resolution
func TestResolveWithPlugins(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	hostname := "test.example.com"
	expectedIP := net.ParseIP("192.168.1.1")

	// Create a mock resolver
	mockResolver := new(MockResolver)
	mockResolver.On("Name").Return("mock")
	mockResolver.On("CanResolve", hostname).Return(true)
	mockResolver.On("Resolve", hostname).Return([]net.IP{expectedIP}, nil)

	// Register the resolver
	server.RegisterResolver(mockResolver)

	// Test resolving with plugins
	ips, err := server.resolveWithPlugins(hostname)
	assert.NoError(t, err)
	assert.Equal(t, []net.IP{expectedIP}, ips)

	// Test resolving an unknown hostname
	unknownHostname := "unknown.example.com"
	mockResolver.On("CanResolve", unknownHostname).Return(false)
	ips, err = server.resolveWithPlugins(unknownHostname)
	assert.Error(t, err)
	assert.Nil(t, ips)

	mockResolver.AssertExpectations(t)
}

// Test round-robin resolution
func TestRoundRobinResolution(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	hostname := "roundrobin.example.com"
	ip1 := net.ParseIP("192.168.1.1")
	ip2 := net.ParseIP("192.168.1.2")
	ip3 := net.ParseIP("192.168.1.3")

	// Custom RoundRobinResolver that implements round-robin behavior
	roundRobinResolver := &CustomRoundRobinResolver{
		hostname: hostname,
		ips:      []net.IP{ip1, ip2, ip3},
	}

	// Register the resolver
	server.RegisterResolver(roundRobinResolver)

	// Test first query - should get first IP
	ips, err := server.resolveWithPlugins(hostname)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{ip1}, ips)

	// Test second query - should get second IP
	ips, err = server.resolveWithPlugins(hostname)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{ip2}, ips)

	// Test third query - should get third IP
	ips, err = server.resolveWithPlugins(hostname)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{ip3}, ips)

	// Test fourth query - should cycle back to first IP
	ips, err = server.resolveWithPlugins(hostname)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{ip1}, ips)
}

// Test setting the REST server
func TestSetRESTServer(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	mockRESTServer := new(MockRESTServer)
	server.SetRESTServer(mockRESTServer)

	assert.Equal(t, mockRESTServer, server.restServer)
}

// Test cache cleanup
func TestCleanupExpiredCache(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	// Add multiple entries to the cache
	hostname1 := "test1.example.com"
	hostname2 := "test2.example.com"
	ips := []net.IP{net.ParseIP("192.168.1.1")}

	server.cacheResult(hostname1, ips)
	server.cacheResult(hostname2, ips)

	// Check that both entries are in the cache
	assert.Len(t, server.cache, 2)

	// Manually expire one entry
	server.cacheMutex.Lock()
	server.cache[hostname1].expiresAt = time.Now().Add(-1 * time.Second) // Set to past
	server.cacheMutex.Unlock()

	// Run cache cleanup
	server.cleanupExpiredCache()

	// Check that one entry was removed
	assert.Len(t, server.cache, 1)
	assert.NotNil(t, server.resolveFromCache(hostname2))
	assert.Nil(t, server.resolveFromCache(hostname1))
}

// Integration test for DNS query handling
func TestHandleDNSRequestIntegration(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	hostname := "test.example.com"
	expectedIP := net.ParseIP("192.168.1.1")

	// Create a mock resolver
	mockResolver := new(MockResolver)
	mockResolver.On("Name").Return("mock")
	mockResolver.On("CanResolve", hostname).Return(true)
	mockResolver.On("Resolve", hostname).Return([]net.IP{expectedIP}, nil)

	// Register the resolver
	server.RegisterResolver(mockResolver)

	// Create a DNS question
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)

	// Create a mock response writer
	mockWriter := &MockResponseWriter{}

	// Handle the request
	server.handleDNSRequest(mockWriter, m)

	// Verify the response
	resp := mockWriter.writtenMsg
	require.NotNil(t, resp)
	assert.Equal(t, dns.RcodeSuccess, resp.Rcode)
	assert.Len(t, resp.Answer, 1)

	// Check the A record
	aRecord, ok := resp.Answer[0].(*dns.A)
	require.True(t, ok)
	assert.Equal(t, expectedIP.String(), aRecord.A.String())
	assert.Equal(t, uint32(cfg.Server.CacheTTL.Seconds()), aRecord.Hdr.Ttl)

	mockResolver.AssertExpectations(t)
}

// MockResponseWriter implements the dns.ResponseWriter interface for testing
type MockResponseWriter struct {
	writtenMsg *dns.Msg
}

func (m *MockResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53}
}

func (m *MockResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
}

func (m *MockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.writtenMsg = msg
	return nil
}

func (m *MockResponseWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

func (m *MockResponseWriter) Close() error {
	return nil
}

func (m *MockResponseWriter) TsigStatus() error {
	return nil
}

func (m *MockResponseWriter) TsigTimersOnly(bool) {
}

func (m *MockResponseWriter) Hijack() {
}

// CustomRoundRobinResolver is a simple resolver that implements round-robin behavior
type CustomRoundRobinResolver struct {
	hostname string
	ips      []net.IP
	counter  int
}

func (r *CustomRoundRobinResolver) CanResolve(hostname string) bool {
	return hostname == r.hostname
}

func (r *CustomRoundRobinResolver) Resolve(hostname string) ([]net.IP, error) {
	if hostname != r.hostname {
		return nil, fmt.Errorf("unknown hostname: %s", hostname)
	}

	// Simple round-robin implementation
	ip := r.ips[r.counter%len(r.ips)]
	r.counter++

	return []net.IP{ip}, nil
}

func (r *CustomRoundRobinResolver) Name() string {
	return "custom-round-robin"
}

func (r *CustomRoundRobinResolver) Init(config map[string]interface{}) error {
	return nil
}
// TestCacheTTLBehavior verifies that the cache TTL is properly handled
func TestCacheTTLBehavior(t *testing.T) {
	// Create a server with a short cache TTL
	cfg := createTestConfig()
	cfg.Server.CacheTTL = 500 * time.Millisecond // Very short TTL for testing
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	hostname := "ttl-test.example.com"
	ip := net.ParseIP("192.168.1.50")

	// Add entry to cache
	server.cacheResult(hostname, []net.IP{ip})

	// Verify entry is in cache
	result := server.resolveFromCache(hostname)
	assert.NotNil(t, result, "Entry should be in cache initially")
	assert.Equal(t, ip, result[0], "Cached IP should match")

	// Get the TTL from the cache entry's expiration time
	server.cacheMutex.RLock()
	entry := server.cache[hostname]
	// Calculate the TTL by getting the duration between now and the entry's expiration
	ttlRemaining := time.Until(entry.expiresAt)
	server.cacheMutex.RUnlock()

	// Verify the TTL is close to what we set
	assert.InDelta(t, cfg.Server.CacheTTL.Milliseconds(), ttlRemaining.Milliseconds(), 100, 
		"Cache entry TTL should be close to the configured value")

	// Create a DNS response and check TTL in record
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	mockWriter := &MockResponseWriter{}
	server.handleDNSRequest(mockWriter, m)
	
	// Verify the DNS TTL in the response header
	resp := mockWriter.writtenMsg
	require.NotNil(t, resp)
	assert.Len(t, resp.Answer, 1, "Response should have one answer")
	
	aRecord, ok := resp.Answer[0].(*dns.A)
	require.True(t, ok, "Answer should be an A record")
	assert.Equal(t, uint32(cfg.Server.CacheTTL.Seconds()), aRecord.Hdr.Ttl, 
		"TTL in DNS response should match config")

	// Wait for the cache to expire
	time.Sleep(cfg.Server.CacheTTL + 100*time.Millisecond)
	
	// Verify entry is no longer in cache
	result = server.resolveFromCache(hostname)
	assert.Nil(t, result, "Entry should have expired from cache")
}

// TestRESTAPIIntegration tests the interaction between the DNS server and REST API
func TestRESTAPIIntegration(t *testing.T) {
	cfg := createTestConfig()
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)
	
	// Create a mock REST server
	mockRESTServer := new(MockRESTServer)
	mockRESTServer.On("Stop").Return(nil)
	
	// Set the REST server
	server.SetRESTServer(mockRESTServer)
	
	// Verify the REST server was set
	assert.Equal(t, mockRESTServer, server.restServer)
	
	// Test clean server shutdown with REST server
	err := server.Stop()
	assert.NoError(t, err, "Server shutdown should succeed")
	
	// Verify that the REST server's Stop method was called
	mockRESTServer.AssertExpectations(t)
}

// TestCompleteSystemIntegration tests the entire DNS resolution flow
func TestCompleteSystemIntegration(t *testing.T) {
	cfg := createTestConfig()
	// Use a very short cache TTL for testing
	cfg.Server.CacheTTL = 200 * time.Millisecond
	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	// Set up test data
	hostname := "integration.example.com"
	ip1 := net.ParseIP("192.168.1.10")
	ip2 := net.ParseIP("192.168.1.11")
	ip3 := net.ParseIP("192.168.1.12")

	// Create a round-robin resolver
	resolver := &CustomRoundRobinResolver{
		hostname: hostname,
		ips:      []net.IP{ip1, ip2, ip3},
	}

	// Register the resolver
	server.RegisterResolver(resolver)

	// Test DNS resolution with caching and round-robin
	mockWriter := &MockResponseWriter{}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)

	// First query - should get first IP and cache it
	server.handleDNSRequest(mockWriter, m)
	resp1 := mockWriter.writtenMsg
	require.NotNil(t, resp1)
	assert.Equal(t, dns.RcodeSuccess, resp1.Rcode)
	assert.Len(t, resp1.Answer, 1)
	record1 := resp1.Answer[0].(*dns.A)
	assert.Equal(t, ip1.String(), record1.A.String())

	// Second query - should get cached result (same IP)
	mockWriter.writtenMsg = nil
	server.handleDNSRequest(mockWriter, m)
	resp2 := mockWriter.writtenMsg
	require.NotNil(t, resp2)
	record2 := resp2.Answer[0].(*dns.A)
	assert.Equal(t, ip1.String(), record2.A.String())

	// Wait for cache to expire
	time.Sleep(cfg.Server.CacheTTL + 100*time.Millisecond)

	// Third query - should get second IP (round-robin) and cache it
	mockWriter.writtenMsg = nil
	server.handleDNSRequest(mockWriter, m)
	resp3 := mockWriter.writtenMsg
	require.NotNil(t, resp3)
	record3 := resp3.Answer[0].(*dns.A)
	assert.Equal(t, ip2.String(), record3.A.String())

	// Test error case - unknown hostname
	unknownMsg := new(dns.Msg)
	unknownMsg.SetQuestion(dns.Fqdn("unknown.example.com"), dns.TypeA)
	mockWriter.writtenMsg = nil
	server.handleDNSRequest(mockWriter, unknownMsg)
	errResp := mockWriter.writtenMsg
	require.NotNil(t, errResp)
	assert.Equal(t, 0, len(errResp.Answer), "Should have no answers for unknown hostname")

	// Test non-A record type
	aaaaMsg := new(dns.Msg)
	aaaaMsg.SetQuestion(dns.Fqdn(hostname), dns.TypeAAAA)
	mockWriter.writtenMsg = nil
	server.handleDNSRequest(mockWriter, aaaaMsg)
	aaaaResp := mockWriter.writtenMsg
	require.NotNil(t, aaaaResp)
	assert.Equal(t, 0, len(aaaaResp.Answer), "Should have no answers for AAAA query")
}
