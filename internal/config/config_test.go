package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyDefaults(t *testing.T) {
	t.Run("zero value gets all defaults", func(t *testing.T) {
		cfg := &Config{}
		applyDefaults(cfg)

		assert.Equal(t, "0.0.0.0", cfg.Server.Address)
		assert.Equal(t, 53, cfg.Server.Port)
		assert.Equal(t, 3600*time.Second, cfg.Server.CacheTTL)

		assert.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53"}, cfg.Upstream.Servers)
		assert.Equal(t, 5*time.Second, cfg.Upstream.Timeout)

		assert.Equal(t, "info", cfg.Logging.Level)
		assert.Equal(t, "text", cfg.Logging.Format)

		// REST defaults are only filled in when REST is enabled.
		assert.Equal(t, "", cfg.REST.Address)
		assert.Equal(t, 0, cfg.REST.Port)
	})

	t.Run("already-set fields are left unchanged", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address:  "10.0.0.1",
				Port:     5353,
				CacheTTL: 42 * time.Second,
			},
			Upstream: UpstreamConfig{
				Servers: []string{"9.9.9.9:53"},
				Timeout: 7 * time.Second,
			},
			Logging: LoggingConfig{
				Level:  "warn",
				Format: "json",
			},
		}
		applyDefaults(cfg)

		assert.Equal(t, "10.0.0.1", cfg.Server.Address)
		assert.Equal(t, 5353, cfg.Server.Port)
		assert.Equal(t, 42*time.Second, cfg.Server.CacheTTL)
		assert.Equal(t, []string{"9.9.9.9:53"}, cfg.Upstream.Servers)
		assert.Equal(t, 7*time.Second, cfg.Upstream.Timeout)
		assert.Equal(t, "warn", cfg.Logging.Level)
		assert.Equal(t, "json", cfg.Logging.Format)
	})

	t.Run("REST enabled fills in address and port defaults", func(t *testing.T) {
		cfg := &Config{REST: RESTConfig{Enabled: true}}
		applyDefaults(cfg)

		assert.Equal(t, "0.0.0.0", cfg.REST.Address)
		assert.Equal(t, 8080, cfg.REST.Port)
	})

	t.Run("REST disabled leaves address and port zero", func(t *testing.T) {
		cfg := &Config{REST: RESTConfig{Enabled: false}}
		applyDefaults(cfg)

		assert.Equal(t, "", cfg.REST.Address)
		assert.Equal(t, 0, cfg.REST.Port)
	})
}

func TestCreateDefaultConfig(t *testing.T) {
	expected := &Config{
		Server: ServerConfig{
			Address:  "0.0.0.0",
			Port:     53,
			CacheTTL: 3600 * time.Second,
		},
		Upstream: UpstreamConfig{
			Servers: []string{"8.8.8.8:53", "1.1.1.1:53"},
			Timeout: 5 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	assert.Equal(t, expected, CreateDefaultConfig())
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadConfig_AppliesDefaultsForMinimalFile(t *testing.T) {
	path := writeTempConfig(t, "server:\n  address: 127.0.0.1\n")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	// Explicitly-set field is respected.
	assert.Equal(t, "127.0.0.1", cfg.Server.Address)

	// Everything else gets the same defaults as applyDefaults.
	assert.Equal(t, 53, cfg.Server.Port)
	assert.Equal(t, 3600*time.Second, cfg.Server.CacheTTL)
	assert.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53"}, cfg.Upstream.Servers)
	assert.Equal(t, 5*time.Second, cfg.Upstream.Timeout)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "text", cfg.Logging.Format)
}

func TestLoadConfig_ParsesDurationStrings(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		getField func(*Config) time.Duration
		want     time.Duration
	}{
		{
			name: "cache_ttl 10s",
			yaml: "server:\n  cache_ttl: 10s\n",
			getField: func(c *Config) time.Duration {
				return c.Server.CacheTTL
			},
			want: 10 * time.Second,
		},
		{
			name: "upstream timeout 500ms",
			yaml: "upstream:\n  timeout: 500ms\n",
			getField: func(c *Config) time.Duration {
				return c.Upstream.Timeout
			},
			want: 500 * time.Millisecond,
		},
		{
			name: "plugins.file.refresh_interval 2h",
			yaml: "plugins:\n  file:\n    refresh_interval: 2h\n",
			getField: func(c *Config) time.Duration {
				return c.Plugins.File.RefreshInterval
			},
			want: 2 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)
			cfg, err := LoadConfig(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, tt.getField(cfg))
		})
	}
}

func TestLoadConfig_InvalidDurationStrings(t *testing.T) {
	// yaml.v3 unmarshals time.Duration fields directly, so an invalid
	// duration string fails during the typed Unmarshal pass (returning the
	// generic "error parsing config file" wrapper) before LoadConfig's own
	// field-specific ParseDuration checks ever run.
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "invalid cache_ttl",
			yaml: "server:\n  cache_ttl: not-a-duration\n",
		},
		{
			name: "invalid upstream timeout",
			yaml: "upstream:\n  timeout: not-a-duration\n",
		},
		{
			name: "invalid refresh_interval",
			yaml: "plugins:\n  file:\n    refresh_interval: not-a-duration\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)
			_, err := LoadConfig(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "error parsing config file")
		})
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file does not exist")
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "server:\n  address: [this is not valid yaml\n")
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing config file")
}

func TestLoadConfig_DefaultPathWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(cwd)) }()

	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.WriteFile("config.yaml", []byte("server:\n  address: 1.2.3.4\n"), 0644))

	cfg, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", cfg.Server.Address)
}

func TestSaveConfig_RoundTrip(t *testing.T) {
	original := &Config{
		Server: ServerConfig{
			Address:  "192.168.0.1",
			Port:     5300,
			CacheTTL: 17 * time.Minute,
		},
		Upstream: UpstreamConfig{
			Servers: []string{"9.9.9.9:53", "1.0.0.1:53"},
			Timeout: 3 * time.Second,
		},
		Plugins: PluginsConfig{
			File: FilePluginConfig{
				Enabled:         true,
				Path:            "/etc/godns/hosts.yaml",
				RefreshInterval: 90 * time.Second,
			},
			Memory: MemoryPluginConfig{
				Enabled: true,
				Entries: []map[string]interface{}{
					{"hostname": "example.com", "ips": []interface{}{"1.2.3.4"}},
				},
			},
		},
		REST: RESTConfig{
			Enabled: true,
			Address: "127.0.0.1",
			Port:    9090,
		},
		Logging: LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
	}

	path := filepath.Join(t.TempDir(), "roundtrip.yaml")
	require.NoError(t, SaveConfig(original, path))

	loaded, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, original, loaded)
}

func TestSaveConfig_WriteError(t *testing.T) {
	// A path inside a nonexistent directory cannot be written.
	path := filepath.Join(t.TempDir(), "nonexistent-dir", "config.yaml")
	err := SaveConfig(CreateDefaultConfig(), path)
	require.Error(t, err)
}
