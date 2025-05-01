package file

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a temporary hosts file with the given content
func createTempHostsFile(t *testing.T, content string) string {
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "hosts")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err, "Failed to create temporary hosts file")
	return tmpFile
}

// Test basic plugin initialization and host resolution
func TestFilePluginInit(t *testing.T) {
	// Create a temporary hosts file
	content := `
# Test hosts file
192.168.1.10 test1.local
192.168.1.11 test2.local test2-alias.local
192.168.1.12 test3.local  # With comment
`
	tmpFile := createTempHostsFile(t, content)
	defer os.Remove(tmpFile)

	// Test initialization with valid config
	config := map[string]interface{}{
		"path":             tmpFile,
		"refresh_interval": "1s",
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err)
	require.NotNil(t, plugin)

	// Test resolution
	ips, err := plugin.Resolve("test1.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.10", ips[0].String())

	// Test alias resolution
	ips, err = plugin.Resolve("test2-alias.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.11", ips[0].String())

	// Test comment handling
	ips, err = plugin.Resolve("test3.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.12", ips[0].String())

	// Test case insensitivity
	ips, err = plugin.Resolve("TEST1.LOCAL")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.10", ips[0].String())

	// Test non-existent hostname
	ips, err = plugin.Resolve("nonexistent.local")
	assert.Error(t, err)
	assert.Nil(t, ips)

	// Verify plugin name
	assert.Equal(t, "file", plugin.Name())
}

// Test file refresh functionality
func TestFileRefresh(t *testing.T) {
	// Create initial hosts file
	initialContent := "192.168.1.10 refresh-test.local"
	tmpFile := createTempHostsFile(t, initialContent)
	defer os.Remove(tmpFile)

	// Initialize plugin with a very short refresh interval
	config := map[string]interface{}{
		"path":             tmpFile,
		"refresh_interval": "100ms", // Use shorter interval for testing
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err)

	// Test initial resolution
	ips, err := plugin.Resolve("refresh-test.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.10", ips[0].String())

	// Update the hosts file
	updatedContent := "192.168.1.20 refresh-test.local\n192.168.1.30 new-host.local"
	err = os.WriteFile(tmpFile, []byte(updatedContent), 0644)
	require.NoError(t, err)

	// Wait for at least one refresh cycle plus a small buffer
	time.Sleep(200 * time.Millisecond)

	// Test that the resolution has been updated
	ips, err = plugin.Resolve("refresh-test.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.20", ips[0].String())

	// Test that the new host is available
	ips, err = plugin.Resolve("new-host.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.30", ips[0].String())
}

// Test error handling with invalid files
func TestErrorHandling(t *testing.T) {
	// Test with non-existent file
	config := map[string]interface{}{
		"path":             "/nonexistent/file",
		"refresh_interval": "1s",
	}

	plugin, err := NewFilePlugin(config)
	assert.Error(t, err)
	assert.Nil(t, plugin)

	// Test with unreadable file
	if os.Geteuid() == 0 {
		t.Skip("Skipping test that requires non-root user")
	}

	unreadableFile := filepath.Join(t.TempDir(), "unreadable")
	err = os.WriteFile(unreadableFile, []byte("192.168.1.1 test.local"), 0000)
	require.NoError(t, err)
	defer os.Remove(unreadableFile)

	config = map[string]interface{}{
		"path":             unreadableFile,
		"refresh_interval": "1s",
	}

	plugin, err = NewFilePlugin(config)
	assert.Error(t, err)
	assert.Nil(t, plugin)

	// Test with invalid refresh interval
	validFile := createTempHostsFile(t, "192.168.1.1 test.local")
	defer os.Remove(validFile)

	config = map[string]interface{}{
		"path":             validFile,
		"refresh_interval": "invalid",
	}

	plugin, err = NewFilePlugin(config)
	assert.Error(t, err, "Should error with invalid refresh interval")
	assert.Contains(t, err.Error(), "invalid refresh interval")
	assert.Nil(t, plugin)

	// Test with missing path
	config = map[string]interface{}{
		"refresh_interval": "1s",
	}

	plugin, err = NewFilePlugin(config)
	assert.Error(t, err)
	assert.Nil(t, plugin)
}

