# Devnet-4 Port Plan

**Branch:** `devnet-4` (push directly, no feature branches)
**Starting point:** `devnet-4` branch (identical to `main`)
**Method:** Phase by phase → implement → unit tests → spec tests → `/spec-compliance` check → push
**Source of truth:** leanSpec spec diff (devnet-3 → HEAD)
**Cross-reference:** zeam (devnet4), ethlambda (devnet4-phase4-network), nlean (feat/devnet4), lantern (devnet-4)

---

## Spec Changes Summary

| Category | Key Changes |
|---|---|
| **Block Envelope** | `SignedBlockWithAttestation` → `SignedBlock`; `BlockWithAttestation` removed; proposer signs `hash_tree_root(block)` with proposal key |
| **Validator Model** | Single `pubkey` → dual `attestation_pubkey` + `proposal_pubkey`; `GenesisValidatorEntry` with dual keys |
| **Attestation** | `MAX_ATTESTATIONS_DATA = 16`; `aggregate_by_data()` removed; `data_root_bytes()` removed |
| **Block Building** | Fixed-point loop over aggregated payloads; greedy proof selection; recursive children aggregation; `MAX_ATTESTATIONS_DATA` enforced |
| **Forkchoice Store** | `gossip_signatures` → `attestation_signatures`; `aggregate()` rewrite with recursive API; `on_block` enforces single AttestationData + MAX cap |
| **Aggregation** | New recursive `AggregatedSignatureProof.aggregate(xmss_participants, children, raw_xmss, message, slot)` API |

---

## Testing Strategy (Every Phase)

Each phase follows this checklist before pushing:

1. **Unit tests** — test each changed function in isolation
2. **Spec tests** — run relevant leanSpec test vectors (`make test-spec`)
3. **Spec compliance** — run `/spec-compliance` skill against changed files
4. **Build** — `go build ./...` clean
5. **All existing tests** — `go test ./...` passes (no regressions)

Full integration test (3-node devnet) runs after Phase 6 when all infrastructure is in place.

---

## Phase Overview

| Phase | Focus | Depends on |
|---|---|---|
| 1 | Dual-key Validator types + constants | None |
| 2 | Dual-key management (KeyManager, genesis, keygen) | Phase 1 |
| 3 | Dual-key signing + verification | Phase 2 |
| 4 | Block envelope (SignedBlock rename, remove ProposerAttestation) | Phase 3 |
| 5 | Store & forkchoice rewrite | Phase 4 |
| 6 | Block building rewrite (fixed-point, greedy selection) | Phase 5 |
| 7 | Recursive aggregation | Phase 6 |

---

## Phase 1: Dual-Key Validator Types

### Spec reference
- Validator Model [Modified]: single `pubkey` → dual `attestation_pubkey` + `proposal_pubkey`
- Chain Config [New]: `MAX_ATTESTATIONS_DATA = 16`

### Implementation
| File | Change |
|---|---|
| `types/validator.go` | `Pubkey` → `AttestationPubkey` + `ProposalPubkey` |
| `types/validator_encoding.go` | SSZ encoding (60→112 bytes) |
| `types/state_encoding.go` | Validator size references (60→112) |
| `types/constants.go` | Add `MaxAttestationsData = 16` |
| `genesis/config.go` | Populate both keys from same genesis key (Phase 2 parses separate keys) |
| All `.Pubkey` references | → `.AttestationPubkey` (temporary; Phase 3 corrects proposal contexts) |
| `Makefile` | Don't delete `*_encoding.go` on `make clean` |

### Unit tests
- [ ] Validator SSZ roundtrip with dual keys
- [ ] Validator HashTreeRoot with dual keys matches spec
- [ ] State SSZ roundtrip with 112-byte validators
- [ ] Validator SizeSSZ returns 112

### Spec tests
- [ ] `test_consensus_containers.py` — Validator dual pubkeys portion

### Spec compliance
- [ ] Run `/spec-compliance` on `types/validator.go`
- [ ] Run `/spec-compliance` on `types/constants.go`

---

## Phase 2: Dual-Key Management

### Spec reference
- Validator Model [New]: `GenesisValidatorEntry` pairs two per-validator pubkeys
- XMSS containers: key manager API updated to per-purpose keys

