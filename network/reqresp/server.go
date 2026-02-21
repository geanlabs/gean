package reqresp

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
)

// RegisterReqResp registers request/response protocol handlers.
func RegisterReqResp(h host.Host, handler *ReqRespHandler) {
	h.SetStreamHandler(StatusProtocol, func(s network.Stream) {
		defer s.Close()
		handleStatus(s, handler)
	})

	h.SetStreamHandler(BlocksByRootProtocol, func(s network.Stream) {
		defer s.Close()
		handleBlocksByRoot(s, handler)
	})
}

func handleStatus(s network.Stream, handler *ReqRespHandler) {
	if handler.OnStatus == nil {
		return
	}
	req, err := ReadStatus(s)
	if err != nil {
		return
	}
	resp := handler.OnStatus(req)
	if _, err := s.Write([]byte{ResponseSuccess}); err != nil {
		return
	}
	if err := WriteStatus(s, resp); err != nil {
		return
	}
}

func handleBlocksByRoot(s network.Stream, handler *ReqRespHandler) {
	if handler.OnBlocksByRoot == nil {
		return
	}
	roots, err := readBlocksByRootRequest(s)
	if err != nil {
		return
	}
	blocks := handler.OnBlocksByRoot(roots)
	for _, block := range blocks {
		if _, err := s.Write([]byte{ResponseSuccess}); err != nil {
			return
		}
		if err := writeSignedBlock(s, block); err != nil {
			return
		}
	}
}
