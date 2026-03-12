package httprest

import (
	"net/http"

	"github.com/geanlabs/gean/api/handlers"
	"github.com/geanlabs/gean/chain/forkchoice"
)

// NewMux constructs the HTTP router for the Lean API.
func NewMux(storeGetter func() *forkchoice.Store) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc(HealthPath, handlers.Health())
	mux.HandleFunc(FinalizedStatePath, handlers.FinalizedState(storeGetter))
	mux.HandleFunc(JustifiedPath, handlers.JustifiedCheckpoint(storeGetter))
	mux.HandleFunc(ForkChoicePath, handlers.ForkChoice(storeGetter))
	return mux
}