### Implementation
| File | Change |
|---|---|
| `xmss/keys.go` | Split `keys` map into `attestationKeys` + `proposalKeys` |
| `xmss/keys.go` | Add `GetAttestationKey(vid)`, `GetProposalKey(vid)` accessors |
| `xmss/keys.go` | Update `LoadKeys()` for dual key files with legacy fallback |
| `genesis/config.go` | Parse `GenesisValidatorEntry` with dual pubkeys from YAML |
| `cmd/keygen/` | Generate dual keypairs per validator |

### Cross-client reference
- nlean: `ValidatorKeyMaterial(AttestationPublicKey, AttestationSecretKey, ProposalPublicKey, ProposalSecretKey)` with legacy fallback
- ethlambda: `ValidatorKeyPair { attestation_key, proposal_key }` with independent OTS advancement
- zeam: `attestation_keypair` + `proposal_keypair` in key-manager

### Unit tests
- [ ] Key generation produces two keypairs per validator
- [ ] Key loading (new dual format) works
- [ ] Key loading (legacy single-key fallback) works
- [ ] `GetAttestationKey` returns correct key
- [ ] `GetProposalKey` returns correct key
- [ ] Genesis config parses dual pubkeys from YAML
- [ ] `ValidatorIDs()` returns union of both key maps

### Spec tests
- [ ] `test_xmss_containers.py` — per-purpose key manager

### Spec compliance
- [ ] Run `/spec-compliance` on `xmss/keys.go`
- [ ] Run `/spec-compliance` on `genesis/config.go`

---

## Phase 3: Dual-Key Signing + Verification

### Spec reference
- BlockSignatures.proposer_signature: signs `hash_tree_root(block)` with proposal key
- SignedBlock.verify_signatures: takes `validators` directly
- Attestation verification: uses `attestation_pubkey`

### Implementation
| File | Change |
|---|---|
| `xmss/keys.go` | `SignAttestation()` routes through `attestationKeys` |
| `xmss/keys.go` | `SignBlock()` routes through `proposalKeys`, signs block root |
| `node/store_block.go` | `verifyBlockSignatures()`: proposer sig verified with `ProposalPubkey` over block root |
| `node/store_block.go` | Body attestation sigs verified with `AttestationPubkey` (already done Phase 1) |
| `node/block.go` | `onGossipAttestation()`: verify with `AttestationPubkey` (already done Phase 1) |

### Cross-client reference
- All clients: proposer uses `proposal_pubkey` to sign block root
- ethlambda: `key_manager.sign_block_root(validator_id, slot, &block_root)`
- zeam: `key_manager.signBlockRoot(slot_proposer_id, &produced_block.blockRoot, slot)`

### Unit tests
- [ ] Proposer signature over block root verifies with ProposalPubkey
- [ ] Proposer signature fails verification with AttestationPubkey
- [ ] Attestation signature verifies with AttestationPubkey
- [ ] Attestation signature fails verification with ProposalPubkey
- [ ] Block with correctly signed proposer sig processes successfully
- [ ] Block with wrong key type → verification fails

### Spec tests
- [ ] `test_valid_signatures.py` — updated for SignedBlock
- [ ] `test_invalid_signatures.py` — updated, `test_valid_signature_wrong_validator` removed

### Spec compliance
- [ ] Run `/spec-compliance` on `node/store_block.go` (verifyBlockSignatures)
- [ ] Run `/spec-compliance` on `xmss/keys.go` (SignBlock, SignAttestation)

---

## Phase 4: Block Envelope Simplification

### Spec reference
- SignedBlockWithAttestation → SignedBlock
- BlockWithAttestation removed
- Proposer no longer bundles per-block attestation
- Proposer attests at interval 1 like all validators

### Implementation
| File | Change |
|---|---|
| `types/block.go` | Remove `BlockWithAttestation`; rename `SignedBlockWithAttestation` → `SignedBlock` |
| `types/block_encoding.go` | Remove BlockWithAttestation SSZ; update SignedBlock encoding |
| `node/store_block.go` | Remove `ProcessProposerAttestation`, `processProposerAttestation` |
| `node/validator.go` | `maybePropose`: build `SignedBlock`, sign block root, no proposer attestation |
| `node/validator.go` | `produceAttestations`: remove proposer skip, add self-delivery for aggregator |
| `node/block.go` | Remove `ProcessProposerAttestation` call; update types |
| `node/node.go` | Update `BlockCh` type, `OnBlock` parameter |
| `node/consensus_store.go` | Update `GetSignedBlock`, `writeBlockData`, `StorePendingBlock` |
| `p2p/*.go` | Update all `SignedBlockWithAttestation` → `SignedBlock` |
| `spectests/*.go` | Update test fixtures |

