package file

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/yourusername/godns/pkg/api"
)

// FilePlugin implements the Resolver interface for DNS resolution from a file
type FilePlugin struct {
	path            string
	refreshInterval time.Duration
	entries         map[string][]net.IP
	entriesMutex    sync.RWMutex
	logger          *logrus.Logger
	stopChan        chan struct{}
}

// NewFilePlugin creates a new file-based resolver
func NewFilePlugin(config map[string]interface{}) (api.Resolver, error) {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	})

	// Extract configuration
	path, ok := config["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("file plugin requires a valid path")
	}

	var refreshInterval time.Duration = 60 * time.Second // Default: 60 seconds
	if ri, ok := config["refresh_interval"]; ok {
		switch v := ri.(type) {
		case time.Duration:
			refreshInterval = v
		case int:
			refreshInterval = time.Duration(v) * time.Second
		case string:
			var err error
			refreshInterval, err = time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("invalid refresh interval: %v", err)
			}
		default:
			return nil, fmt.Errorf("refresh interval must be a duration string, int (seconds), or time.Duration")
		}
	}

	if refreshInterval <= 0 {
		return nil, fmt.Errorf("refresh interval must be positive")
	}
	fp := &FilePlugin{
		path:            path,
		refreshInterval: refreshInterval,
		entries:         make(map[string][]net.IP),
		logger:          logger,
		stopChan:        make(chan struct{}),
	}

	// Initialize by loading the file
	if err := fp.loadHostsFile(); err != nil {
		return nil, err
	}

	// Start background refresh
	go fp.refreshLoop()

	return fp, nil
}

// Name returns the plugin name
func (fp *FilePlugin) Name() string {
	return "file"
}

// Init initializes the plugin with the given configuration
func (fp *FilePlugin) Init(config map[string]interface{}) error {
	// No-op as initialization is done in constructor
	return nil
}

// CanResolve determines if this plugin can resolve the given hostname
func (fp *FilePlugin) CanResolve(hostname string) bool {
	fp.entriesMutex.RLock()
	defer fp.entriesMutex.RUnlock()

	hostname = strings.ToLower(hostname)
	_, exists := fp.entries[hostname]
	return exists
}

// Resolve returns IP addresses for the given hostname
func (fp *FilePlugin) Resolve(hostname string) ([]net.IP, error) {
	fp.entriesMutex.RLock()
	defer fp.entriesMutex.RUnlock()

	hostname = strings.ToLower(hostname)
	ips, exists := fp.entries[hostname]
	if !exists {
		return nil, fmt.Errorf("hostname %s not found in hosts file", hostname)
	}

	return ips, nil
}

// loadHostsFile loads the hosts file into memory
func (fp *FilePlugin) loadHostsFile() error {
	file, err := os.Open(fp.path)
	if err != nil {
		fp.logger.Errorf("Error opening hosts file %s: %v", fp.path, err)
		return err
	}
	defer file.Close()

	newEntries := make(map[string][]net.IP)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			fp.logger.Warnf("Line %d: Invalid format (not enough fields): %s", lineNum, line)
			continue
		}

		// First field is the IP address
		ip := net.ParseIP(fields[0])
		if ip == nil {
			fp.logger.Warnf("Line %d: Invalid IP address: %s", lineNum, fields[0])
			continue
		}

		// Remaining fields are hostnames
		for _, hostname := range fields[1:] {
			hostname = strings.ToLower(hostname)
			newEntries[hostname] = append(newEntries[hostname], ip)
			fp.logger.Debugf("Added mapping: %s -> %s", hostname, ip.String())
		}
	}

	if err := scanner.Err(); err != nil {
		fp.logger.Errorf("Error reading hosts file: %v", err)
		return err
	}

	fp.entriesMutex.Lock()
	fp.entries = newEntries
	fp.entriesMutex.Unlock()

	fp.logger.Infof("Loaded %d hostname entries from %s", len(newEntries), fp.path)
	return nil
}

// refreshLoop periodically reloads the hosts file
func (fp *FilePlugin) refreshLoop() {
	ticker := time.NewTicker(fp.refreshInterval)
	defer ticker.Stop()

	fp.logger.Infof("Starting refresh loop with interval of %v", fp.refreshInterval)

	for {
		select {
		case <-ticker.C:
			fp.logger.Debugf("Refreshing hosts file %s", fp.path)
			if err := fp.loadHostsFile(); err != nil {
				fp.logger.Warnf("Failed to refresh hosts file: %v", err)
			}
		case <-fp.stopChan:
			fp.logger.Info("Stopping file plugin refresh loop")
			return
		}
	}
}

// Stop stops the background refresh goroutine
func (fp *FilePlugin) Stop() {
	close(fp.stopChan)
}

// GetEntries returns a copy of all hostname-to-IP mappings
func (fp *FilePlugin) GetEntries() map[string][]net.IP {
	fp.entriesMutex.RLock()
	defer fp.entriesMutex.RUnlock()

	// Create a deep copy to avoid race conditions
	entries := make(map[string][]net.IP, len(fp.entries))
	for hostname, ips := range fp.entries {
		ipsCopy := make([]net.IP, len(ips))
		copy(ipsCopy, ips)
		entries[hostname] = ipsCopy
	}

	return entries
}

// GetHostnames returns all hostnames in the hosts file
func (fp *FilePlugin) GetHostnames() []string {
	fp.entriesMutex.RLock()
	defer fp.entriesMutex.RUnlock()

	hostnames := make([]string, 0, len(fp.entries))
	for hostname := range fp.entries {
		hostnames = append(hostnames, hostname)
	}

	return hostnames
}

// ForceRefresh forces an immediate refresh of the hosts file
func (fp *FilePlugin) ForceRefresh() error {
	fp.logger.Info("Forcing refresh of hosts file")
	return fp.loadHostsFile()
}
