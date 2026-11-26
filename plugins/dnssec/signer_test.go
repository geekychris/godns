package dnssec

import (
	"crypto"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeyPair(t *testing.T, zone string) (*dns.DNSKEY, crypto.Signer, *Signer) {
	t.Helper()
	key, priv, err := GenerateKey(zone, dns.ECDSAP256SHA256, 256)
	require.NoError(t, err)
	signer, err := NewSigner(zone, key, priv, time.Hour)
	require.NoError(t, err)
	return key, priv, signer
}

func TestSigner_MatchesZone(t *testing.T) {
	_, _, signer := testKeyPair(t, "example.com")

	tests := []struct {
		name string
		want bool
	}{
		{name: "example.com", want: true},
		{name: "example.com.", want: true},
		{name: "EXAMPLE.COM.", want: true},
		{name: "www.example.com", want: true},
		{name: "other.com", want: false},
		{name: "notexample.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, signer.MatchesZone(tt.name))
		})
	}
}

func TestSigner_SignRRset(t *testing.T) {
	key, _, signer := testKeyPair(t, "example.com")

	tests := []struct {
		name    string
		rrset   []dns.RR
		wantErr bool
	}{
		{
			name: "signs A rrset",
			rrset: []dns.RR{&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   []byte{192, 0, 2, 1},
			}},
		},
		{
			name:  "signs DNSKEY rrset",
			rrset: []dns.RR{key},
		},
		{
			name:    "empty rrset errors",
			rrset:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, err := signer.SignRRset(tt.rrset)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, sig)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, sig)
			assert.Equal(t, tt.rrset[0].Header().Rrtype, sig.TypeCovered)
			assert.Equal(t, key.KeyTag(), sig.KeyTag)
			assert.NoError(t, sig.Verify(signer.DNSKEY(), tt.rrset))
			assert.True(t, sig.ValidityPeriod(time.Now()))
		})
	}
}

func TestSigner_SignRRset_TamperedDataFailsVerify(t *testing.T) {
	_, _, signer := testKeyPair(t, "example.com")

	rrset := []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   []byte{192, 0, 2, 1},
	}}

	sig, err := signer.SignRRset(rrset)
	require.NoError(t, err)

	tampered := []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   []byte{192, 0, 2, 99},
	}}

	assert.Error(t, sig.Verify(signer.DNSKEY(), tampered))
}

func TestNewSigner_RequiresKeyAndPriv(t *testing.T) {
	key, priv, err := GenerateKey("example.com", dns.ECDSAP256SHA256, 256)
	require.NoError(t, err)

	_, err = NewSigner("example.com", nil, priv, 0)
	assert.Error(t, err)

	_, err = NewSigner("example.com", key, nil, 0)
	assert.Error(t, err)

	_, err = NewSigner("", key, priv, 0)
	assert.Error(t, err)
}