### Cross-client reference
- ALL clients: removed BlockWithAttestation, proposer attests at interval 1
- ethlambda: self-delivery only if `is_aggregator` via `store::on_gossip_attestation()`
- zeam: self-delivery via `publishAttestation()` → `chain.onGossipAttestation()`

### Unit tests
- [ ] SignedBlock SSZ roundtrip (no BlockWithAttestation)
- [ ] Block without ProposerAttestation builds correctly
- [ ] Proposer produces attestation at interval 1
- [ ] Aggregator sees own node's attestations via self-delivery
- [ ] Non-aggregator nodes don't self-deliver

### Spec tests
- [ ] `test_consensus_containers.py` — SignedBlock (no BlockWithAttestation)

### Spec compliance
- [ ] Run `/spec-compliance` on `types/block.go`
- [ ] Run `/spec-compliance` on `node/validator.go`
- [ ] Run `/spec-compliance` on `node/store_block.go`

### Note
This phase removes ProposerAttestation. Full devnet validation deferred to after Phase 6. The store rewrite (Phase 5) and block builder rewrite (Phase 6) provide the infrastructure needed for justification to work without the embedded proposer vote.

---

## Phase 5: Store & Forkchoice Rewrite

### Spec reference
- `gossip_signatures` → `attestation_signatures`
- `aggregate_committee_signatures()` → `aggregate()`
- `on_block`: enforce single AttestationData + MAX_ATTESTATIONS_DATA cap
- `on_attestation`: drop subnet filtering, use attestation_pubkey
- `validate_attestation`: takes AttestationData directly

### Implementation
| File | Change |
|---|---|
| `node/store_gossip.go` | Rename `GossipSignatureEntry` → `AttestationSignatureEntry`, `GossipSignatures` → `AttestationSignatures` |
| `node/store_aggregate.go` | Rewrite `aggregate()`: greedy selection over (new, known) pools + raw signatures; recursive API |
| `node/store_block.go` | `on_block`: validate single AttestationData per entry, count ≤ MAX_ATTESTATIONS_DATA |
| `node/store_validate.go` | `ValidateAttestationData`: takes AttestationData directly |
| `node/block.go` | `onGossipAttestation`: immediate vote tracker update (SetNew); drop subnet filtering |
| `node/block.go` | `onGossipAggregatedAttestation`: immediate vote tracker update per participant |
| `node/block.go` | `processOneBlock`: disaggregate block body attestations into per-validator votes (SetKnown) |
| `node/consensus_store.go` | `ExtractLatestAllAttestations`: include attestation_signatures (gossip) votes |
| `node/tick.go` | Drain all pending messages (blocks + attestations) before each tick |

### Cross-client reference
- zeam: nested `AttestationData → ValidatorIndex → Signature` map; immediate `onAttestationUnlocked` for gossip
- ethlambda: `HashedAttestationData` key; self-delivery for aggregators
- nlean: `VoteSource` enum (Known/New/Merged); safe target can regress

### Unit tests
- [ ] `aggregate()` produces valid proofs from gossip signatures
- [ ] `on_block` rejects duplicate AttestationData in body
- [ ] `on_block` rejects > MAX_ATTESTATIONS_DATA
- [ ] Gossip attestation immediately updates vote tracker (SetNew)
- [ ] Block body attestations disaggregated into per-validator votes (SetKnown)
- [ ] Attestation source from head state (not store-wide)

### Spec tests
- [ ] `test_gossip_attestation_validation.py` — 15 scenarios
- [ ] `test_gossip_aggregated_attestation_validation.py` — 6 scenarios
- [ ] `test_fork_choice_head.py` — attestation-based weighting
- [ ] `test_fork_choice_reorgs.py` — weight-based reorgs
- [ ] `test_attestation_source_divergence.py` — source mismatch

### Spec compliance
- [ ] Run `/spec-compliance` on `node/store_aggregate.go`
- [ ] Run `/spec-compliance` on `node/store_block.go` (on_block invariants)
- [ ] Run `/spec-compliance` on `node/block.go` (onGossipAttestation)
- [ ] Run `/spec-compliance` on `node/store_validate.go`

---

## Phase 6: Block Building Rewrite

### Spec reference
- `build_block`: fixed-point loop over aggregated_payloads
- Greedy proof selection via `select_greedily`
- Recursive children aggregation for compaction
- MAX_ATTESTATIONS_DATA enforced
- Genesis edge case: substitute parent_root for zero-hash checkpoint