// Test parsing of different hosts file formats
func TestHostsFileFormats(t *testing.T) {
	// Test with different hosts file formats
	content := `
# Various formats test
127.0.0.1 localhost
::1 localhost-ipv6

# Multiple hostnames
192.168.1.5 host1.local host1-alias.local host1-another.local

# IP address whitespace variations
192.168.1.6	host2.local	# Tab separated
192.168.1.7       host3.local       # Multiple spaces

# Empty lines and comments

# Lines with only comments
# Another comment

# IPv6 addresses
fe80::1 ipv6-host.local
2001:db8::1 ipv6-another.local
`
	tmpFile := createTempHostsFile(t, content)
	defer os.Remove(tmpFile)

	config := map[string]interface{}{
		"path":             tmpFile,
		"refresh_interval": "1s",
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err)

	// Test IPv4 resolution
	ips, err := plugin.Resolve("localhost")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", ips[0].String())

	// Test IPv6 resolution
	ips, err = plugin.Resolve("localhost-ipv6")
	require.NoError(t, err)
	assert.Equal(t, "::1", ips[0].String())

	// Test multiple hostnames
	ips, err = plugin.Resolve("host1.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.5", ips[0].String())

	ips, err = plugin.Resolve("host1-alias.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.5", ips[0].String())

	ips, err = plugin.Resolve("host1-another.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.5", ips[0].String())

	// Test tab separated
	ips, err = plugin.Resolve("host2.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.6", ips[0].String())

	// Test multiple spaces
	ips, err = plugin.Resolve("host3.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.7", ips[0].String())

	// Test IPv6 hosts
	ips, err = plugin.Resolve("ipv6-host.local")
	require.NoError(t, err)
	assert.Equal(t, "fe80::1", ips[0].String())

	ips, err = plugin.Resolve("ipv6-another.local")
	require.NoError(t, err)
	assert.Equal(t, "2001:db8::1", ips[0].String())
}

// Test CanResolve functionality
func TestCanResolve(t *testing.T) {
	// Create a hosts file
	content := "192.168.1.1 test.local alias.local"
	tmpFile := createTempHostsFile(t, content)
	defer os.Remove(tmpFile)

	config := map[string]interface{}{
		"path":             tmpFile,
		"refresh_interval": "1s",
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err)

	// Test existing hostname
	assert.True(t, plugin.CanResolve("test.local"))
	assert.True(t, plugin.CanResolve("alias.local"))

	// Test different case
	assert.True(t, plugin.CanResolve("TEST.LOCAL"))
	assert.True(t, plugin.CanResolve("ALIAS.LOCAL"))

	// Test non-existent hostname
	assert.False(t, plugin.CanResolve("nonexistent.local"))
}

// Test invalid entries in hosts file
func TestInvalidEntries(t *testing.T) {
	// Create a hosts file with some invalid entries
	content := `
# Valid entries
192.168.1.1 valid1.local
192.168.1.2 valid2.local

# Invalid entries
invalid-ip invalid-host.local
192.168.1. incomplete-ip.local
.168.1.3 another-incomplete.local

# Valid again
192.168.1.3 valid3.local
`
	tmpFile := createTempHostsFile(t, content)
	defer os.Remove(tmpFile)

	config := map[string]interface{}{
		"path":             tmpFile,
		"refresh_interval": "1s",
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err, "Plugin should initialize even with invalid entries")

	// Test valid entries
	assert.True(t, plugin.CanResolve("valid1.local"))
	assert.True(t, plugin.CanResolve("valid2.local"))
	assert.True(t, plugin.CanResolve("valid3.local"))

	// Test invalid entries - should not be resolvable
	assert.False(t, plugin.CanResolve("invalid-host.local"))
	assert.False(t, plugin.CanResolve("incomplete-ip.local"))
	assert.False(t, plugin.CanResolve("another-incomplete.local"))

	// Test resolution of valid entries
	ips, err := plugin.Resolve("valid1.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", ips[0].String())
}

// Test plugin initialization with default refresh interval
func TestDefaultRefreshInterval(t *testing.T) {
	// Create a hosts file
	content := "192.168.1.1 test.local"
	tmpFile := createTempHostsFile(t, content)
	defer os.Remove(tmpFile)

	// Initialize without specifying refresh interval
	config := map[string]interface{}{
		"path": tmpFile,
		// No refresh_interval specified
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err)
	require.NotNil(t, plugin)

	// Test that resolution works
	ips, err := plugin.Resolve("test.local")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", ips[0].String())
}

// TestExplicitIPConversion tests explicit conversion between string IPs and net.IP
func TestExplicitIPConversion(t *testing.T) {
	// Create a hosts file
	content := "192.168.1.1 test.local"
	tmpFile := createTempHostsFile(t, content)
	defer os.Remove(tmpFile)

	config := map[string]interface{}{
		"path": tmpFile,
	}

	plugin, err := NewFilePlugin(config)
	require.NoError(t, err)

	// Get IP and convert to string and back to net.IP
	ips, err := plugin.Resolve("test.local")
	require.NoError(t, err)
	require.Len(t, ips, 1)

	// Explicitly use net.IP type
	ipString := ips[0].String()
	parsedIP := net.ParseIP(ipString)
	require.NotNil(t, parsedIP)
	
	// Check that it matches the original
	assert.Equal(t, ips[0].String(), parsedIP.String())
	
	// Test that IP comparison works
	expectedIP := net.ParseIP("192.168.1.1")
	assert.True(t, ips[0].Equal(expectedIP))
}
