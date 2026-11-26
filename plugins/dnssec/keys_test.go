package dnssec

import (
	"crypto"
	"path/filepath"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name      string
		zone      string
		algorithm uint8
		bits      int
		wantErr   bool
	}{
		{name: "ecdsa p256", zone: "example.com", algorithm: dns.ECDSAP256SHA256, bits: 256},
		{name: "rsa sha256", zone: "example.com", algorithm: dns.RSASHA256, bits: 1024},
		{name: "ed25519", zone: "example.com", algorithm: dns.ED25519, bits: 256},
		{name: "empty zone fails", zone: "", algorithm: dns.ECDSAP256SHA256, bits: 256, wantErr: true},
		{name: "invalid algorithm fails", zone: "example.com", algorithm: 0, bits: 256, wantErr: true},
		{name: "invalid ecdsa bit size fails", zone: "example.com", algorithm: dns.ECDSAP256SHA256, bits: 1024, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, priv, err := GenerateKey(tt.zone, tt.algorithm, tt.bits)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, key)
				assert.Nil(t, priv)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, key)
			require.NotNil(t, priv)
			assert.Equal(t, dns.Fqdn(tt.zone), key.Hdr.Name)
			assert.Equal(t, tt.algorithm, key.Algorithm)
			assert.NotEmpty(t, key.PublicKey)
			var _ crypto.Signer = priv
		})
	}
}

func TestSaveLoadKeyRoundTrip(t *testing.T) {
	key, priv, err := GenerateKey("example.com", dns.ECDSAP256SHA256, 256)
	require.NoError(t, err)

	pathPrefix := filepath.Join(t.TempDir(), "Kexample.com")
	require.NoError(t, SaveKey(pathPrefix, key, priv))

	loadedKey, loadedPriv, err := LoadKey(pathPrefix)
	require.NoError(t, err)
	require.NotNil(t, loadedKey)
	require.NotNil(t, loadedPriv)

	assert.Equal(t, key.Hdr.Name, loadedKey.Hdr.Name)
	assert.Equal(t, key.Algorithm, loadedKey.Algorithm)
	assert.Equal(t, key.PublicKey, loadedKey.PublicKey)
	assert.Equal(t, key.KeyTag(), loadedKey.KeyTag())

	// A signature made with the original key must verify against the
	// reloaded public key, and a signature made with the reloaded private
	// key must verify against the original public key — proving the
	// on-disk round trip preserves a usable key pair.
	rrset := []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   []byte{192, 0, 2, 1},
	}}

	signer, err := NewSigner("example.com", key, priv, 0)
	require.NoError(t, err)
	sig, err := signer.SignRRset(rrset)
	require.NoError(t, err)
	assert.NoError(t, sig.Verify(loadedKey, rrset))

	reloadedSigner, err := NewSigner("example.com", loadedKey, loadedPriv, 0)
	require.NoError(t, err)
	sig2, err := reloadedSigner.SignRRset(rrset)
	require.NoError(t, err)
	assert.NoError(t, sig2.Verify(key, rrset))
}

func TestLoadKey_MissingFiles(t *testing.T) {
	_, _, err := LoadKey(filepath.Join(t.TempDir(), "nonexistent"))
	require.Error(t, err)
}
