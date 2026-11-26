package dnssec

import (
	"errors"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_ValidateMsg(t *testing.T) {
	trustedKey, trustedPriv, trustedSigner := testKeyPair(t, "example.com")
	_, _, otherSigner := testKeyPair(t, "example.com") // same zone, different (untrusted) key pair

	aRecord := func() dns.RR {
		return &dns.A{
			Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
			A:   []byte{192, 0, 2, 1},
		}
	}

	validator, err := NewValidator([]*dns.DNSKEY{trustedKey})
	require.NoError(t, err)

	tests := []struct {
		name     string
		buildMsg func(t *testing.T) *dns.Msg
		wantErr  bool
	}{
		{
			name: "valid signature from a trusted key is accepted",
			buildMsg: func(t *testing.T) *dns.Msg {
				a := aRecord()
				sig, err := trustedSigner.SignRRset([]dns.RR{a})
				require.NoError(t, err)
				msg := new(dns.Msg)
				msg.Answer = []dns.RR{a, sig}
				return msg
			},
			wantErr: false,
		},
		{
			name: "tampered rrset is rejected",
			buildMsg: func(t *testing.T) *dns.Msg {
				a := aRecord()
				sig, err := trustedSigner.SignRRset([]dns.RR{a})
				require.NoError(t, err)

				tampered := aRecord().(*dns.A)
				tampered.A = []byte{192, 0, 2, 99}

				msg := new(dns.Msg)
				msg.Answer = []dns.RR{tampered, sig}
				return msg
			},
			wantErr: true,
		},
		{
			name: "missing RRSIG entirely is rejected",
			buildMsg: func(t *testing.T) *dns.Msg {
				msg := new(dns.Msg)
				msg.Answer = []dns.RR{aRecord()}
				return msg
			},
			wantErr: true,
		},
		{
			name: "signature from a key absent from the trusted set is rejected",
			buildMsg: func(t *testing.T) *dns.Msg {
				a := aRecord()
				sig, err := otherSigner.SignRRset([]dns.RR{a})
				require.NoError(t, err)
				msg := new(dns.Msg)
				msg.Answer = []dns.RR{a, sig}
				return msg
			},
			wantErr: true,
		},
		{
			name: "expired signature is rejected",
			buildMsg: func(t *testing.T) *dns.Msg {
				shortLivedSigner, err := NewSigner("example.com", trustedKey, trustedPriv, -time.Hour)
				require.NoError(t, err)
				a := aRecord()
				sig, err := shortLivedSigner.SignRRset([]dns.RR{a})
				require.NoError(t, err)
				msg := new(dns.Msg)
				msg.Answer = []dns.RR{a, sig}
				return msg
			},
			wantErr: true,
		},
		{
			name: "message with no answers passes trivially",
			buildMsg: func(t *testing.T) *dns.Msg {
				return new(dns.Msg)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateMsg(tt.buildMsg(t))

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrValidationFailed))
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestNewValidator_RequiresAtLeastOneKey(t *testing.T) {
	_, err := NewValidator(nil)
	assert.Error(t, err)
}

func TestValidator_ValidateMsg_NilMessage(t *testing.T) {
	trustedKey, _, _ := testKeyPair(t, "example.com")
	validator, err := NewValidator([]*dns.DNSKEY{trustedKey})
	require.NoError(t, err)

	err = validator.ValidateMsg(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}
