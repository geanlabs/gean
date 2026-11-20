package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/core/httpapi"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, healthResponse{
		Status:  "healthy",
		Service: "lean-rpc-api",
	})
}
