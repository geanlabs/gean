# Gean Devnet-2 Implementation Plan

This plan is self-contained and assumes PM is reference-only. Only Gean is edited.

## Authoritative Targets (Reference Only)
- leanSpec fixtures: `4edcf7bc9271e6a70ded8aff17710d68beac4266`
- leanSig: `73bedc26ed961b110df7ac2e234dc11361a4bf25`
- leanMultisig: `e5b27c0456dfd011ef13fffa442a61d3ab2737c4`
- leanMetrics: `3b32b300cca5ed7a7a2b3f142273fae9dbc171bf`
- Interop reference: Zeam `origin/devnet2@3aa5d55c429ce9fd59b75803877df8db337b2b1c`

## Non-Negotiable Engineering Constraints
- SSZ encode/decode must be generated with `fastssz` (no manual SSZ logic).
- CGo headers must be generated with `cbindgen` (no manual headers).
- Code philosophy: subtraction over addition, readability over cleverness, avoid unnecessary abstractions.

---

## Step 1: Lock Dependencies (Gean Only)
**Actions**
- Update `gean/xmss/leansig-ffi/Cargo.toml` to pin leanSig at `73bedc2`.
- Add leanMultisig FFI crate pinned to `e5b27c0` and expose C API for:
  - `aggregate_signatures(...)`
  - `verify_aggregated_signatures(...)`
- Use `cbindgen` to generate CGo headers.

**Tests (run after step)**
- `go test ./...`
- Any new FFI unit tests (if added)

---

## Step 2: Migrate Core SSZ Containers to Devnet-2 Shapes
**Actions**
- Update Gean types to match devnet-2 containers:
  - `SignedAttestation` -> `{validator_id, message: AttestationData, signature}`
  - `BlockBody.Attestations` -> aggregated attestation list
  - `SignedBlockWithAttestation.Signature` -> `{attestation_signatures, proposer_signature}`
- Regenerate SSZ encode/decode with `fastssz`.
- Update any codecs/transports that assume old shapes.

**Tests**
- `go test ./types/...`
- `go test ./spectests/...`

---

## Step 3: Add AggregatedSignatureProof Type + leanMultisig Integration
**Actions**
- Add Go type for `AggregatedSignatureProof` with:
  - `participants` (bitlist)
  - `proof_data` (byte blob)
- Wire leanMultisig FFI into Gean aggregation and verification paths.

**Tests**
- Unit tests for proof aggregation + verification
- `go test ./xmss/...` (or FFI test package)

---

## Step 4: Replace Concatenation Aggregation with Proofs
**Actions**
- Replace `AggregateAttestations` logic to:
  - Group by `AttestationData`
  - Build aggregated attestations
  - Produce aggregated proofs via leanMultisig
- Remove concatenation-based signature aggregation.

**Tests**
- `go test ./chain/...`

---

## Step 5: Rework Block Production & Validation
**Actions**
- Block production:
  - Build aggregated attestations + proof list
  - Keep proposer signature as single XMSS
- Block validation:
  - Enforce `len(attestations) == len(attestation_signatures)`
  - Check proof participants == aggregation_bits
  - Verify proof over attestation data root
  - Verify proposer signature separately

**Tests**
- `go test ./chain/...`
- `go test ./node/...`

---

## Step 6: Add Devnet-2 Forkchoice Caches
**Actions**
- Add signature caches:
  - `signature_key -> individual signatures`
  - `signature_key -> aggregated proofs`
- On block import, store proofs per participant for future block building.
- Use cached proofs during block production.

**Tests**
- `go test ./chain/forkchoice/...`

---

## Step 7: Networking Alignment (Devnet-2 Only)
**Actions**
- Use only block + attestation gossip topics (no devnet-3 aggregation topic).
- Ensure fork-digest topic formatting matches leanSpec fixtures.
- Support BlocksByRoot protocol: `"/leanconsensus/req/blocks_by_root/1/ssz_snappy"`.

**Tests**
- `go test ./network/...`

---

## Step 8: Update Spectests to Devnet-2 Fixtures
**Actions**
- Pin fixtures to `leanSpec@4edcf7b`.
- Update spectest converters to new schemas.
- Rebaseline expected outputs as needed.

**Tests**
- `go test ./spectests/...`

---

## Step 9: Interop Smoke (Zeam devnet-2)
**Actions**
- Run interop smoke with Zeam `origin/devnet2@3aa5d55`.
- Validate:
  - SSZ decode compatibility
  - Signature verification parity
  - BlocksByRoot sync recovery

**Tests**
- Execute interop smoke run (document exact commands used)

---

## Step 10: Final Compliance Gate
**Actions**
- Run full test suite: `go test ./...`
- Re-run spec tests + interop smoke
- Report readiness with exact hashes

---

## Explicit Non-Goals (Devnet-2 Strict)
- No recursive aggregation
- No devnet-3 aggregation gossip pipeline
- No devnet-3 ENR aggregator extensions
