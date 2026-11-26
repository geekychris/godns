package dnssec

import (
	"crypto"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// defaultInceptionSkew backdates RRSIG Inception slightly to tolerate clock
// drift between godns and whatever validates its signatures.
const defaultInceptionSkew = 5 * time.Minute

// Signer signs authoritative RRsets for a single zone with a configured key.
type Signer struct {
	zone         string
	dnskey       *dns.DNSKEY
	priv         crypto.Signer
	signatureTTL time.Duration
}

// NewSigner creates a Signer for zone using the given DNSKEY/private key
// pair. signatureTTL controls how long each generated RRSIG remains valid;
// if zero, it defaults to one hour.
func NewSigner(zone string, key *dns.DNSKEY, priv crypto.Signer, signatureTTL time.Duration) (*Signer, error) {
	if key == nil || priv == nil {
		return nil, fmt.Errorf("dnssec: NewSigner requires a non-nil key and private key")
	}
	if zone == "" {
		return nil, fmt.Errorf("dnssec: NewSigner requires a non-empty zone")
	}
	if signatureTTL == 0 {
		signatureTTL = time.Hour
	}

	return &Signer{
		zone:         strings.ToLower(dns.Fqdn(zone)),
		dnskey:       key,
		priv:         priv,
		signatureTTL: signatureTTL,
	}, nil
}

// Zone returns the fully-qualified, lowercased zone apex this Signer signs for.
func (s *Signer) Zone() string {
	return s.zone
}

// MatchesZone reports whether name is the zone apex or a name under it.
func (s *Signer) MatchesZone(name string) bool {
	name = strings.ToLower(dns.Fqdn(name))
	return name == s.zone || strings.HasSuffix(name, "."+s.zone)
}

// DNSKEY returns the zone's public DNSKEY RR.
func (s *Signer) DNSKEY() *dns.DNSKEY {
	return s.dnskey
}

// SignRRset produces an RRSIG covering rrset. rrset must be non-empty and
// must already be a proper RRset (same name/class/type).
func (s *Signer) SignRRset(rrset []dns.RR) (*dns.RRSIG, error) {
	if len(rrset) == 0 {
		return nil, fmt.Errorf("dnssec: cannot sign an empty rrset")
	}

	now := time.Now().UTC()
	rrsig := &dns.RRSIG{
		Algorithm:  s.dnskey.Algorithm,
		SignerName: s.zone,
		KeyTag:     s.dnskey.KeyTag(),
		Inception:  uint32(now.Add(-defaultInceptionSkew).Unix()),
		Expiration: uint32(now.Add(s.signatureTTL).Unix()),
	}

	if err := rrsig.Sign(s.priv, rrset); err != nil {
		return nil, fmt.Errorf("dnssec: signing rrset for %s: %w", rrset[0].Header().Name, err)
	}

	return rrsig, nil
}
