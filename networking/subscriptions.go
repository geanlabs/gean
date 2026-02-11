// subscriptions.go contains inbound gossip subscription worker loops.
package networking

// processBlocks handles incoming block messages.
func (s *Service) processBlocks() {
	defer s.wg.Done()

	for {
		msg, err := s.blockSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("block subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleBlockMessage(s.ctx, msg.Data, msg.ReceivedFrom); err != nil {
				s.logger.Error("handle block error", "error", err)
			}
		}
	}
}

// processAttestations handles incoming attestation messages.
func (s *Service) processAttestations() {
	defer s.wg.Done()

	for {
		msg, err := s.attestationSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("attestation subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleAttestationMessage(s.ctx, msg.Data); err != nil {
				s.logger.Error("handle attestation error", "error", err)
			}
		}
	}
}
