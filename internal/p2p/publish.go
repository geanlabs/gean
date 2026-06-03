package p2p

import (
	"context"
	"crypto/sha256"
	"fmt"
	"runtime/debug"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

const unknownBuildValue = "unknown"

var clientGitCommit = unknownBuildValue

func SetClientGitCommit(commit string) {
	if commit == "" {
		clientGitCommit = unknownBuildValue
		return
	}
	clientGitCommit = commit
}

type publishLogInfo struct {
	slot         uint64
	proposer     string
	blockRoot    [32]byte
	hasBlockRoot bool
}

func (h *Host) PublishBlock(ctx context.Context, block *types.SignedBlock) error {
	if block == nil {
		return fmt.Errorf("publish block: nil block")
	}
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	info := publishLogInfo{proposer: unknownBuildValue}
	if block.Block != nil {
		info.slot = block.Block.Slot
		info.proposer = fmt.Sprintf("%d", block.Block.ProposerIndex)
		if root, err := block.HashTreeRoot(); err == nil {
			info.blockRoot = root
			info.hasBlockRoot = true
		}
	}
	return h.publishToTopic(ctx, BlockTopic(), data, info)
}

func (h *Host) PublishAttestation(ctx context.Context, att *types.SignedAttestation, committeeCount uint64) error {
	if att == nil {
		return fmt.Errorf("publish attestation: nil attestation")
	}
	data, err := att.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}
	info := publishLogInfo{proposer: unknownBuildValue}
	if att.Data != nil {
		info.slot = att.Data.Slot
		if att.Data.Head != nil {
			info.blockRoot = att.Data.Head.Root
			info.hasBlockRoot = true
		}
	}
	subnet := SubnetID(att.ValidatorID, committeeCount)
	topic := AttestationSubnetTopic(subnet)
	return h.publishToTopic(ctx, topic, data, info)
}

func (h *Host) PublishAggregatedAttestation(ctx context.Context, agg *types.SignedAggregatedAttestation) error {
	if agg == nil {
		return fmt.Errorf("publish aggregation: nil aggregation")
	}
	data, err := agg.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal aggregation: %w", err)
	}
	info := publishLogInfo{proposer: unknownBuildValue}
	if agg.Data != nil {
		info.slot = agg.Data.Slot
		if agg.Data.Head != nil {
			info.blockRoot = agg.Data.Head.Root
			info.hasBlockRoot = true
		}
	}
	return h.publishToTopic(ctx, AggregationTopic(), data, info)
}

func (h *Host) publishToTopic(ctx context.Context, topic string, sszData []byte, info publishLogInfo) error {
	compressed := SnappyRawEncode(sszData)
	logPublish(topic, sszData, compressed, info)
	t, ok := h.topics[topic]
	if !ok {
		return fmt.Errorf("not subscribed to topic: %s", topic)
	}
	return t.Publish(ctx, compressed)
}

func logPublish(topic string, sszData, compressed []byte, info publishLogInfo) {
	sszHash := sha256.Sum256(sszData)
	compressedHash := sha256.Sum256(compressed)
	_, decodeErr := SnappyRawDecode(compressed)
	messageID := ComputeMessageID(topic, compressed)
	logger.Info(logger.Gossip,
		"publish gossip topic=%s slot=%d proposer=%s block_root=%s sha256_ssz=0x%x sha256_compressed=0x%x compressed_len=%d snappy_self_decode_ok=%t message_id=0x%x client_git_sha=%s snappy=%s",
		topic,
		info.slot,
		info.proposer,
		formatOptionalRoot(info.blockRoot, info.hasBlockRoot),
		sszHash,
		compressedHash,
		len(compressed),
		decodeErr == nil,
		messageID,
		clientGitCommit,
		snappyBuildVersion(),
	)
}

func formatOptionalRoot(root [32]byte, ok bool) string {
	if !ok {
		return unknownBuildValue
	}
	return fmt.Sprintf("0x%x", root)
}

func snappyBuildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "github.com/golang/snappy@unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/golang/snappy" {
			version := dep.Version
			if dep.Replace != nil {
				version = dep.Replace.Version
			}
			if version == "" {
				version = unknownBuildValue
			}
			return dep.Path + "@" + version
		}
	}
	return "github.com/golang/snappy@unknown"
}
