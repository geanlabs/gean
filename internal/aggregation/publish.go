package aggregation

import (
	"context"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func publishAggregates(ctx context.Context, publisher Publisher, aggs []*types.SignedAggregatedAttestation) {
	if publisher == nil {
		return
	}
	for _, agg := range aggs {
		if err := publisher.PublishAggregatedAttestation(ctx, agg); err != nil {
			logger.Warn(logger.Signature, "publish aggregated attestation failed: %v", err)
		}
	}
}
