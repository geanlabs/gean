package testdriver

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type stateTransitionResponse struct {
	Succeeded bool                 `json:"succeeded"`
	Error     *string              `json:"error"`
	Post      *stateTransitionPost `json:"post"`
}

type stateTransitionPost struct {
	Slot                       uint64 `json:"slot"`
	LatestBlockHeaderSlot      uint64 `json:"latestBlockHeaderSlot"`
	LatestBlockHeaderStateRoot string `json:"latestBlockHeaderStateRoot"`
	HistoricalBlockHashesCount uint64 `json:"historicalBlockHashesCount"`
}

type driverStepResponse struct {
	Accepted bool           `json:"accepted"`
	Error    *string        `json:"error"`
	Snapshot driverSnapshot `json:"snapshot"`
}

type driverSnapshot struct {
	HeadSlot            uint64           `json:"headSlot"`
	HeadRoot            string           `json:"headRoot"`
	Time                uint64           `json:"time"`
	JustifiedCheckpoint driverCheckpoint `json:"justifiedCheckpoint"`
	FinalizedCheckpoint driverCheckpoint `json:"finalizedCheckpoint"`
	SafeTarget          string           `json:"safeTarget"`
}

type driverCheckpoint struct {
	Slot uint64 `json:"slot"`
	Root string `json:"root"`
}

type verifySignaturesResponse struct {
	Succeeded bool    `json:"succeeded"`
	Error     *string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, fmt.Sprintf("marshal response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}
