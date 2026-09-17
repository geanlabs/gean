package execution

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/geanlabs/gean/internal/types"
)

// The Engine API types are go-ethereum's own. They carry the wire encodings
// for the remote client and are what an in-process geth consumes directly,
// so gean adds only the conversion between its SSZ payload and geth's
// executable data.
type (
	Hash                    = common.Hash
	PayloadID               = engine.PayloadID
	ForkchoiceState         = engine.ForkchoiceStateV1
	PayloadAttributes       = engine.PayloadAttributes
	PayloadStatus           = engine.PayloadStatusV1
	ForkchoiceUpdatedResult = engine.ForkChoiceResponse
)

// Payload verdicts, as the specification spells them. geth no longer returns
// INVALID_BLOCK_HASH, but other execution clients still send it.
const (
	StatusValid            = "VALID"
	StatusInvalid          = "INVALID"
	StatusSyncing          = "SYNCING"
	StatusAccepted         = "ACCEPTED"
	StatusInvalidBlockHash = "INVALID_BLOCK_HASH"
)

// NewPayloadAttributes describes the build gean asks for: no withdrawals, no
// RANDAO mix, and the consensus parent root as the beacon root. Cancun-era
// clients require the withdrawals list and the beacon root to be present
// rather than null, so both are set even though one is empty.
func NewPayloadAttributes(timestamp uint64, feeRecipient [types.AddressSize]byte, parentRoot [32]byte) *PayloadAttributes {
	beaconRoot := common.Hash(parentRoot)
	return &PayloadAttributes{
		Timestamp:             timestamp,
		SuggestedFeeRecipient: feeRecipient,
		Withdrawals:           []*gethtypes.Withdrawal{},
		BeaconRoot:            &beaconRoot,
	}
}

// ToExecutableData converts the consensus payload for the Engine API. The
// base fee is little-endian in SSZ and a big-endian integer in geth, and the
// Cancun blob-gas fields are pointers that must be present.
func ToExecutableData(p *types.ExecutionPayload) *engine.ExecutableData {
	if p == nil {
		p = &types.ExecutionPayload{}
	}
	baseFee := slices.Clone(p.BaseFeePerGas[:])
	slices.Reverse(baseFee)
	data := &engine.ExecutableData{
		ParentHash:    p.ParentHash,
		FeeRecipient:  p.FeeRecipient,
		StateRoot:     p.StateRoot,
		ReceiptsRoot:  p.ReceiptsRoot,
		LogsBloom:     slices.Clone(p.LogsBloom[:]),
		Random:        p.PrevRandao,
		Number:        p.BlockNumber,
		GasLimit:      p.GasLimit,
		GasUsed:       p.GasUsed,
		Timestamp:     p.Timestamp,
		ExtraData:     slices.Clone(p.ExtraData),
		BaseFeePerGas: new(big.Int).SetBytes(baseFee),
		BlockHash:     p.BlockHash,
		Transactions:  make([][]byte, len(p.Transactions)),
		Withdrawals:   make([]*gethtypes.Withdrawal, 0, len(p.Withdrawals)),
		BlobGasUsed:   new(uint64),
		ExcessBlobGas: new(uint64),
	}
	*data.BlobGasUsed = p.BlobGasUsed
	*data.ExcessBlobGas = p.ExcessBlobGas
	for i, tx := range p.Transactions {
		data.Transactions[i] = slices.Clone(tx)
	}
	for _, w := range p.Withdrawals {
		if w == nil {
			continue
		}
		data.Withdrawals = append(data.Withdrawals, &gethtypes.Withdrawal{
			Index: w.Index, Validator: w.ValidatorIndex, Address: w.Address, Amount: w.Amount,
		})
	}
	return data
}

// FromExecutableData converts a built or received payload into the consensus
// type, rejecting shapes the SSZ container cannot hold.
func FromExecutableData(d *engine.ExecutableData) (*types.ExecutionPayload, error) {
	if d == nil {
		return nil, fmt.Errorf("executable data is nil")
	}
	if len(d.LogsBloom) != types.BytesPerLogsBloom {
		return nil, fmt.Errorf("logs bloom is %d bytes, want %d", len(d.LogsBloom), types.BytesPerLogsBloom)
	}
	if len(d.ExtraData) > types.MaxExtraDataBytes {
		return nil, fmt.Errorf("extra data is %d bytes, max %d", len(d.ExtraData), types.MaxExtraDataBytes)
	}
	p := &types.ExecutionPayload{
		ParentHash:   d.ParentHash,
		FeeRecipient: d.FeeRecipient,
		StateRoot:    d.StateRoot,
		ReceiptsRoot: d.ReceiptsRoot,
		PrevRandao:   d.Random,
		BlockNumber:  d.Number,
		GasLimit:     d.GasLimit,
		GasUsed:      d.GasUsed,
		Timestamp:    d.Timestamp,
		ExtraData:    slices.Clone(d.ExtraData),
		BlockHash:    d.BlockHash,
		Transactions: make([][]byte, len(d.Transactions)),
		Withdrawals:  make([]*types.Withdrawal, len(d.Withdrawals)),
	}
	copy(p.LogsBloom[:], d.LogsBloom)
	if d.BaseFeePerGas != nil {
		if d.BaseFeePerGas.Sign() < 0 || d.BaseFeePerGas.BitLen() > 256 {
			return nil, fmt.Errorf("base fee %s does not fit 256 bits", d.BaseFeePerGas)
		}
		d.BaseFeePerGas.FillBytes(p.BaseFeePerGas[:])
		slices.Reverse(p.BaseFeePerGas[:])
	}
	if d.BlobGasUsed != nil {
		p.BlobGasUsed = *d.BlobGasUsed
	}
	if d.ExcessBlobGas != nil {
		p.ExcessBlobGas = *d.ExcessBlobGas
	}
	for i, tx := range d.Transactions {
		p.Transactions[i] = slices.Clone(tx)
	}
	for i, w := range d.Withdrawals {
		if w == nil {
			return nil, fmt.Errorf("withdrawal %d is nil", i)
		}
		p.Withdrawals[i] = &types.Withdrawal{
			Index: w.Index, ValidatorIndex: w.Validator, Address: w.Address, Amount: w.Amount,
		}
	}
	return p, nil
}
