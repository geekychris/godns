package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/yourusername/godns/internal/config"
	"github.com/yourusername/godns/plugins/memory"
)

// Server represents the REST API server interface
type Server interface {
	Start() error
	Stop() error
}

// RESTServer implements the Server interface
type RESTServer struct {
	config       *config.Config
	logger       *logrus.Logger
	memoryPlugin *memory.MemoryPlugin
	httpServer   *http.Server
	router       *mux.Router
}

// NewServer creates a new REST API server
func NewServer(cfg *config.Config, logger *logrus.Logger, memoryPlugin *memory.MemoryPlugin) Server {
	logger.Debug("Creating new REST API server")
	
	router := mux.NewRouter()
	
	server := &RESTServer{
		config:       cfg,
		logger:       logger,
		memoryPlugin: memoryPlugin,
		router:       router,
	}
	
	// Create and register the handler
	handler := NewHandler(logger, memoryPlugin)
	handler.RegisterRoutes(router)
	
	// Set up middleware
	router.Use(server.loggingMiddleware)
	
	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.REST.Address, cfg.REST.Port)
	server.httpServer = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	// Verify memory plugin is available
	if memoryPlugin == nil {
		logger.Warn("Memory plugin is nil, REST API will have limited functionality")
	} else {
		logger.Debug("Memory plugin is available for REST API")
	}
	
	logger.Debugf("REST API server initialized with address: %s", addr)
	
	return server
}

// Start starts the REST API server
func (s *RESTServer) Start() error {
	if !s.config.REST.Enabled {
		s.logger.Info("REST API is disabled, not starting server")
		return nil
	}
	
	if s.httpServer == nil {
		return fmt.Errorf("HTTP server not initialized")
	}
	
	s.logger.Infof("Starting REST API server on %s", s.httpServer.Addr)
	
	// Start the server
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Errorf("Failed to start REST API server: %v", err)
		return fmt.Errorf("error starting REST API server: %v", err)
	}
	
	return nil
}

// Stop stops the REST API server
func (s *RESTServer) Stop() error {
	if s.httpServer != nil {
		s.logger.Info("Stopping REST API server")
		
		// Create a deadline to wait for current connections to complete
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		return s.httpServer.Shutdown(ctx)
	}
	
	return nil
}

// loggingMiddleware logs HTTP requests
func (s *RESTServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Call the next handler
		next.ServeHTTP(w, r)
		
		// Log request details
		s.logger.WithFields(logrus.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"remote":   r.RemoteAddr,
			"duration": time.Since(start),
		}).Debug("REST API request")
	})
}

