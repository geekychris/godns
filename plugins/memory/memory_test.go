package memory

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to convert string IPs to net.IP
func stringsToIPs(ipStrings []string) []net.IP {
	ips := make([]net.IP, len(ipStrings))
	for i, ipStr := range ipStrings {
		ips[i] = net.ParseIP(ipStr)
	}
	return ips
}
func TestMemoryPluginInit(t *testing.T) {
	// Test initialization with valid config
	config := map[string]interface{}{
		"entries": []map[string]interface{}{
			{
				"hostname": "test.local",
				"ips":      []string{"192.168.1.1", "192.168.1.2"},
			},
		},
	}

	plugin, err := NewMemoryPlugin(config)
	require.NoError(t, err)
	require.NotNil(t, plugin)

	// Verify the entry was loaded
	mp := plugin.(*MemoryPlugin)
	ips, err := mp.GetEntry("test.local")
	require.NoError(t, err)
	assert.Len(t, ips, 2)

	// Test initialization with malformed config
	invalidConfig := map[string]interface{}{
		"entries": 123, // Invalid type
	}
	_, err = NewMemoryPlugin(invalidConfig)
	assert.Error(t, err)

	// Test initialization with empty config
	emptyConfig := map[string]interface{}{}
	emptyPlugin, err := NewMemoryPlugin(emptyConfig)
	require.NoError(t, err)
	require.NotNil(t, emptyPlugin)
	// Should initialize with empty entries
	mp = emptyPlugin.(*MemoryPlugin)
	assert.Empty(t, mp.entries)
}

func TestAddingAndRetrievingEntries(t *testing.T) {
	// Create a plugin with empty config
	config := map[string]interface{}{}
	plugin, err := NewMemoryPlugin(config)
	require.NoError(t, err)
	mp := plugin.(*MemoryPlugin)

	// Add an entry
	hostname := "example.local"
	ipStrings := []string{"192.168.1.10", "192.168.1.11"}
	err = mp.AddEntry(hostname, stringsToIPs(ipStrings))
	require.NoError(t, err)

	// Retrieve the entry
	retrievedIPs, err := mp.GetEntry(hostname)
	require.NoError(t, err)
	assert.Len(t, retrievedIPs, 2)

	// Convert string IPs to net.IP for comparison
	expectedIPs := []net.IP{
		net.ParseIP("192.168.1.10"),
		net.ParseIP("192.168.1.11"),
	}
	assert.Equal(t, expectedIPs, retrievedIPs)

	// Test getting non-existent entry
	_, err = mp.GetEntry("nonexistent.local")
	assert.Error(t, err)

	// Test adding entry with invalid IP
	err = mp.AddEntry("invalid.local", []net.IP{nil})
	assert.Error(t, err)
}

func TestRoundRobinFunctionality(t *testing.T) {
	// Create a plugin with empty config
	config := map[string]interface{}{}
	plugin, err := NewMemoryPlugin(config)
	require.NoError(t, err)
	mp := plugin.(*MemoryPlugin)

	// Add an entry with multiple IPs
	hostname := "roundrobin.local"
	ipStrings := []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"}
	err = mp.AddEntry(hostname, stringsToIPs(ipStrings))
	require.NoError(t, err)

	// Test round-robin behavior
	// First resolution should return first IP
	resolvedIPs, err := mp.Resolve(hostname)
	require.NoError(t, err)
	assert.Equal(t, net.ParseIP("192.168.1.10"), resolvedIPs[0])

	// Second resolution should return second IP
	resolvedIPs, err = mp.Resolve(hostname)
	require.NoError(t, err)
	assert.Equal(t, net.ParseIP("192.168.1.11"), resolvedIPs[0])

	// Third resolution should return third IP
	resolvedIPs, err = mp.Resolve(hostname)
	require.NoError(t, err)
	assert.Equal(t, net.ParseIP("192.168.1.12"), resolvedIPs[0])

	// Fourth resolution should cycle back to first IP
	resolvedIPs, err = mp.Resolve(hostname)
	require.NoError(t, err)
	assert.Equal(t, net.ParseIP("192.168.1.10"), resolvedIPs[0])
}

func TestThreadSafety(t *testing.T) {
	// Create a plugin with empty config
	config := map[string]interface{}{}
	plugin, err := NewMemoryPlugin(config)
	require.NoError(t, err)
	mp := plugin.(*MemoryPlugin)

	// Add an entry with multiple IPs
	hostname := "concurrent.local"
	ipStrings := []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"}
	err = mp.AddEntry(hostname, stringsToIPs(ipStrings))
	require.NoError(t, err)

	// Test concurrent access
	var wg sync.WaitGroup
	const numGoroutines = 10
	const numRequests = 10

	// Track results to verify round-robin behavior works correctly under load
	results := make([]net.IP, numGoroutines*numRequests)
	var resultsMutex sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numRequests; j++ {
				resolvedIPs, err := mp.Resolve(hostname)
				if err != nil {
					t.Errorf("Error resolving hostname: %v", err)
					return
				}
				
				resultsMutex.Lock()
				results[goroutineID*numRequests+j] = resolvedIPs[0]
				resultsMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Verify we got some results (we can't guarantee order due to concurrency)
	for _, ip := range results {
		assert.NotNil(t, ip)
		// Check if it's one of our expected IPs
		found := false
		for _, expectedIP := range ipStrings {
			if ip.String() == expectedIP {
				found = true
				break
			}
		}
		assert.True(t, found, "IP %s not found in expected list", ip.String())
	}
}

func TestCaseInsensitivity(t *testing.T) {
	// Create a plugin with empty config
	config := map[string]interface{}{}
	plugin, err := NewMemoryPlugin(config)
	require.NoError(t, err)
	mp := plugin.(*MemoryPlugin)

	// Add an entry
	hostname := "MixedCase.Local"
	ipStrings := []string{"192.168.1.10"}
	err = mp.AddEntry(hostname, stringsToIPs(ipStrings))
	require.NoError(t, err)

	// Test retrieving with different case
	testCases := []string{
		"mixedcase.local",
		"MIXEDCASE.LOCAL",
		"MixedCase.Local",
		"mixedCase.LOCAL",
	}

	for _, tc := range testCases {
		resolvedIPs, err := mp.Resolve(tc)
		require.NoError(t, err, "Failed to resolve %s", tc)
		assert.Equal(t, net.ParseIP("192.168.1.10"), resolvedIPs[0])
	}
}

func TestErrorHandling(t *testing.T) {
	// Create a plugin with empty config
	config := map[string]interface{}{}
	plugin, err := NewMemoryPlugin(config)
	require.NoError(t, err)
	mp := plugin.(*MemoryPlugin)

	// Test resolving non-existent hostname
	_, err = mp.Resolve("nonexistent.local")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test adding entry with empty hostname
	err = mp.AddEntry("", stringsToIPs([]string{"192.168.1.10"}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname cannot be empty")

	// Test adding entry with empty IPs
	err = mp.AddEntry("empty.local", stringsToIPs([]string{}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no IP addresses provided")

	// Test can resolve behavior
	assert.False(t, mp.CanResolve("nonexistent.local"))
	
	err = mp.AddEntry("exists.local", stringsToIPs([]string{"192.168.1.10"}))
	require.NoError(t, err)
	assert.True(t, mp.CanResolve("exists.local"))
	// Case insensitive check
	assert.True(t, mp.CanResolve("EXISTS.LOCAL"))
}
