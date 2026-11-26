package dnssec

import (
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Validator checks DNSSEC signatures on DNS responses against a locally
// pinned set of trusted DNSKEY records. It does not walk a chain of trust
// (no DS records, no parent-zone delegation, no root anchor) — only RRsets
// signed directly by one of the trusted keys are accepted.
type Validator struct {
	trusted map[string][]*dns.DNSKEY // keyed by lowercased, fully-qualified owner name
}

// NewValidator creates a Validator trusting exactly the given DNSKEY RRs.
func NewValidator(trustedKeys []*dns.DNSKEY) (*Validator, error) {
	if len(trustedKeys) == 0 {
		return nil, fmt.Errorf("dnssec: NewValidator requires at least one trusted key")
	}

	trusted := make(map[string][]*dns.DNSKEY)
	for _, k := range trustedKeys {
		if k == nil {
			continue
		}
		name := strings.ToLower(dns.Fqdn(k.Hdr.Name))
		trusted[name] = append(trusted[name], k)
	}

	return &Validator{trusted: trusted}, nil
}

// rrsetKey identifies one RRset within a message by owner name and type.
type rrsetKey struct {
	name  string
	rtype uint16
}

// ValidateMsg checks that every RRset in msg.Answer carries a covering
// RRSIG that verifies against a trusted DNSKEY and is within its validity
// period. A message with no answers passes trivially (there is nothing to
// validate). Any failure is returned wrapping ErrValidationFailed.
func (v *Validator) ValidateMsg(msg *dns.Msg) error {
	if msg == nil {
		return fmt.Errorf("%w: nil message", ErrValidationFailed)
	}

	groups := make(map[rrsetKey][]dns.RR)
	var rrsigs []*dns.RRSIG

	for _, rr := range msg.Answer {
		if sig, ok := rr.(*dns.RRSIG); ok {
			rrsigs = append(rrsigs, sig)
			continue
		}
		key := rrsetKey{name: strings.ToLower(rr.Header().Name), rtype: rr.Header().Rrtype}
		groups[key] = append(groups[key], rr)
	}

	for key, rrset := range groups {
		if err := v.validateRRset(key, rrset, rrsigs); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateRRset(key rrsetKey, rrset []dns.RR, rrsigs []*dns.RRSIG) error {
	for _, sig := range rrsigs {
		if sig.TypeCovered != key.rtype || strings.ToLower(sig.Hdr.Name) != key.name {
			continue
		}

		signerName := strings.ToLower(dns.Fqdn(sig.SignerName))
		for _, trustedKey := range v.trusted[signerName] {
			if sig.KeyTag != trustedKey.KeyTag() {
				continue
			}
			if err := sig.Verify(trustedKey, rrset); err != nil {
				continue
			}
			if !sig.ValidityPeriod(time.Now()) {
				continue
			}
			return nil
		}
	}

	return fmt.Errorf("%w: no trusted RRSIG verifies %s %s", ErrValidationFailed, key.name, dns.TypeToString[key.rtype])
}
