package aggregation

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/xmss"
)

func TestAggregationYieldsToWaitingProposal(t *testing.T) {
	for _, fail := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			gate := proving.NewGate()
			if !gate.Acquire(context.Background(), false) {
				t.Fatal("background acquire failed")
			}
			acquired := make(chan struct{})
			cache := xmss.NewPubKeyCache()
			defer cache.Close()
			calls := 0
			prove := func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error) {
				calls++
				go func() {
					if gate.Acquire(context.Background(), true) {
						close(acquired)
						gate.Release(true)
					}
				}()
				synctest.Wait()
				if !gate.ProposalPending() {
					t.Fatal("proposal did not wait for active proof")
				}
				if fail {
					return nil, errors.New("proof failure")
				}
				return []byte{1}, nil
			}
			snap := budgetTestSnapshot()
			aggs, payloads, deletes, truncated, skips := aggregateFromSnapshotWithProver(gate.ProposalPending, snap, cache, time.Now().Add(time.Hour), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator(), prove)
			if calls != 1 || !truncated || skips[metrics.AggGroupSkipProposalPending] != 1 {
				t.Fatalf("calls=%d truncated=%v skips=%v", calls, truncated, skips)
			}
			if fail {
				if len(aggs)+len(payloads)+len(deletes) != 0 {
					t.Fatal("failed proof retired inputs")
				}
			} else {
				if len(aggs) != 1 || len(payloads) != 1 || len(deletes) != 3 {
					t.Fatal("completed output was lost")
				}
				for _, key := range deletes {
					if key.DataRoot != rootByte(2) {
						t.Fatal("deferred group's inputs deleted")
					}
				}
			}
			gate.Release(false)
			synctest.Wait()
			select {
			case <-acquired:
			default:
				t.Fatal("waiting proposal did not acquire released prover")
			}
			if gate.ProposalPending() {
				t.Fatal("proposal priority leaked after completion")
			}
		})
	}
}

func TestAggregationYieldsAfterPreparation(t *testing.T) {
	snap := budgetTestSnapshot()
	delete(snap.attSigs, rootByte(1))
	delete(snap.attSigs, rootByte(3))
	cache := xmss.NewPubKeyCache()
	defer cache.Close()
	checks := 0
	shouldYield := func() bool { checks++; return checks == 2 }
	prove := func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error) {
		t.Fatal("started proof after proposal became pending")
		return nil, nil
	}
	aggs, payloads, deletes, truncated, skips := aggregateFromSnapshotWithProver(shouldYield, snap, cache, time.Now().Add(time.Hour), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator(), prove)
	if !truncated || skips[metrics.AggGroupSkipProposalPending] != 1 || len(aggs)+len(payloads)+len(deletes) != 0 {
		t.Fatalf("unexpected yield: %v %v", truncated, skips)
	}
	if len(snap.attSigs[rootByte(2)].Signatures) != 3 {
		t.Fatal("unproved inputs lost")
	}
}
