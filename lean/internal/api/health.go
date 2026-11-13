package api

import "net/http"

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "healthy",
		Service: "lean-rpc-api",
	})
}
