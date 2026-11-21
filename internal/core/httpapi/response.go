package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteJSON writes v as a JSON response with status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, fmt.Sprintf("marshal response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status is already sent, so a failed write has nowhere to be reported.
	_, _ = w.Write(data)
}

// WriteSSZ writes SSZ-encoded data as an octet-stream response.
func WriteSSZ(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}
