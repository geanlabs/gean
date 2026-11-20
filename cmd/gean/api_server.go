//go:build !hive_testdriver

package main

import (
	"net/http"

	"github.com/geanlabs/gean/internal/lean/api"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/store"
)

func newAPIServer(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.Server {
	return api.NewAPIServer(s, fc, aggCtl)
}
