package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/miekg/dns"

	"github.com/yourusername/godns/internal/config"
	"github.com/yourusername/godns/internal/rest"
	"github.com/yourusername/godns/internal/server"
	"github.com/yourusername/godns/plugins/dnssec"
	"github.com/yourusername/godns/plugins/file"
	"github.com/yourusername/godns/plugins/memory"
)

var (
	configPath string
	logger     *logrus.Logger
)

func main() {
	// Parse command line flags
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Initialize logger
	logger = logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	})

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Configure logger based on config
	logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		logger.Warnf("Invalid log level %s, defaulting to info", cfg.Logging.Level)
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	if cfg.Logging.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

	logger.Info("Starting GoDNS server")

	// Create DNS server but don't start it yet
	dnsServer := server.NewDNSServer(cfg, logger)

	// Initialize plugins once to get the memory plugin for REST API
	var memoryPlugin *memory.MemoryPlugin
	if err := registerPlugins(dnsServer, cfg, &memoryPlugin); err != nil {
		logger.Fatalf("Failed to register plugins: %v", err)
	}

	if err := initDNSSEC(dnsServer, cfg); err != nil {
		logger.Fatalf("Failed to initialize DNSSEC: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start REST API server if enabled (before DNS server)
	var restServer rest.Server
	if cfg.REST.Enabled {
		logger.Info("Initializing REST API server")
		if memoryPlugin == nil {
			logger.Warn("REST API is enabled but memory plugin is not available")
		} else {
			logger.Debug("Memory plugin available for REST API")
		}

		// Create REST server
		restServer = rest.NewServer(cfg, logger, memoryPlugin, dnsServer.Metrics())
		logger.Infof("REST API server configured on %s:%d", cfg.REST.Address, cfg.REST.Port)

		// Set REST server in DNS server for clean shutdown
		dnsServer.SetRESTServer(restServer)

		// Start REST server in a goroutine
		restErrChan := make(chan error, 1)
		go func() {
			logger.Debug("Starting REST API server goroutine")
			if err := restServer.Start(); err != nil {
				logger.Errorf("REST API server error: %v", err)
				restErrChan <- err
			}
			logger.Debug("REST API server goroutine exited")
		}()

		// Wait briefly to ensure REST server starts
		select {
		case err := <-restErrChan:
			logger.Fatalf("Failed to start REST API server: %v", err)
		case <-time.After(1 * time.Second):
			logger.Info("REST API server started successfully")
		}
	} else {
		logger.Info("REST API is disabled in configuration")
	}

	// Start the DNS server in a goroutine
	dnsErrChan := make(chan error, 1)
	go func() {
		logger.Debug("Starting DNS server goroutine")
		if err := dnsServer.Start(); err != nil {
			logger.Errorf("DNS server error: %v", err)
			dnsErrChan <- err
		}
		logger.Debug("DNS server goroutine exited")
	}()

	// Wait for either server error or termination signal
	select {
	case err := <-dnsErrChan:
		logger.Fatalf("Failed to start DNS server: %v", err)
	case sig := <-sigChan:
		logger.Infof("Received signal %v, shutting down", sig)
	}

	// Stop the DNS server (this will also stop the REST server if it was set)
	if err := dnsServer.Stop(); err != nil {
		logger.Errorf("Error stopping servers: %v", err)
	}

	logger.Info("Server shutdown complete")
}

// registerPlugins registers all enabled plugins with the DNS server and returns the memory plugin if requested
// If memoryPluginOut is non-nil, the memory plugin will be assigned to it for use with the REST API
func registerPlugins(dnsServer *server.DNSServer, cfg *config.Config, memoryPluginOut **memory.MemoryPlugin) error {
	outputMode := memoryPluginOut != nil

	// Register file plugin if enabled
	if cfg.Plugins.File.Enabled {
		logger.Info("Initializing file plugin")
		filePlugin, err := file.NewFilePlugin(map[string]interface{}{
			"path":             cfg.Plugins.File.Path,
			"refresh_interval": cfg.Plugins.File.RefreshInterval,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize file plugin: %v", err)
		}

		// Register with DNS server
		dnsServer.RegisterResolver(filePlugin)
	}

	// Register memory plugin if enabled
	if cfg.Plugins.Memory.Enabled {
		logger.Info("Initializing memory plugin")
		memoryPlugin, err := memory.NewMemoryPlugin(map[string]interface{}{
			"entries": cfg.Plugins.Memory.Entries,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize memory plugin: %v", err)
		}

		// Register with DNS server
		dnsServer.RegisterResolver(memoryPlugin)

		// Expose memory plugin for REST API if output pointer is provided
		if outputMode {
			// Check if memoryPlugin can be cast to *memory.MemoryPlugin
			if mp, ok := memoryPlugin.(*memory.MemoryPlugin); ok {
				*memoryPluginOut = mp
				logger.Debug("Memory plugin exposed for REST API use")
			} else {
				logger.Warn("Could not cast memory plugin to expected type")
			}
		}
	}

	return nil
}

// initDNSSEC wires up DNSSEC signing and/or validation on dnsServer
// according to cfg.DNSSEC. Both halves are independently opt-in; leaving
// either disabled (the default) leaves the corresponding server hook nil.
func initDNSSEC(dnsServer *server.DNSServer, cfg *config.Config) error {
	if cfg.DNSSEC.Signing.Enabled {
		logger.Infof("Initializing DNSSEC signing for zone %s", cfg.DNSSEC.Signing.Zone)

		key, priv, err := dnssec.LoadKey(cfg.DNSSEC.Signing.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load DNSSEC signing key: %w", err)
		}

		signer, err := dnssec.NewSigner(cfg.DNSSEC.Signing.Zone, key, priv, cfg.DNSSEC.Signing.SignatureTTL)
		if err != nil {
			return fmt.Errorf("failed to initialize DNSSEC signer: %w", err)
		}

		dnsServer.SetSigner(signer)
	}

	if cfg.DNSSEC.Validation.Enabled {
		logger.Info("Initializing DNSSEC validation of upstream responses")

		if len(cfg.DNSSEC.Validation.TrustedKeyFiles) == 0 {
			return fmt.Errorf("dnssec validation is enabled but no trusted_key_files are configured")
		}

		var trustedKeys []*dns.DNSKEY
		for _, keyFile := range cfg.DNSSEC.Validation.TrustedKeyFiles {
			data, err := os.ReadFile(keyFile)
			if err != nil {
				return fmt.Errorf("failed to read trusted DNSKEY file %s: %w", keyFile, err)
			}

			rr, err := dns.NewRR(string(data))
			if err != nil {
				return fmt.Errorf("failed to parse trusted DNSKEY file %s: %w", keyFile, err)
			}

			key, ok := rr.(*dns.DNSKEY)
			if !ok {
				return fmt.Errorf("trusted key file %s does not contain a DNSKEY record", keyFile)
			}

			trustedKeys = append(trustedKeys, key)
		}

		validator, err := dnssec.NewValidator(trustedKeys)
		if err != nil {
			return fmt.Errorf("failed to initialize DNSSEC validator: %w", err)
		}

		dnsServer.SetValidator(validator)
	}

	return nil
}
