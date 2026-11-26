// Command godns-keygen generates a DNSSEC key pair for a zone, writing the
// public DNSKEY RR and private key in the layout expected by godns'
// dnssec.signing config (see docs/dnssec.md).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/miekg/dns"

	"github.com/yourusername/godns/plugins/dnssec"
)

func main() {
	zone := flag.String("zone", "", "zone to generate a key for (required)")
	algorithm := flag.String("algorithm", "ECDSAP256SHA256", "signing algorithm (RSASHA256, ECDSAP256SHA256, ECDSAP384SHA384, or ED25519)")
	bits := flag.Int("bits", 256, "key size in bits (ignored for ECDSA/ED25519, which have a fixed size)")
	out := flag.String("out", "", "output path prefix; writes <out>.key and <out>.private (defaults to K<zone>)")
	flag.Parse()

	if *zone == "" {
		fmt.Fprintln(os.Stderr, "godns-keygen: -zone is required")
		flag.Usage()
		os.Exit(2)
	}

	alg, err := algorithmFromString(*algorithm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "godns-keygen: %v\n", err)
		os.Exit(2)
	}

	outPrefix := *out
	if outPrefix == "" {
		outPrefix = "K" + strings.TrimSuffix(dns.Fqdn(*zone), ".")
	}

	key, priv, err := dnssec.GenerateKey(*zone, alg, *bits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "godns-keygen: generating key: %v\n", err)
		os.Exit(1)
	}

	if err := dnssec.SaveKey(outPrefix, key, priv); err != nil {
		fmt.Fprintf(os.Stderr, "godns-keygen: saving key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s key for zone %s (key tag %d)\n", dns.AlgorithmToString[alg], *zone, key.KeyTag())
	fmt.Printf("  public key:  %s.key\n", outPrefix)
	fmt.Printf("  private key: %s.private\n", outPrefix)
}

func algorithmFromString(s string) (uint8, error) {
	s = strings.ToUpper(s)
	for id, name := range dns.AlgorithmToString {
		if name == s {
			return id, nil
		}
	}
	return 0, fmt.Errorf("unknown algorithm %q", s)
}
