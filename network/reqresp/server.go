package reqresp

import (
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
)

var reqrespLog = logging.NewComponentLogger(logging.CompReqResp)

// RegisterReqResp registers request/response protocol handlers.
func RegisterReqResp(h host.Host, handler *ReqRespHandler) {
	h.SetStreamHandler(StatusProtocol, func(s network.Stream) {
		defer s.Close()
		handleStatus(s, handler)
	})

	bbr := func(s network.Stream) {
		defer s.Close()
		handleBlocksByRoot(s, handler)
	}
	h.SetStreamHandler(BlocksByRootProtocol, bbr)
	h.SetStreamHandler(BlocksByRootProtocolLegacy, bbr)
}

func handleStatus(s network.Stream, handler *ReqRespHandler) {
	if handler.OnStatus == nil {
		return
	}
	req, err := ReadStatus(s)
	if err != nil {
		reqrespLog.Debug("status read failed", "peer_id", s.Conn().RemotePeer().String(), "err", err)
		return
	}
	reqrespLog.Info("status request received",
		"peer_id", s.Conn().RemotePeer().String(),
		"remote_finalized_slot", req.Finalized.Slot,
		"remote_finalized_root", logging.LongHash(req.Finalized.Root),
		"remote_head_slot", req.Head.Slot,
		"remote_head_root", logging.LongHash(req.Head.Root),
	)
	resp := handler.OnStatus(req)
	if _, err := s.Write([]byte{ResponseSuccess}); err != nil {
		return
	}
	if err := WriteStatus(s, resp); err != nil {
		return
	}
	reqrespLog.Info("status response sent",
		"peer_id", s.Conn().RemotePeer().String(),
		"local_finalized_slot", resp.Finalized.Slot,
		"local_finalized_root", logging.LongHash(resp.Finalized.Root),
		"local_head_slot", resp.Head.Slot,
		"local_head_root", logging.LongHash(resp.Head.Root),
	)
}

func handleBlocksByRoot(s network.Stream, handler *ReqRespHandler) {
	if handler.OnBlocksByRoot == nil {
		return
	}
	start := time.Now()
	roots, err := readBlocksByRootRequest(s)
	if err != nil {
		reqrespLog.Debug("blocks_by_root read failed",
			"peer_id", s.Conn().RemotePeer().String(),
			"err", err,
		)
		return
	}
	metrics.BlocksByRootRequestsTotal.WithLabelValues("inbound").Inc()
	reqrespLog.Info("blocks_by_root request received",
		"peer_id", s.Conn().RemotePeer().String(),
		"roots_count", len(roots),
	)
	blocks := handler.OnBlocksByRoot(roots)
	reqrespLog.Info("blocks_by_root response sent",
		"peer_id", s.Conn().RemotePeer().String(),
		"blocks_count", len(blocks),
		"roots_requested", len(roots),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	for _, block := range blocks {
		if _, err := s.Write([]byte{ResponseSuccess}); err != nil {
			return
		}
		if err := writeSignedBlock(s, block); err != nil {
			return
		}
	}
}
