package main

import (
	"encoding/json"
	"flag"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseErrorResponse(t *testing.T) {
	t.Run("valid error body", func(t *testing.T) {
		err := parseErrorResponse([]byte(`{"error":"boom"}`))
		require.Error(t, err)
		assert.Equal(t, "server returned an error: boom", err.Error())
	})

	t.Run("malformed body", func(t *testing.T) {
		err := parseErrorResponse([]byte(`not json`))
		require.Error(t, err)
		assert.Equal(t, "server returned an error (failed to parse response)", err.Error())
	})
}

func TestHttpGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	resp, err := httpGet(srv.URL, time.Second)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHttpGet_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	start := time.Now()
	_, err := httpGet(srv.URL, 50*time.Millisecond)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 5*time.Second)
}

func newRESTServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *config) {
	t.Helper()
	srv := httptest.NewServer(handler)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	cfg := &config{
		restServer: host,
		restPort:   port,
		timeout:    2 * time.Second,
	}
	return srv, cfg
}

func TestListCommand(t *testing.T) {
	t.Run("non-empty list", func(t *testing.T) {
		entries := []EntryResponse{
			{Hostname: "a.example.com", IPs: []string{"1.1.1.1"}},
			{Hostname: "b.example.com", IPs: []string{"2.2.2.2", "2.2.2.3"}},
		}
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(entries)
		})
		defer srv.Close()

		err := listCommand(cfg)
		assert.NoError(t, err)
	})

	t.Run("empty list", func(t *testing.T) {
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]EntryResponse{})
		})
		defer srv.Close()

		err := listCommand(cfg)
		assert.NoError(t, err)
	})

	t.Run("server error", func(t *testing.T) {
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "list failed"})
		})
		defer srv.Close()

		err := listCommand(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list failed")
	})
}

func TestGetCommand(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(EntryResponse{Hostname: "found.example.com", IPs: []string{"9.9.9.9"}})
		})
		defer srv.Close()
		cfg.hostname = "found.example.com"

		err := getCommand(cfg)
		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "no such host"})
		})
		defer srv.Close()
		cfg.hostname = "missing.example.com"

		err := getCommand(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such host")
	})
}

func TestAddCommand_InvalidIP(t *testing.T) {
	cfg := &config{
		hostname: "example.com",
		ips:      []string{"not-an-ip"},
		timeout:  time.Second,
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := addCommand(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IP address")
	assert.False(t, called, "addCommand should not make a network call for an invalid IP")
}

func TestAddCommand_Success(t *testing.T) {
	var gotBody EntryRequest
	srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	cfg.hostname = "add.example.com"
	cfg.ips = []string{"10.0.0.1", "10.0.0.2"}

	err := addCommand(cfg)
	require.NoError(t, err)
	assert.Equal(t, "add.example.com", gotBody.Hostname)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, gotBody.IPs)
}

func TestAddCommand_ServerError(t *testing.T) {
	srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "add failed"})
	})
	defer srv.Close()
	cfg.hostname = "add.example.com"
	cfg.ips = []string{"10.0.0.1"}

	err := addCommand(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add failed")
}

func TestDeleteCommand(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			w.WriteHeader(http.StatusNoContent)
		})
		defer srv.Close()
		cfg.hostname = "delete.example.com"

		err := deleteCommand(cfg)
		assert.NoError(t, err)
	})

	t.Run("server error", func(t *testing.T) {
		srv, cfg := newRESTServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "delete failed"})
		})
		defer srv.Close()
		cfg.hostname = "delete.example.com"

		err := deleteCommand(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete failed")
	})
}

// resetFlags installs a fresh ContinueOnError FlagSet as flag.CommandLine and
// sets os.Args so parseFlags() can be exercised repeatedly within one test
// binary without exiting the process.
func resetFlags(t *testing.T, args []string) {
	t.Helper()
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	})

	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	os.Args = args
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCfg func(*testing.T, *config)
	}{
		{
			name: "query with defaults",
			args: []string{"godns-client", "query", "example.com"},
			wantCfg: func(t *testing.T, cfg *config) {
				assert.Equal(t, "query", cfg.command)
				assert.Equal(t, "example.com", cfg.hostname)
				assert.Equal(t, "localhost", cfg.dnsServer)
				assert.Equal(t, 53, cfg.dnsPort)
				assert.Equal(t, 5*time.Second, cfg.timeout)
			},
		},
		{
			name: "list command",
			args: []string{"godns-client", "list"},
			wantCfg: func(t *testing.T, cfg *config) {
				assert.Equal(t, "list", cfg.command)
			},
		},
		{
			name: "add with flags and multiple IPs",
			args: []string{"godns-client", "-dns-port", "5353", "-timeout", "2s", "add", "host.example.com", "1.1.1.1", "2.2.2.2"},
			wantCfg: func(t *testing.T, cfg *config) {
				assert.Equal(t, "add", cfg.command)
				assert.Equal(t, "host.example.com", cfg.hostname)
				assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, cfg.ips)
				assert.Equal(t, 5353, cfg.dnsPort)
				assert.Equal(t, 2*time.Second, cfg.timeout)
			},
		},
		{
			name: "get command",
			args: []string{"godns-client", "get", "host.example.com"},
			wantCfg: func(t *testing.T, cfg *config) {
				assert.Equal(t, "get", cfg.command)
				assert.Equal(t, "host.example.com", cfg.hostname)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags(t, tt.args)
			cfg := parseFlags()
			tt.wantCfg(t, cfg)
		})
	}
}
