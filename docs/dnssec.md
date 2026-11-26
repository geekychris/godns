# DNSSEC

GoDNS supports two independent, opt-in DNSSEC features:

- **Signing**: authoritative answers (cache hits and plugin-resolved `A`
  records) for one configured zone are signed with RRSIGs, and the zone's
  `DNSKEY` is served (self-signed) on a `DNSKEY` query.
- **Validation**: responses forwarded from upstream servers are checked
  against a locally pinned set of trusted `DNSKEY` records; a missing or
  invalid `RRSIG` causes GoDNS to return `SERVFAIL` instead of passing the
  (unverified) answer through.

Both default to disabled. Existing `config.yaml` files with no `dnssec:`
section behave exactly as before.

## Known limitations

- **No chain of trust.** Validation checks signatures against a fixed,
  locally pinned set of `DNSKEY`s — there is no DS-record lookup, no walk
  up through parent zones, and no root trust anchor. You must distribute
  the upstream zone's public key to GoDNS out of band (see
  [Validating upstream responses](#validating-upstream-responses) below).
- **No NSEC/NSEC3.** GoDNS does not generate or verify authenticated denial
  of existence; NXDOMAIN responses are not signed or validated.
- **Only `A` records and the zone's own `DNSKEY` are signed.** Other
  authoritative query types (`AAAA`, `MX`, `CNAME`, ...) keep today's
  raw-forward behavior, since the cache/plugin pipeline itself only
  understands `A` records.
- **One signing zone at a time.** `dnssec.signing` holds exactly one
  zone/key pair; signing multiple zones requires running separate GoDNS
  instances (or a follow-up feature).
- **UDP only, no size negotiation.** Signed responses (an `A`/`DNSKEY` RRset
  plus an RRSIG) are larger than unsigned ones. GoDNS's DNS listener is
  UDP-only with no TCP fallback or EDNS0 buffer-size negotiation, so very
  large signed responses can be truncated by intermediate networks. This
  is unlikely to matter for a single key and a handful of `A` records, but
  is worth knowing before enabling signing on a zone with large RRsets.
- **No automatic key rollover.** Rotation (below) is a manual procedure.
- **Keys are files, not REST-managed.** Signing/trusted keys are configured
  by filesystem path; there is no REST endpoint for uploading or rotating
  keys.

## Generating a key

Use the `godns-keygen` tool to generate a key pair for a zone:

```sh
go run ./cmd/godns-keygen -zone example.com -out /etc/godns/keys/Kexample.com
```

Flags:

| Flag          | Default            | Notes                                                              |
|---------------|--------------------|----------------------------------------------------------------------|
| `-zone`       | (required)         | The zone to generate a key for, e.g. `example.com`.                 |
| `-algorithm`  | `ECDSAP256SHA256`  | One of `RSASHA256`, `ECDSAP256SHA256`, `ECDSAP384SHA384`, `ED25519`. |
| `-bits`       | `256`              | Key size in bits. Ignored for ECDSA/ED25519, which have a fixed size.|
| `-out`        | `K<zone>`          | Output path prefix.                                                  |

This writes two files:

- `<out>.key` — the public `DNSKEY` RR, in BIND zone-file text format.
- `<out>.private` — the private key, in BIND's `Private-key-format: v1.3`.

Keep `<out>.private` readable only by the GoDNS process (e.g. `chmod 600`);
`godns-keygen` already writes it with mode `0600`.

## Signing your own zone

Point `dnssec.signing` at the generated key pair:

```yaml
dnssec:
  signing:
    enabled: true
    zone: "example.com"
    key_file: "/etc/godns/keys/Kexample.com"
    signature_ttl: "1h"   # how long each generated RRSIG remains valid
```

With signing enabled, GoDNS will:

- Append an `RRSIG` to any `A` answer served from cache or a plugin
  resolver, for names at or below `zone`.
- Answer `DNSKEY` queries for `zone` with the public key plus a
  self-signed `RRSIG`.
- Leave answers forwarded from upstream servers unsigned — GoDNS has no
  authority to assert over data it fetched from elsewhere.

A client (or GoDNS acting as a validator, see below) can verify a signed
answer against the published `DNSKEY`:

```sh
dig @127.0.0.1 -p 53 example.com DNSKEY
dig @127.0.0.1 -p 53 www.example.com A +dnssec
```

## Validating upstream responses

Point `dnssec.validation` at the public key(s) you trust for your
upstream resolvers:

```yaml
dnssec:
  validation:
    enabled: true
    trusted_key_files:
      - "/etc/godns/keys/Kexample.com.key"
```

Each entry in `trusted_key_files` is a `.key` file (the same public-key
text format `godns-keygen` writes) containing one `DNSKEY` RR. With
validation enabled, GoDNS sets the EDNS0 `DO` bit on forwarded queries and
requires every RRset in the upstream response to carry a covering,
cryptographically valid, time-valid `RRSIG` signed by one of the trusted
keys. If any RRset fails that check — missing `RRSIG`, bad signature,
expired/not-yet-valid signature, or a signer not in the trusted set — the
query fails with `SERVFAIL` rather than returning an unverified answer.

Since there is no chain-of-trust lookup, `trusted_key_files` must be kept
in sync with whatever keys the upstream zone is actually signing with; a
key rollover on the upstream side (see below) requires updating this list
too.

## Key rotation

GoDNS has no automatic rollover; follow this manual procedure to replace a
signing key without a validation gap:

1. **Generate the new key** alongside the old one:
   ```sh
   go run ./cmd/godns-keygen -zone example.com -out /etc/godns/keys/Kexample.com.new
   ```
2. **Publish both keys** to anyone validating your zone, by distributing
   both `.key` files (old and new) before switching. If GoDNS itself is
   also configured to validate this zone elsewhere, add the new `.key`
   file to that `trusted_key_files` list first.
3. **Wait at least one `signature_ttl`** (the old zone's configured value)
   plus the propagation/cache time of anyone relying on the old key, so no
   validator is still relying on a signature that could outlive the
   rollover.
4. **Switch `dnssec.signing.key_file`** to the new key's path prefix and
   restart GoDNS. New answers are now signed with the new key.
5. **Remove the old key** from any `trusted_key_files` lists once you are
   confident nothing is still validating against it, and delete the old
   key files.

For validation-only key changes (an upstream server rotated its key),
simply add the new public key to `trusted_key_files` before the upstream
switches, and remove the old one once the upstream has fully rolled over.
