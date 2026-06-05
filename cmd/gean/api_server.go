//go:build !hive_testdriver

package main

import (
	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func startAPIServer(address string, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) error {
	return api.StartAPIServer(address, s, fc, aggCtl)
}
