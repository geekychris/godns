package server

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBlackHoleUDPListener opens a UDP socket that reads datagrams and never
// replies, simulating an upstream DNS server that hangs.
func newBlackHoleUDPListener(t *testing.T) (addr string, stop func()) {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1500)
		for {
			select {
			case <-done:
				return
			default:
			}
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			// Intentionally never respond.
		}
	}()

	return conn.LocalAddr().String(), func() {
		close(done)
		conn.Close()
	}
}

// newUDPDNSResponder starts a minimal UDP DNS server that always answers
// queries for `hostname` with `ip`.
func newUDPDNSResponder(t *testing.T, hostname string, ip net.IP) (addr string, stop func()) {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1500)
		for {
			select {
			case <-done:
				return
			default:
			}
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			req := new(dns.Msg)
			if err := req.Unpack(buf[:n]); err != nil {
				continue
			}

			resp := new(dns.Msg)
			resp.SetReply(req)
			resp.Rcode = dns.RcodeSuccess

			if len(req.Question) > 0 {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   req.Question[0].Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: ip,
				}
				resp.Answer = append(resp.Answer, rr)
			}

			out, err := resp.Pack()
			if err != nil {
				continue
			}
			conn.WriteToUDP(out, raddr)
		}
	}()

	return conn.LocalAddr().String(), func() {
		close(done)
		conn.Close()
	}
}

func TestForwardQuery_HonorsConfiguredTimeout(t *testing.T) {
	addr, stop := newBlackHoleUDPListener(t)
	defer stop()

	cfg := createTestConfig()
	cfg.Upstream.Servers = []string{addr}
	cfg.Upstream.Timeout = 200 * time.Millisecond

	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	start := time.Now()
	_, err := server.forwardQuery("example.com")
	elapsed := time.Since(start)

	assert.Error(t, err)
	// Before the fix, Timeout was multiplied by time.Second again, inflating
	// a 200ms timeout to ~158 years. Bound generously to catch regressions.
	assert.Less(t, elapsed, 5*time.Second, "forwardQuery should respect the configured timeout")
}

func TestForwardQueryRaw_HonorsConfiguredTimeout(t *testing.T) {
	addr, stop := newBlackHoleUDPListener(t)
	defer stop()

	cfg := createTestConfig()
	cfg.Upstream.Servers = []string{addr}
	cfg.Upstream.Timeout = 200 * time.Millisecond

	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com"), dns.TypeA)

	start := time.Now()
	_, err := server.forwardQueryRaw(m)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Less(t, elapsed, 5*time.Second, "forwardQueryRaw should respect the configured timeout")
}

func TestForwardQuery_Success(t *testing.T) {
	hostname := "example.com"
	expectedIP := net.ParseIP("203.0.113.42").To4()

	addr, stop := newUDPDNSResponder(t, hostname, expectedIP)
	defer stop()

	cfg := createTestConfig()
	cfg.Upstream.Servers = []string{addr}
	cfg.Upstream.Timeout = 2 * time.Second

	logger := createTestLogger()
	server := NewDNSServer(cfg, logger)

	ips, err := server.forwardQuery(hostname)
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, expectedIP.String(), ips[0].String())
}