### Implementation
| File | Change |
|---|---|
| `node/store_build.go` | Rewrite `buildBlock()` as fixed-point loop with greedy selection |
| `node/store_build.go` | `produce_block_with_signatures`: pass payloads + block-root set |
| `node/store_build.go` | Remove old `selectLatestPerValidator`, `emitAttestationsForGroup` |
| `node/store_build.go` | Add proof compaction (merge same-data proofs via recursive aggregation) |

### Cross-client reference
- zeam: `getProposalAttestationsUnlocked()` with compaction via `compactAttestations()`
- ethlambda: `build_block()` with sorted entries, proof byte budget (9 MiB)
- nlean: `BuildAggregatedAttestations()` with iteration limit (10), max 3 proofs

### Unit tests
- [ ] Fixed-point loop advances justification correctly
- [ ] Greedy selection maximizes validator coverage
- [ ] MAX_ATTESTATIONS_DATA enforced in built blocks
- [ ] Proof compaction merges same-data proofs
- [ ] Genesis edge case (zero-hash substitution) handled
- [ ] Block with compacted proofs processes and verifies correctly

### Spec tests
- [ ] `test_block_production.py` — covers produce_block_with_signatures
- [ ] `test_justification.py` — 20 scenarios (comprehensive justification/finality)
- [ ] `test_attestation_target_selection.py` — updated for aggregated entries

### Spec compliance
- [ ] Run `/spec-compliance` on `node/store_build.go`

### Integration test (FIRST FULL DEVNET TEST)
- [ ] 3-node gean-only devnet: justification/finalization advancing steadily
- [ ] Justified within 5 slots of head
- [ ] No `checkpointExists` failures
- [ ] No silent attestation rejections

---

## Phase 7: Recursive Aggregation

### Spec reference
- `AggregatedSignatureProof.aggregate(xmss_participants, children, raw_xmss, message, slot)`
- Recursive children proof support
- leanMultisig bindings update (PR #496)

### Implementation
| File | Change |
|---|---|
| `xmss/ffi.go` | Update FFI for recursive aggregation (children proofs, log_inv_rate) |
| `xmss/rust/` | Update leanMultisig Rust glue for children support |
| `node/store_aggregate.go` | Wire recursive API into `aggregate()` |
| `node/store_build.go` | Wire recursive API into block builder compaction |

### Cross-client reference
- zeam: Full recursive aggregation with `LOG_INV_RATE` configuration
- lantern: Unconditionally recursive, MAX_RECURSIONS=16 cap
- nlean: API exists but recursive path not yet implemented
- ethlambda: Flat aggregation only (no recursive yet)

### Unit tests
- [ ] Recursive proof with children verifies correctly
- [ ] Flat aggregation still works (backward compatible)
- [ ] Deep aggregation tree (3+ levels) works
- [ ] Block with recursively aggregated proofs processes correctly

### Spec tests
- [ ] `test_signature_aggregation.py` — updated for recursive API
- [ ] ALL spec tests re-run (full regression)

### Spec compliance
- [ ] Run `/spec-compliance` on `xmss/ffi.go`
- [ ] Run `/spec-compliance` on `node/store_aggregate.go`

### Final integration test
- [ ] 3-node gean-only devnet: all phases active
- [ ] All leanSpec test vectors pass (42+ scenarios)
- [ ] No spec divergences

---

## Lessons Learned (from v1 attempt)

1. **Don't remove ProposerAttestation before store + block builder rewrite.** Phase 4 (envelope change) needs Phases 5-6 (store rewrite + builder rewrite) to function in a multi-node devnet. The embedded proposer vote provides guaranteed vote delivery that the new infrastructure must replace.

2. **Test at each phase, but defer full devnet testing to Phase 6.** Phases 1-3 are safe type/key/signing changes. Phase 4 removes ProposerAttestation. Phases 5-6 provide the compensating infrastructure. Only then can a devnet test pass.

3. **Self-delivery is essential for aggregators.** Gossipsub doesn't deliver messages back to the sender. Aggregator nodes must explicitly process their own attestations. ethlambda: conditional on `is_aggregator`. zeam: via `publishAttestation()` path.

4. **Use `/spec-compliance` at every phase.** Don't wait until the end to discover spec divergences.

5. **Parallel verification is a separate optimization** — belongs on `main`, not tied to devnet-4.
