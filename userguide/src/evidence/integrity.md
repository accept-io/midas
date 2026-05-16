# Integrity

Evidence envelopes are signed at emission. Integrity verification ensures
that:

- The envelope's contents have not been altered after emission.
- The envelope was emitted by a known signing identity.
- The envelope's parent (for amended envelopes) is correctly chained.

## How integrity is checked

The signing hash is computed over a canonical serialisation of the
envelope's content. A verifier re-computes the hash, checks the signature
against the issuer's public key, and verifies the parent chain when
present.

The Explorer does not perform integrity verification itself — that is the
job of downstream evidence-store consumers. The Explorer trusts the store
to surface only valid envelopes.

## When integrity matters

- Long-lived audit reviews (regulatory enquiries, post-incident analysis).
- Replays of decision history.
- Cross-region replication, where envelopes need to be verified before
  they are accepted.
