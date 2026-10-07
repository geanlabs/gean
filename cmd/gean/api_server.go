//go:build !hive_testdriver

package main

import (
	"net/http"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
)

func apiHandler(s *store.ConsensusStore, forkChoiceView func() *forkchoice.View, aggCtl *role.Controller, _ crypto.Scheme) http.Handler {
	return api.NewHandler(s, forkChoiceView, aggCtl)
}
