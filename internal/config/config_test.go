package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDefaultConfig(t *testing.T) {
	cfg := CreateDefaultConfig()

	assert.Equal(t, "0.0.0.0", cfg.Server.Address)
	assert.Equal(t, 53, cfg.Server.Port)
	assert.Equal(t, time.Hour, cfg.Server.CacheTTL)
	assert.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53"}, cfg.Upstream.Servers)
	assert.Equal(t, 5*time.Second, cfg.Upstream.Timeout)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "text", cfg.Logging.Format)
}

func TestApplyDefaultsPreservesNonZeroFields(t *testing.T) {
	tests := []struct {
		name  string
		input *Config
		check func(t *testing.T, cfg *Config)
	}{
		{
			name:  "empty config gets all defaults",
			input: &Config{},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "0.0.0.0", cfg.Server.Address)
				assert.Equal(t, 53, cfg.Server.Port)
				assert.Equal(t, time.Hour, cfg.Server.CacheTTL)
				assert.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53"}, cfg.Upstream.Servers)
				assert.Equal(t, 5*time.Second, cfg.Upstream.Timeout)
				assert.Equal(t, "info", cfg.Logging.Level)
				assert.Equal(t, "text", cfg.Logging.Format)
				assert.False(t, cfg.REST.Enabled)
				assert.Empty(t, cfg.REST.Address)
				assert.Zero(t, cfg.REST.Port)
			},
		},
		{
			name: "pre-set server fields are preserved",
			input: &Config{
				Server: ServerConfig{
					Address:  "10.0.0.1",
					Port:     5353,
					CacheTTL: 30 * time.Minute,
				},
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "10.0.0.1", cfg.Server.Address)
				assert.Equal(t, 5353, cfg.Server.Port)
				assert.Equal(t, 30*time.Minute, cfg.Server.CacheTTL)
				// Upstream/Logging still default
				assert.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53"}, cfg.Upstream.Servers)
				assert.Equal(t, 5*time.Second, cfg.Upstream.Timeout)
				assert.Equal(t, "info", cfg.Logging.Level)
			},
		},
		{
			name: "REST defaults only apply when REST.Enabled is already true",
			input: &Config{
				REST: RESTConfig{Enabled: true},
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "0.0.0.0", cfg.REST.Address)
				assert.Equal(t, 8080, cfg.REST.Port)
			},
		},
		{
			name: "non-empty REST address/port untouched when REST.Enabled is false",
			input: &Config{
				REST: RESTConfig{Enabled: false, Address: "192.168.1.1", Port: 9999},
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "192.168.1.1", cfg.REST.Address)
				assert.Equal(t, 9999, cfg.REST.Port)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyDefaults(tt.input)
			tt.check(t, tt.input)
		})
	}
}

func TestLoadConfig_FileNotExist(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))

	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestLoadConfig_InvalidYAMLSyntax(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  address: [unterminated"), 0644))

	cfg, err := LoadConfig(path)

	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing config file")
}

func TestLoadConfig_DurationStringParsing(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name: "valid duration strings parse to exact values",
			yaml: `
server:
  cache_ttl: "1h"
upstream:
  timeout: "30s"
plugins:
  file:
    refresh_interval: "10s"
`,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, time.Hour, cfg.Server.CacheTTL)
				assert.Equal(t, 30*time.Second, cfg.Upstream.Timeout)
				assert.Equal(t, 10*time.Second, cfg.Plugins.File.RefreshInterval)
			},
		},
		// Note: gopkg.in/yaml.v3 special-cases time.Duration fields and runs
		// time.ParseDuration during the direct struct unmarshal (LoadConfig's
		// `yaml.Unmarshal(data, config)` call), which happens before LoadConfig's
		// own "invalid ... duration" validation blocks ever run. So an invalid
		// duration string fails earlier, with the generic parse error below,
		// making those custom messages unreachable dead code for this input shape.
		{
			name: "invalid cache_ttl duration fails at struct unmarshal, not the custom check",
			yaml: `
server:
  cache_ttl: "not-a-duration"
`,
			wantErr: "error parsing config file",
		},
		{
			name: "invalid upstream timeout duration fails at struct unmarshal, not the custom check",
			yaml: `
upstream:
  timeout: "not-a-duration"
`,
			wantErr: "error parsing config file",
		},
		{
			name: "invalid file plugin refresh_interval duration fails at struct unmarshal, not the custom check",
			yaml: `
plugins:
  file:
    refresh_interval: "not-a-duration"
`,
			wantErr: "error parsing config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0644))

			cfg, err := LoadConfig(path)

			if tt.wantErr != "" {
				assert.Nil(t, cfg)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			tt.check(t, cfg)
		})
	}
}

func TestLoadConfig_DefaultsMergedWithPartialOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 9999\n"), 0644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	defaults := CreateDefaultConfig()

	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, defaults.Server.Address, cfg.Server.Address)
	assert.Equal(t, defaults.Server.CacheTTL, cfg.Server.CacheTTL)
	assert.Equal(t, defaults.Upstream.Servers, cfg.Upstream.Servers)
	assert.Equal(t, defaults.Upstream.Timeout, cfg.Upstream.Timeout)
	assert.Equal(t, defaults.Logging, cfg.Logging)
}

func TestLoadConfig_EmptyPathDefaultsToConfigYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("server:\n  port: 1234\n"), 0644))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { require.NoError(t, os.Chdir(origWD)) }()

	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 1234, cfg.Server.Port)
}

func TestSaveConfig_RoundTrip(t *testing.T) {
	cfg := CreateDefaultConfig()
	cfg.Server.Port = 1053
	cfg.Upstream.Servers = []string{"9.9.9.9:53"}

	path := filepath.Join(t.TempDir(), "out.yaml")
	require.NoError(t, SaveConfig(cfg, path))

	loaded, err := LoadConfig(path)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Memory.Entries round-trips as an empty (non-nil) slice rather than nil,
	// which is an irrelevant YAML marshaling detail, so compare it separately.
	assert.Empty(t, loaded.Plugins.Memory.Entries)
	loaded.Plugins.Memory.Entries = cfg.Plugins.Memory.Entries

	assert.Equal(t, cfg, loaded)
	assert.Equal(t, cfg.Server.CacheTTL, loaded.Server.CacheTTL)
	assert.Equal(t, cfg.Upstream.Timeout, loaded.Upstream.Timeout)
}

func TestSaveConfig_WriteError(t *testing.T) {
	cfg := CreateDefaultConfig()

	err := SaveConfig(cfg, filepath.Join(t.TempDir(), "nonexistent-dir", "out.yaml"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error writing config file")
}
