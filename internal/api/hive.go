//go:build hive_testdriver

package api

import (
	"fmt"
	"net"
	"net/http"

	"github.com/geanlabs/gean/internal/api/testdriver"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func StartAPIServerWithTestDriver(address string, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) error {
	mux := buildAPIMuxWithTestDriver(s, fc, aggCtl)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("api listen: %w", err)
	}

	logger.Info(logger.Network, "api server listening on %s (test-driver routes enabled)", address)
	return http.Serve(listener, mux)
}

func buildAPIMuxWithTestDriver(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.ServeMux {
	mux := buildAPIMux(s, fc, aggCtl)
	testdriver.RegisterRoutes(mux, testdriver.NewSession())
	return mux
}
