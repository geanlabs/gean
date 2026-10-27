//go:build !hive_testdriver

package main

import (
	"net/http"

	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func newAPIServer(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.Server {
	return api.NewAPIServer(s, fc, aggCtl)
}
