package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/godns/internal/config"
	"github.com/yourusername/godns/internal/server"
	"github.com/yourusername/godns/plugins/memory"
)

func TestRegisterPlugins(t *testing.T) {
	logger = logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	hostsFile := filepath.Join(t.TempDir(), "hosts")
	require.NoError(t, os.WriteFile(hostsFile, []byte("127.0.0.1 test.local\n"), 0644))

	tests := []struct {
		name          string
		cfg           *config.Config
		withOutputPtr bool
		wantErr       string
		wantCount     int
		wantMemoryOut bool
	}{
		{
			name:      "no plugins enabled",
			cfg:       &config.Config{},
			wantCount: 0,
		},
		{
			name: "file plugin enabled with valid hosts file",
			cfg: &config.Config{
				Plugins: config.PluginsConfig{
					File: config.FilePluginConfig{Enabled: true, Path: hostsFile, RefreshInterval: 60 * time.Second},
				},
			},
			wantCount: 1,
		},
		{
			name: "file plugin enabled with nonexistent path",
			cfg: &config.Config{
				Plugins: config.PluginsConfig{
					File: config.FilePluginConfig{Enabled: true, Path: filepath.Join(t.TempDir(), "missing"), RefreshInterval: 60 * time.Second},
				},
			},
			wantErr:   "failed to initialize file plugin",
			wantCount: 0,
		},
		{
			name: "memory plugin enabled",
			cfg: &config.Config{
				Plugins: config.PluginsConfig{
					Memory: config.MemoryPluginConfig{Enabled: true},
				},
			},
			withOutputPtr: true,
			wantCount:     1,
			wantMemoryOut: true,
		},
		{
			name: "both plugins enabled",
			cfg: &config.Config{
				Plugins: config.PluginsConfig{
					File:   config.FilePluginConfig{Enabled: true, Path: hostsFile, RefreshInterval: 60 * time.Second},
					Memory: config.MemoryPluginConfig{Enabled: true},
				},
			},
			wantCount: 2,
		},
		{
			name: "memory plugin enabled with nil memoryPluginOut",
			cfg: &config.Config{
				Plugins: config.PluginsConfig{
					Memory: config.MemoryPluginConfig{Enabled: true},
				},
			},
			withOutputPtr: false,
			wantCount:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dnsServer := server.NewDNSServer(tt.cfg, logger)

			var memoryPlugin *memory.MemoryPlugin
			var memoryPluginOut **memory.MemoryPlugin
			if tt.withOutputPtr {
				memoryPluginOut = &memoryPlugin
			}

			err := registerPlugins(dnsServer, tt.cfg, memoryPluginOut)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantCount, dnsServer.ResolverCount())

			if tt.wantMemoryOut {
				assert.NotNil(t, memoryPlugin)
			} else if tt.withOutputPtr {
				assert.Nil(t, memoryPlugin)
			}
		})
	}
}
