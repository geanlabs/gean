// lifecycle.go contains networking service lifecycle entrypoints.
package networking

func (s *Service) Start() {
	s.wg.Add(2)
	go s.processBlocks()
	go s.processAttestations()

	if len(s.failedBootnodes) > 0 {
		s.wg.Add(1)
		go s.retryBootnodes()
	}

	s.logger.Info("networking service started",
		"peer_id", s.host.ID(),
		"addrs", s.host.Addrs(),
	)
}

// Stop shuts down the networking service.
func (s *Service) Stop() {
	s.cancel()
	s.blockSub.Cancel()
	s.attestationSub.Cancel()
	s.wg.Wait()
	s.host.Close()
	s.logger.Info("networking service stopped")
}

// PeerCount returns the number of connected peers.
func (s *Service) PeerCount() int {
	return len(s.host.Network().Peers())
}
