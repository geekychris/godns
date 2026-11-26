package dnssec

import (
	"crypto"
	"fmt"
	"os"
	"strings"

	"github.com/miekg/dns"
)

// GenerateKey generates a new DNSSEC key pair for zone, using the given
// algorithm (one of the dns.RSASHA256/dns.ECDSAP256SHA256/... constants) and
// key size in bits. The returned DNSKEY carries both the ZONE and SEP flags,
// since this package manages a single key pair per zone rather than
// separate KSK/ZSK roles.
func GenerateKey(zone string, algorithm uint8, bits int) (*dns.DNSKEY, crypto.Signer, error) {
	if zone == "" {
		return nil, nil, fmt.Errorf("dnssec: zone must not be empty")
	}

	key := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(zone),
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		Flags:     dns.ZONE | dns.SEP,
		Protocol:  3,
		Algorithm: algorithm,
	}

	priv, err := key.Generate(bits)
	if err != nil {
		return nil, nil, fmt.Errorf("dnssec: generating key for zone %s: %w", zone, err)
	}

	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("dnssec: generated private key for zone %s does not implement crypto.Signer", zone)
	}

	return key, signer, nil
}

// SaveKey writes the public DNSKEY RR and the private key to pathPrefix+".key"
// and pathPrefix+".private" respectively, mirroring BIND's key file layout.
func SaveKey(pathPrefix string, key *dns.DNSKEY, priv crypto.Signer) error {
	if key == nil || priv == nil {
		return fmt.Errorf("dnssec: SaveKey requires a non-nil key and private key")
	}

	pubPath := pathPrefix + ".key"
	privPath := pathPrefix + ".private"

	if err := os.WriteFile(pubPath, []byte(key.String()+"\n"), 0o644); err != nil {
		return fmt.Errorf("dnssec: writing public key file %s: %w", pubPath, err)
	}

	privData := key.PrivateKeyString(priv)
	if err := os.WriteFile(privPath, []byte(privData), 0o600); err != nil {
		return fmt.Errorf("dnssec: writing private key file %s: %w", privPath, err)
	}

	return nil
}

// LoadKey reads the public DNSKEY RR and private key previously written by
// SaveKey from pathPrefix+".key" and pathPrefix+".private".
func LoadKey(pathPrefix string) (*dns.DNSKEY, crypto.Signer, error) {
	pubPath := pathPrefix + ".key"
	privPath := pathPrefix + ".private"

	pubData, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, nil, fmt.Errorf("dnssec: reading public key file %s: %w", pubPath, err)
	}

	rr, err := dns.NewRR(string(pubData))
	if err != nil {
		return nil, nil, fmt.Errorf("dnssec: parsing public key file %s: %w", pubPath, err)
	}
	key, ok := rr.(*dns.DNSKEY)
	if !ok {
		return nil, nil, fmt.Errorf("dnssec: %s does not contain a DNSKEY record", pubPath)
	}

	privData, err := os.ReadFile(privPath)
	if err != nil {
		return nil, nil, fmt.Errorf("dnssec: reading private key file %s: %w", privPath, err)
	}

	priv, err := key.NewPrivateKey(strings.TrimRight(string(privData), "\n") + "\n")
	if err != nil {
		return nil, nil, fmt.Errorf("dnssec: parsing private key file %s: %w", privPath, err)
	}

	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("dnssec: private key in %s does not implement crypto.Signer", privPath)
	}

	return key, signer, nil
}
