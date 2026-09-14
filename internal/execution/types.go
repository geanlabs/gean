package execution

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/geanlabs/gean/internal/types"
)

// Engine API wire types. Field names and encodings follow execution-apis:
// DATA fields are 0x-prefixed hex of fixed width, QUANTITY fields are
// 0x-prefixed hex with no leading zeros. Consensus types stay free of this;
// the payload is converted at the boundary.

// Payload verdicts an execution client returns.
const (
	StatusValid            = "VALID"
	StatusInvalid          = "INVALID"
	StatusSyncing          = "SYNCING"
	StatusAccepted         = "ACCEPTED"
	StatusInvalidBlockHash = "INVALID_BLOCK_HASH"
)

// Hash is a 32-byte DATA field.
type Hash [32]byte

func (h Hash) MarshalJSON() ([]byte, error) { return json.Marshal("0x" + hex.EncodeToString(h[:])) }

func (h *Hash) UnmarshalJSON(data []byte) error { return unmarshalFixed(data, h[:], "hash") }

// Address is a 20-byte DATA field.
type Address [20]byte

func (a Address) MarshalJSON() ([]byte, error) { return json.Marshal("0x" + hex.EncodeToString(a[:])) }

func (a *Address) UnmarshalJSON(data []byte) error { return unmarshalFixed(data, a[:], "address") }

// Bloom is the 256-byte logs bloom DATA field.
type Bloom [256]byte

func (b Bloom) MarshalJSON() ([]byte, error) { return json.Marshal("0x" + hex.EncodeToString(b[:])) }

func (b *Bloom) UnmarshalJSON(data []byte) error { return unmarshalFixed(data, b[:], "logs bloom") }

// PayloadID is the 8-byte build handle returned by a forkchoice update.
type PayloadID [8]byte

func (p PayloadID) MarshalJSON() ([]byte, error) {
	return json.Marshal("0x" + hex.EncodeToString(p[:]))
}

func (p *PayloadID) UnmarshalJSON(data []byte) error { return unmarshalFixed(data, p[:], "payload id") }

func (p PayloadID) String() string { return "0x" + hex.EncodeToString(p[:]) }

// Bytes is a variable-length DATA field.
type Bytes []byte

func (b Bytes) MarshalJSON() ([]byte, error) { return json.Marshal("0x" + hex.EncodeToString(b)) }

func (b *Bytes) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return fmt.Errorf("bytes: %w", err)
	}
	*b = raw
	return nil
}

// Quantity is a uint64 QUANTITY field.
type Quantity uint64

func (q Quantity) MarshalJSON() ([]byte, error) { return json.Marshal(fmt.Sprintf("0x%x", uint64(q))) }

func (q *Quantity) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 16)
	if !ok || v.Sign() < 0 || v.BitLen() > 64 {
		return fmt.Errorf("quantity out of range: %q", s)
	}
	*q = Quantity(v.Uint64())
	return nil
}

// U256 is a 256-bit QUANTITY. SSZ stores it little-endian in 32 bytes; the
// wire carries it as a big-endian hex number.
type U256 [32]byte

func (u U256) MarshalJSON() ([]byte, error) {
	be := make([]byte, 32)
	for i := range be {
		be[i] = u[31-i]
	}
	return json.Marshal("0x" + new(big.Int).SetBytes(be).Text(16))
}

func (u *U256) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 16)
	if !ok || v.Sign() < 0 || v.BitLen() > 256 {
		return fmt.Errorf("u256 out of range: %q", s)
	}
	be := v.FillBytes(make([]byte, 32))
	for i := range u {
		u[i] = be[31-i]
	}
	return nil
}

func unmarshalFixed(data []byte, dst []byte, what string) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if len(raw) != len(dst) {
		return fmt.Errorf("%s: %d bytes, want %d", what, len(raw), len(dst))
	}
	copy(dst, raw)
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
