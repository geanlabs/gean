package execution

import (
	"encoding/json"
	"math/big"
	"reflect"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/geanlabs/gean/internal/types"
)

// Engine API hex encodings stay at this boundary; consensus uses SSZ types.

// Payload verdicts an execution client returns.
const (
	StatusValid            = "VALID"
	StatusInvalid          = "INVALID"
	StatusSyncing          = "SYNCING"
	StatusAccepted         = "ACCEPTED"
	StatusInvalidBlockHash = "INVALID_BLOCK_HASH"
)

type Hash = common.Hash
type Address = common.Address
type Bytes = hexutil.Bytes
type Quantity = hexutil.Uint64

// Bloom is the 256-byte logs bloom DATA field.
type Bloom [256]byte

func (b Bloom) MarshalText() ([]byte, error) { return hexutil.Bytes(b[:]).MarshalText() }

func (b *Bloom) UnmarshalJSON(data []byte) error {
	return hexutil.UnmarshalFixedJSON(reflect.TypeOf(*b), data, b[:])
}

// PayloadID is the 8-byte build handle returned by a forkchoice update.
type PayloadID [8]byte

func (p PayloadID) MarshalText() ([]byte, error) { return hexutil.Bytes(p[:]).MarshalText() }

func (p *PayloadID) UnmarshalJSON(data []byte) error {
	return hexutil.UnmarshalFixedJSON(reflect.TypeOf(*p), data, p[:])
}

func (p PayloadID) String() string { return hexutil.Encode(p[:]) }

// U256 is a 256-bit QUANTITY. SSZ stores it little-endian in 32 bytes; the
// wire carries it as a big-endian hex number.
type U256 [32]byte

func (u U256) MarshalText() ([]byte, error) {
	slices.Reverse(u[:])
	return (*hexutil.Big)(new(big.Int).SetBytes(u[:])).MarshalText()
}

func (u *U256) UnmarshalJSON(data []byte) error {
	var n hexutil.Big
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	n.ToInt().FillBytes(u[:])
	slices.Reverse(u[:])
	return nil
}

// ForkchoiceState is the head, safe, and finalized execution block hashes.
type ForkchoiceState struct {
	HeadBlockHash      Hash `json:"headBlockHash"`
	SafeBlockHash      Hash `json:"safeBlockHash"`
	FinalizedBlockHash Hash `json:"finalizedBlockHash"`
}

// PayloadAttributes asks the execution client to start building a payload.
type PayloadAttributes struct {
	Timestamp             Quantity     `json:"timestamp"`
	PrevRandao            Hash         `json:"prevRandao"`
	SuggestedFeeRecipient Address      `json:"suggestedFeeRecipient"`
	Withdrawals           []Withdrawal `json:"withdrawals"`
	ParentBeaconBlockRoot Hash         `json:"parentBeaconBlockRoot"`
}

// Withdrawal is the wire form of a payload withdrawal.
type Withdrawal struct {
	Index          Quantity `json:"index"`
	ValidatorIndex Quantity `json:"validatorIndex"`
	Address        Address  `json:"address"`
	Amount         Quantity `json:"amount"`
}

// PayloadStatus is the execution client's verdict on a payload or a
// forkchoice update.
type PayloadStatus struct {
	Status          string  `json:"status"`
	LatestValidHash *Hash   `json:"latestValidHash"`
	ValidationError *string `json:"validationError"`
}

// ForkchoiceUpdatedResult is the reply to engine_forkchoiceUpdated.
type ForkchoiceUpdatedResult struct {
	PayloadStatus PayloadStatus `json:"payloadStatus"`
	PayloadID     *PayloadID    `json:"payloadId"`
}

