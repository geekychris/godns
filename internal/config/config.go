package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config contains the complete server configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Upstream UpstreamConfig `yaml:"upstream"`
	Plugins  PluginsConfig  `yaml:"plugins"`
	REST     RESTConfig     `yaml:"rest"`
	Logging  LoggingConfig  `yaml:"logging"`
	DNSSEC   DNSSECConfig   `yaml:"dnssec"`
}

// ServerConfig contains DNS server settings
type ServerConfig struct {
	Address  string        `yaml:"address"`
	Port     int           `yaml:"port"`
	CacheTTL time.Duration `yaml:"cache_ttl"`
}

// UpstreamConfig contains upstream DNS server settings
type UpstreamConfig struct {
	Servers []string      `yaml:"servers"`
	Timeout time.Duration `yaml:"timeout"`
}

// PluginsConfig contains plugin-specific configurations
type PluginsConfig struct {
	File   FilePluginConfig   `yaml:"file"`
	Memory MemoryPluginConfig `yaml:"memory"`
	// Add other plugin configs here as needed
}

// FilePluginConfig contains configuration for the file-based plugin
type FilePluginConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Path            string        `yaml:"path"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
}

// MemoryPluginConfig contains configuration for the in-memory plugin
type MemoryPluginConfig struct {
	Enabled bool                     `yaml:"enabled"`
	Entries []map[string]interface{} `yaml:"entries"`
}

// RESTConfig contains REST API settings
type RESTConfig struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DNSSECConfig contains DNSSEC signing and validation settings. Both halves
// are independently opt-in and default to disabled, so existing config
// files are unaffected by this section's presence.
type DNSSECConfig struct {
	Signing    DNSSECSigningConfig    `yaml:"signing"`
	Validation DNSSECValidationConfig `yaml:"validation"`
}

// DNSSECSigningConfig configures authoritative zone signing. KeyFile is a
// path prefix: KeyFile+".key" holds the public DNSKEY RR and
// KeyFile+".private" holds the private key, matching the layout written by
// cmd/godns-keygen.
type DNSSECSigningConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Zone         string        `yaml:"zone"`
	KeyFile      string        `yaml:"key_file"`
	SignatureTTL time.Duration `yaml:"signature_ttl"`
}

// DNSSECValidationConfig configures validation of upstream responses.
// TrustedKeyFiles lists paths to ".key" files (the public DNSKEY RR text
// produced by cmd/godns-keygen) that are pinned as trusted.
type DNSSECValidationConfig struct {
	Enabled         bool     `yaml:"enabled"`
	TrustedKeyFiles []string `yaml:"trusted_key_files"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// Set default config file path if not provided
	if path == "" {
		path = "config.yaml"
	}

	// Check if file exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", path)
	}

	// Read file
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	// Parse YAML into a map first for custom parsing
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	// Create config with defaults
	config := CreateDefaultConfig()

	// Parse the YAML into the config struct
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	// Parse duration strings
	if serverCfg, ok := rawConfig["server"].(map[string]interface{}); ok {
		if cacheTTLStr, ok := serverCfg["cache_ttl"].(string); ok {
			ttl, err := time.ParseDuration(cacheTTLStr)
			if err != nil {
				return nil, fmt.Errorf("invalid cache_ttl duration: %v", err)
			}
			config.Server.CacheTTL = ttl
		}
	}

	// Parse upstream timeout
	if upstreamCfg, ok := rawConfig["upstream"].(map[string]interface{}); ok {
		if timeoutStr, ok := upstreamCfg["timeout"].(string); ok {
			timeout, err := time.ParseDuration(timeoutStr)
			if err != nil {
				return nil, fmt.Errorf("invalid upstream timeout duration: %v", err)
			}
			config.Upstream.Timeout = timeout
		}
	}

	// Parse file plugin refresh interval
	if pluginsCfg, ok := rawConfig["plugins"].(map[string]interface{}); ok {
		if fileCfg, ok := pluginsCfg["file"].(map[string]interface{}); ok {
			if refreshStr, ok := fileCfg["refresh_interval"].(string); ok {
				refresh, err := time.ParseDuration(refreshStr)
				if err != nil {
					return nil, fmt.Errorf("invalid file plugin refresh_interval duration: %v", err)
				}
				config.Plugins.File.RefreshInterval = refresh
			}
		}
	}

	// Parse DNSSEC signing signature TTL
	if dnssecCfg, ok := rawConfig["dnssec"].(map[string]interface{}); ok {
		if signingCfg, ok := dnssecCfg["signing"].(map[string]interface{}); ok {
			if ttlStr, ok := signingCfg["signature_ttl"].(string); ok {
				ttl, err := time.ParseDuration(ttlStr)
				if err != nil {
					return nil, fmt.Errorf("invalid dnssec signing signature_ttl duration: %v", err)
				}
				config.DNSSEC.Signing.SignatureTTL = ttl
			}
		}
	}

	return config, nil
}

// applyDefaults sets default values for configuration options
func applyDefaults(config *Config) {
	// Server defaults
	if config.Server.Address == "" {
		config.Server.Address = "0.0.0.0"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 53
	}
	if config.Server.CacheTTL == 0 {
		config.Server.CacheTTL = time.Hour
	}

	// Upstream defaults
	if len(config.Upstream.Servers) == 0 {
		config.Upstream.Servers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}
	if config.Upstream.Timeout == 0 {
		config.Upstream.Timeout = 5 * time.Second
	}

	// REST API defaults
	if config.REST.Enabled && config.REST.Address == "" {
		config.REST.Address = "0.0.0.0"
	}
	if config.REST.Enabled && config.REST.Port == 0 {
		config.REST.Port = 8080
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "text"
	}

	// DNSSEC defaults
	if config.DNSSEC.Signing.SignatureTTL == 0 {
		config.DNSSEC.Signing.SignatureTTL = time.Hour
	}
}

// CreateDefaultConfig creates a config with default values
func CreateDefaultConfig() *Config {
	config := &Config{}
	applyDefaults(config)
	return config
}

// SaveConfig saves the configuration to a YAML file
func SaveConfig(config *Config, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshaling config: %v", err)
	}

	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("error writing config file: %v", err)
	}

	return nil
}
