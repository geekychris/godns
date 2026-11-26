// Package dnssec provides opt-in DNSSEC zone signing and upstream response
// validation for godns. Signing and validation are independent of each
// other and independent of the plugin resolver pipeline: a Signer adds
// RRSIGs to authoritative answers produced by godns' own resolvers, and a
// Validator checks RRSIGs on responses received from upstream servers.
package dnssec

import "errors"

// ErrValidationFailed is returned (wrapped) by Validator.ValidateMsg when a
// response fails DNSSEC validation: a covering RRSIG is missing, the
// signature does not verify against any trusted key, or the signature is
// outside its validity period. Callers should map this to SERVFAIL.
var ErrValidationFailed = errors.New("dnssec: response failed DNSSEC validation")
