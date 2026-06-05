package api

import (
	"fmt"
	"net"
	"net/http"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func StartAPIServer(address string, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) error {
	mux := buildAPIMux(s, fc, aggCtl)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("api listen: %w", err)
	}

	logger.Info(logger.Network, "api server listening on %s", address)
	return http.Serve(listener, mux)
}