// Payload is the wire form of ExecutionPayloadV3.
type Payload struct {
	ParentHash    Hash         `json:"parentHash"`
	FeeRecipient  Address      `json:"feeRecipient"`
	StateRoot     Hash         `json:"stateRoot"`
	ReceiptsRoot  Hash         `json:"receiptsRoot"`
	LogsBloom     Bloom        `json:"logsBloom"`
	PrevRandao    Hash         `json:"prevRandao"`
	BlockNumber   Quantity     `json:"blockNumber"`
	GasLimit      Quantity     `json:"gasLimit"`
	GasUsed       Quantity     `json:"gasUsed"`
	Timestamp     Quantity     `json:"timestamp"`
	ExtraData     Bytes        `json:"extraData"`
	BaseFeePerGas U256         `json:"baseFeePerGas"`
	BlockHash     Hash         `json:"blockHash"`
	Transactions  []Bytes      `json:"transactions"`
	Withdrawals   []Withdrawal `json:"withdrawals"`
	BlobGasUsed   Quantity     `json:"blobGasUsed"`
	ExcessBlobGas Quantity     `json:"excessBlobGas"`
}

// PayloadToWire converts the consensus payload for the engine API.
func PayloadToWire(p *types.ExecutionPayload) *Payload {
	if p == nil {
		p = &types.ExecutionPayload{}
	}
	w := &Payload{
		ParentHash:    p.ParentHash,
		FeeRecipient:  p.FeeRecipient,
		StateRoot:     p.StateRoot,
		ReceiptsRoot:  p.ReceiptsRoot,
		LogsBloom:     p.LogsBloom,
		PrevRandao:    p.PrevRandao,
		BlockNumber:   Quantity(p.BlockNumber),
		GasLimit:      Quantity(p.GasLimit),
		GasUsed:       Quantity(p.GasUsed),
		Timestamp:     Quantity(p.Timestamp),
		ExtraData:     Bytes(append([]byte(nil), p.ExtraData...)),
		BaseFeePerGas: p.BaseFeePerGas,
		BlockHash:     p.BlockHash,
		Transactions:  make([]Bytes, len(p.Transactions)),
		Withdrawals:   make([]Withdrawal, len(p.Withdrawals)),
		BlobGasUsed:   Quantity(p.BlobGasUsed),
		ExcessBlobGas: Quantity(p.ExcessBlobGas),
	}
	for i, tx := range p.Transactions {
		w.Transactions[i] = Bytes(append([]byte(nil), tx...))
	}
	for i, wd := range p.Withdrawals {
		if wd == nil {
			continue
		}
		w.Withdrawals[i] = Withdrawal{
			Index:          Quantity(wd.Index),
			ValidatorIndex: Quantity(wd.ValidatorIndex),
			Address:        wd.Address,
			Amount:         Quantity(wd.Amount),
		}
	}
	return w
}

// PayloadFromWire converts an engine API payload into the consensus type.
func PayloadFromWire(w *Payload) *types.ExecutionPayload {
	if w == nil {
		return &types.ExecutionPayload{}
	}
	p := &types.ExecutionPayload{
		ParentHash:    w.ParentHash,
		FeeRecipient:  w.FeeRecipient,
		StateRoot:     w.StateRoot,
		ReceiptsRoot:  w.ReceiptsRoot,
		LogsBloom:     w.LogsBloom,
		PrevRandao:    w.PrevRandao,
		BlockNumber:   uint64(w.BlockNumber),
		GasLimit:      uint64(w.GasLimit),
		GasUsed:       uint64(w.GasUsed),
		Timestamp:     uint64(w.Timestamp),
		ExtraData:     append([]byte(nil), w.ExtraData...),
		BaseFeePerGas: w.BaseFeePerGas,
		BlockHash:     w.BlockHash,
		Transactions:  make([][]byte, len(w.Transactions)),
		Withdrawals:   make([]*types.Withdrawal, len(w.Withdrawals)),
		BlobGasUsed:   uint64(w.BlobGasUsed),
		ExcessBlobGas: uint64(w.ExcessBlobGas),
	}
	for i, tx := range w.Transactions {
		p.Transactions[i] = append([]byte(nil), tx...)
	}
	for i, wd := range w.Withdrawals {
		p.Withdrawals[i] = &types.Withdrawal{
			Index:          uint64(wd.Index),
			ValidatorIndex: uint64(wd.ValidatorIndex),
			Address:        wd.Address,
			Amount:         uint64(wd.Amount),
		}
	}
	return p
}
