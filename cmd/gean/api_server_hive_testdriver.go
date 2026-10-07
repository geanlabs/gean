//go:build hive_testdriver

package main

import (
	"net/http"
	"os"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/api/testdriver"
	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/storage/store"
)

func apiHandler(s *store.ConsensusStore, forkChoiceView func() *forkchoice.View, aggCtl *role.Controller, scheme crypto.Scheme) http.Handler {
	if testdriver.IsEnabled(os.Getenv(testdriver.EnvVar)) {
		logger.Info(logger.Node, "%s=1: enabling test-driver routes", testdriver.EnvVar)
		return api.NewHandlerWithTestDriver(s, forkChoiceView, aggCtl, scheme)
	}
	return api.NewHandler(s, forkChoiceView, aggCtl)
}
