package chain

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/lean/internal/pools"
	"github.com/geanlabs/gean/lean/internal/store"
)

// pruneStaleAttestationPools clears the attestation-keyed pools against a
// head-relative cutoff instead of the finalized slot.
//
// Finalization pruning is the only other path that touches these three pools,
// and it runs on finalization alone. PeriodicPrune, the existing stall
// fallback, prunes non-canonical states and blocks and none of the pools, needs
// finalization to be more than two pruning intervals behind, and fires only on
// exact multiples of that interval. So the one situation that makes the pools
// grow without limit is also the one that switches off everything that empties
// them.
//
// Below the finalized slot this is a no-op: finalization pruning already covers
// that range, and a cutoff at or under it would only repeat work.
func pruneStaleAttestationPools(p *pools.Attestations, headSlot, finalizedSlot uint64) {
	if p == nil || headSlot <= store.AttestationRetentionSlots {
		return
	}
	cutoff := headSlot - store.AttestationRetentionSlots
	if cutoff <= finalizedSlot {
		return
	}

	// All three pools key on the same target slot, so a data root leaves them
	// together and the order here does not matter.
	prunedSigs := p.AttestationSignatures.PruneStaleBelow(cutoff)
	prunedKnown := p.KnownPayloads.PruneStaleBelow(cutoff)
	prunedNew := p.NewPayloads.PruneStaleBelow(cutoff)

	if prunedSigs+prunedKnown+prunedNew == 0 {
		return
	}
	logger.Warn(logger.Store, "finalization lagging, pruned stale attestation pools: cutoff=%d head=%d finalized=%d gossip_sigs=%d payloads=%d",
		cutoff, headSlot, finalizedSlot, prunedSigs, prunedKnown+prunedNew)
}
