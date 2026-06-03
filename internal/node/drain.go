package node

import "github.com/geanlabs/gean/internal/logger"

func (e *Engine) drainPendingBlocks() {
	drained := 0
	for {
		select {
		case block := <-e.BlockCh:
			e.onBlock(block)
			drained++
		default:
			if drained > 0 {
				logger.Info(logger.Chain, "drained %d pending blocks before attestation", drained)
			}
			return
		}
	}
}
