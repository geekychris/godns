package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

)

// Response/Request types for testing
type AddEntryRequest struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}

type ResolveResponse struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}
// MockMemoryPlugin mocks the memory plugin for testing
type MockMemoryPlugin struct {
	mock.Mock
}

// Override methods to use mocking
func (m *MockMemoryPlugin) GetEntry(hostname string) ([]net.IP, error) {
	args := m.Called(hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]net.IP), args.Error(1)
}

func (m *MockMemoryPlugin) AddEntry(hostname string, ips []net.IP) error {
	args := m.Called(hostname, ips)
	return args.Error(0)
}

func (m *MockMemoryPlugin) DeleteEntry(hostname string) error {
	args := m.Called(hostname)
	return args.Error(0)
}

func (m *MockMemoryPlugin) ListEntries() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *MockMemoryPlugin) Name() string {
	return "mock-memory"
}

func (m *MockMemoryPlugin) CanResolve(hostname string) bool {
	args := m.Called(hostname)
	return args.Bool(0)
}

func (m *MockMemoryPlugin) Resolve(hostname string) ([]net.IP, error) {
	args := m.Called(hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]net.IP), args.Error(1)
}

func (m *MockMemoryPlugin) Init(config map[string]interface{}) error {
	return nil
}

// TestListEntries tests the listing of entries via REST API
func TestListEntries(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostnames := []string{"test1.local", "test2.local"}
	ips := []net.IP{net.ParseIP("192.168.1.1")}

	// Mock both ListEntries and GetEntry calls
	mockPlugin.On("ListEntries").Return(hostnames)
	for _, hostname := range hostnames {
		mockPlugin.On("GetEntry", hostname).Return(ips, nil)
	}

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("GET", "/api/entries", nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp []EntryResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp, 2)
	assert.Contains(t, hostnames, resp[0].Hostname)
	assert.Contains(t, hostnames, resp[1].Hostname)

	mockPlugin.AssertExpectations(t)
}

// TestGetEntry tests retrieving a specific entry via REST API
func TestGetEntry(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "test1.local"
	ips := []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.11")}
	mockPlugin.On("GetEntry", hostname).Return(ips, nil)

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("GET", "/api/entries/"+hostname, nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp EntryResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, hostname, resp.Hostname)
	assert.Len(t, resp.IPs, 2)
	assert.Contains(t, resp.IPs, "192.168.1.10")
	assert.Contains(t, resp.IPs, "192.168.1.11")

	mockPlugin.AssertExpectations(t)
}

// TestGetEntryNotFound tests getting a non-existent entry
func TestGetEntryNotFound(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "nonexistent.local"
	mockPlugin.On("GetEntry", hostname).Return([]net.IP{}, fmt.Errorf("entry not found"))

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("GET", "/api/entries/"+hostname, nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var resp ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "not found")

	mockPlugin.AssertExpectations(t)
}

// TestAddEntry tests adding a new entry via REST API
func TestAddEntry(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "new.local"
	ips := []string{"192.168.1.20", "192.168.1.21"}

	// Expected net.IP values
	netIPs := []net.IP{net.ParseIP("192.168.1.20"), net.ParseIP("192.168.1.21")}
	mockPlugin.On("AddEntry", hostname, mock.MatchedBy(func(args []net.IP) bool {
		// Check if the IPs match (comparing string representations)
		if len(args) != len(netIPs) {
			return false
		}
		for i, ip := range args {
			if ip.String() != netIPs[i].String() {
				return false
			}
		}
		return true
	})).Return(nil)

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create request payload
	payload := AddEntryRequest{
		Hostname: hostname,
		IPs:      ips,
	}
	jsonPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	// Create test request
	req := httptest.NewRequest("PUT", "/api/entries", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusCreated, rr.Code)

	mockPlugin.AssertExpectations(t)
}

// TestAddEntryInvalidRequest tests adding an entry with invalid request data
func TestAddEntryInvalidRequest(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	testCases := []struct {
		name    string
		payload AddEntryRequest
	}{
		{
			name: "missing hostname",
			payload: AddEntryRequest{
				Hostname: "",
				IPs:      []string{"192.168.1.1"},
			},
		},
		{
			name: "missing IPs",
			payload: AddEntryRequest{
				Hostname: "test.local",
				IPs:      []string{},
			},
		},
		{
			name: "invalid IP",
			payload: AddEntryRequest{
				Hostname: "test.local",
				IPs:      []string{"invalid-ip"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonPayload, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			// Create test request
			req := httptest.NewRequest("PUT", "/api/entries", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Handle request
			router.ServeHTTP(rr, req)

			// Check response
			assert.Equal(t, http.StatusBadRequest, rr.Code)

			var resp ErrorResponse
			err = json.Unmarshal(rr.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.NotEmpty(t, resp.Error)
		})
	}
}

// TestDeleteEntry tests deleting an entry via REST API
func TestDeleteEntry(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "delete.local"
	mockPlugin.On("DeleteEntry", hostname).Return(nil)

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("DELETE", "/api/entries/"+hostname, nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusNoContent, rr.Code)

	mockPlugin.AssertExpectations(t)
}

// TestDeleteEntryNotFound tests deleting a non-existent entry
func TestDeleteEntryNotFound(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "nonexistent.local"
	mockPlugin.On("DeleteEntry", hostname).Return(fmt.Errorf("entry not found"))

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("DELETE", "/api/entries/"+hostname, nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var resp ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "not found")

	mockPlugin.AssertExpectations(t)
}

// TestResolveDNS tests the DNS resolution endpoint
func TestResolveDNS(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "resolve.local"
	ips := []net.IP{net.ParseIP("192.168.1.30")}
	mockPlugin.On("Resolve", hostname).Return(ips, nil)

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("GET", "/api/resolve/"+hostname, nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ResolveResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, hostname, resp.Hostname)
	assert.Len(t, resp.IPs, 1)
	assert.Equal(t, "192.168.1.30", resp.IPs[0])

	mockPlugin.AssertExpectations(t)
}

// TestResolveDNSNotFound tests resolving a non-existent hostname
func TestResolveDNSNotFound(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Set up test data
	hostname := "nonexistent.local"
	mockPlugin.On("Resolve", hostname).Return(nil, fmt.Errorf("hostname not found"))

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("GET", "/api/resolve/"+hostname, nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var resp ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "not found")

	mockPlugin.AssertExpectations(t)
}

// TestHealthCheck tests the health check endpoint
func TestHealthCheck(t *testing.T) {
	// Set up logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create mock plugin
	mockPlugin := new(MockMemoryPlugin)

	// Create test server
	handler := NewHandler(logger, mockPlugin)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// Create test request
	req := httptest.NewRequest("GET", "/api/health", nil)
	rr := httptest.NewRecorder()

	// Handle request
	router.ServeHTTP(rr, req)

	// Check response
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}
