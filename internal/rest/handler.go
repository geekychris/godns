package rest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/yourusername/godns/pkg/api"
)

// Handler implements the REST API handlers
type Handler struct {
	logger       *logrus.Logger
	memoryPlugin api.MemoryPlugin
}

// EntryRequest represents a request to add or update a DNS entry
type EntryRequest struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}

// EntryResponse represents a DNS entry response
type EntryResponse struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// NewHandler creates a new handler
func NewHandler(logger *logrus.Logger, memoryPlugin api.MemoryPlugin) *Handler {
	return &Handler{
		logger:       logger,
		memoryPlugin: memoryPlugin,
	}
}

// RegisterRoutes registers the API routes with the router
func (h *Handler) RegisterRoutes(router *mux.Router) {
	h.logger.Debug("Registering REST API routes")
	
	// API routes
	apiRouter := router.PathPrefix("/api").Subrouter()
	
	// Entries endpoints
	entriesRouter := apiRouter.PathPrefix("/entries").Subrouter()
	entriesRouter.HandleFunc("", h.listEntries).Methods(http.MethodGet)
	entriesRouter.HandleFunc("", h.addOrUpdateEntry).Methods(http.MethodPut)
	entriesRouter.HandleFunc("/{hostname}", h.getEntry).Methods(http.MethodGet)
	entriesRouter.HandleFunc("/{hostname}", h.deleteEntry).Methods(http.MethodDelete)
	
	// DNS resolution endpoint
	apiRouter.HandleFunc("/resolve/{hostname}", h.resolveDNS).Methods(http.MethodGet)
	
	// Add health check endpoint
	apiRouter.HandleFunc("/health", h.healthCheck).Methods(http.MethodGet)
	
	h.logger.Debug("REST API routes registered successfully")
}

// listEntries lists all DNS entries
func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("REST API: List all entries")
	
	if h.memoryPlugin == nil {
		h.respondWithError(w, http.StatusInternalServerError, "Memory plugin not available")
		return
	}
	
	hostnames := h.memoryPlugin.ListEntries()
	entries := make([]EntryResponse, 0, len(hostnames))
	
	for _, hostname := range hostnames {
		ips, err := h.memoryPlugin.GetEntry(hostname)
		if err != nil {
			continue
		}
		
		ipStrs := make([]string, len(ips))
		for i, ip := range ips {
			ipStrs[i] = ip.String()
		}
		
		entries = append(entries, EntryResponse{
			Hostname: hostname,
			IPs:      ipStrs,
		})
	}
	
	h.respondWithJSON(w, http.StatusOK, entries)
}

// getEntry gets a specific DNS entry by hostname
func (h *Handler) getEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]
	hostname = strings.ToLower(hostname)
	
	h.logger.Debugf("REST API: Get entry for hostname: %s", hostname)
	
	if h.memoryPlugin == nil {
		h.respondWithError(w, http.StatusInternalServerError, "Memory plugin not available")
		return
	}
	
	ips, err := h.memoryPlugin.GetEntry(hostname)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, fmt.Sprintf("Hostname not found: %s", hostname))
		return
	}
	
	ipStrs := make([]string, len(ips))
	for i, ip := range ips {
		ipStrs[i] = ip.String()
	}
	
	entry := EntryResponse{
		Hostname: hostname,
		IPs:      ipStrs,
	}
	
	h.respondWithJSON(w, http.StatusOK, entry)
}

// addOrUpdateEntry adds or updates a DNS entry
func (h *Handler) addOrUpdateEntry(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("REST API: Add/update entry")
	
	if h.memoryPlugin == nil {
		h.respondWithError(w, http.StatusInternalServerError, "Memory plugin not available")
		return
	}
	
	var req EntryRequest
	
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()
	
	// Validate request
	if req.Hostname == "" {
		h.respondWithError(w, http.StatusBadRequest, "Hostname is required")
		return
	}
	
	if len(req.IPs) == 0 {
		h.respondWithError(w, http.StatusBadRequest, "At least one IP address is required")
		return
	}
	
	// Convert string IPs to net.IP
	var ips []net.IP
	for _, ipStr := range req.IPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			h.respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid IP address: %s", ipStr))
			return
		}
		ips = append(ips, ip)
	}
	
	// Normalize hostname
	req.Hostname = strings.ToLower(req.Hostname)
	
	h.logger.Infof("REST API: Adding/updating entry for hostname: %s with IPs: %v", req.Hostname, req.IPs)
	
	// Add or update the entry
	if err := h.memoryPlugin.AddEntry(req.Hostname, ips); err != nil {
		h.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add entry: %v", err))
		return
	}
	
	h.respondWithJSON(w, http.StatusCreated, EntryResponse{
		Hostname: req.Hostname,
		IPs:      req.IPs,
	})
}

// deleteEntry deletes a DNS entry by hostname
func (h *Handler) deleteEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]
	hostname = strings.ToLower(hostname)
	
	h.logger.Infof("REST API: Deleting entry for hostname: %s", hostname)
	
	if h.memoryPlugin == nil {
		h.respondWithError(w, http.StatusInternalServerError, "Memory plugin not available")
		return
	}
	
	if err := h.memoryPlugin.DeleteEntry(hostname); err != nil {
		h.respondWithError(w, http.StatusNotFound, fmt.Sprintf("Hostname not found: %s", hostname))
		return
	}
	
	// Return 204 No Content status
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("REST API: Health check")
	h.respondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// respondWithError responds with an error message
func (h *Handler) respondWithError(w http.ResponseWriter, code int, message string) {
	h.logger.Warnf("REST API Error: %s (code: %d)", message, code)
	h.respondWithJSON(w, code, ErrorResponse{Error: message})
}

// respondWithJSON responds with a JSON payload
func (h *Handler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		h.logger.Errorf("Failed to marshal JSON response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// resolveDNS resolves a DNS hostname
func (h *Handler) resolveDNS(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]
	hostname = strings.ToLower(hostname)
	
	h.logger.Debugf("REST API: Resolve hostname: %s", hostname)
	
	if h.memoryPlugin == nil {
		h.respondWithError(w, http.StatusInternalServerError, "Memory plugin not available")
		return
	}
	
	ips, err := h.memoryPlugin.Resolve(hostname)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, fmt.Sprintf("hostname %s not found", hostname))
		return
	}
	
	ipStrs := make([]string, len(ips))
	for i, ip := range ips {
		ipStrs[i] = ip.String()
	}
	
	response := EntryResponse{
		Hostname: hostname,
		IPs:      ipStrs,
	}
	
	h.respondWithJSON(w, http.StatusOK, response)
}
