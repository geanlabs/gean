package testdriver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/internal/core/httpapi"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
)

func (sess *Session) ForkChoiceInitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req specfixtures.ForkChoiceInit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		anchorState, err := req.AnchorState.ToState()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorState: %v", err))
			return
		}
		if req.GenesisTime != nil {
			anchorState.Config.GenesisTime = *req.GenesisTime
		}

		anchorBlock, err := req.AnchorBlock.ToBlock()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorBlock: %v", err))
			return
		}
		replay, err := NewReplay(anchorState, anchorBlock)
		if err != nil {
			writeInitFailure(w, err.Error())
			return
		}

		sess.mu.Lock()
		sess.replay = replay
		sess.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}
}

func writeInitFailure(w http.ResponseWriter, msg string) {
	resp := driverStepResponse{Accepted: false, Error: &msg, Snapshot: driverSnapshot{}}
	httpapi.WriteJSON(w, http.StatusBadRequest, resp)
}

func (sess *Session) ForkChoiceStepHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var step specfixtures.ForkChoiceStep
		if err := json.NewDecoder(r.Body).Decode(&step); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		sess.mu.Lock()
		defer sess.mu.Unlock()

		if sess.replay == nil {
			msg := "fork_choice/step called before fork_choice/init"
			resp := driverStepResponse{Accepted: false, Error: &msg, Snapshot: driverSnapshot{}}
			httpapi.WriteJSON(w, http.StatusOK, resp)
			return
		}

		stepErr := sess.replay.Step(&step)
		snap := sess.replay.snapshot()
		accepted := stepErr == nil
		var errPtr *string
		if stepErr != nil {
			msg := stepErr.Error()
			errPtr = &msg
		}
		httpapi.WriteJSON(w, http.StatusOK, driverStepResponse{Accepted: accepted, Error: errPtr, Snapshot: snap})
	}
}

func (r *Replay) snapshot() driverSnapshot {
	headRoot := r.Store.Head()
	headSlot, _, _ := r.Store.BlockSlotAndParent(headRoot)
	justified := r.Store.LatestJustified()
	finalized := r.Store.LatestFinalized()
	safeTarget := r.Store.SafeTarget()
	return driverSnapshot{
		HeadSlot:            headSlot,
		HeadRoot:            fmt.Sprintf("0x%x", headRoot),
		Time:                r.Store.Time(),
		JustifiedCheckpoint: driverCheckpoint{Slot: justified.Slot, Root: fmt.Sprintf("0x%x", justified.Root)},
		FinalizedCheckpoint: driverCheckpoint{Slot: finalized.Slot, Root: fmt.Sprintf("0x%x", finalized.Root)},
		SafeTarget:          fmt.Sprintf("0x%x", safeTarget),
	}
}
