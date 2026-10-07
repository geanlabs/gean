//go:build !hive_testdriver

package main

import (
	"net/http"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
)

func apiHandler(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) http.Handler {
	return api.NewHandler(s, fc, aggCtl)
}
