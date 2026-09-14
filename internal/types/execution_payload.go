package types

import (
	ssz "github.com/ferranbt/fastssz"
)

// Execution-payload bounds, per the execution-apis Cancun schema. They size the
// SSZ lists and are consensus constants shared with every client that carries
// a payload in the block body.
const (
	AddressSize               = 20
	BytesPerLogsBloom         = 256
	MaxExtraDataBytes         = 32
	MaxBytesPerTransaction    = 1 << 30
	MaxTransactionsPerPayload = 1 << 20
	MaxWithdrawalsPerPayload  = 16
)

type Withdrawal struct {
	Index          uint64            `json:"index"`
	ValidatorIndex uint64            `json:"validator_index"`
	Address        [AddressSize]byte `json:"address" ssz-size:"20"`
	Amount         uint64            `json:"amount"`
}

// ExecutionPayload is the Cancun ExecutionPayloadV3 container, embedded verbatim
// so the block body can be handed to an execution layer unchanged. BaseFeePerGas
// is a little-endian uint256 as SSZ encodes it.
type ExecutionPayload struct {
	ParentHash    [RootSize]byte          `json:"parent_hash" ssz-size:"32"`
	FeeRecipient  [AddressSize]byte       `json:"fee_recipient" ssz-size:"20"`
	StateRoot     [RootSize]byte          `json:"state_root" ssz-size:"32"`
	ReceiptsRoot  [RootSize]byte          `json:"receipts_root" ssz-size:"32"`
	LogsBloom     [BytesPerLogsBloom]byte `json:"logs_bloom" ssz-size:"256"`
	PrevRandao    [RootSize]byte          `json:"prev_randao" ssz-size:"32"`
	BlockNumber   uint64                  `json:"block_number"`
	GasLimit      uint64                  `json:"gas_limit"`
	GasUsed       uint64                  `json:"gas_used"`
	Timestamp     uint64                  `json:"timestamp"`
	ExtraData     []byte                  `json:"extra_data" ssz-max:"32"`
	BaseFeePerGas [RootSize]byte          `json:"base_fee_per_gas" ssz-size:"32"`
	BlockHash     [RootSize]byte          `json:"block_hash" ssz-size:"32"`
	Transactions  [][]byte                `json:"transactions" ssz-size:"?,?" ssz-max:"1048576,1073741824"`
	Withdrawals   []*Withdrawal           `json:"withdrawals" ssz-max:"16"`
	BlobGasUsed   uint64                  `json:"blob_gas_used"`
	ExcessBlobGas uint64                  `json:"excess_blob_gas"`
}

// ExecutionPayloadHeader is the payload with its two variable lists collapsed to
// their SSZ roots; the state caches one so the next block can be checked
// against the last applied payload without keeping the payload itself.
type ExecutionPayloadHeader struct {
	ParentHash       [RootSize]byte          `json:"parent_hash" ssz-size:"32"`
	FeeRecipient     [AddressSize]byte       `json:"fee_recipient" ssz-size:"20"`
	StateRoot        [RootSize]byte          `json:"state_root" ssz-size:"32"`
	ReceiptsRoot     [RootSize]byte          `json:"receipts_root" ssz-size:"32"`
	LogsBloom        [BytesPerLogsBloom]byte `json:"logs_bloom" ssz-size:"256"`
	PrevRandao       [RootSize]byte          `json:"prev_randao" ssz-size:"32"`
	BlockNumber      uint64                  `json:"block_number"`
	GasLimit         uint64                  `json:"gas_limit"`
	GasUsed          uint64                  `json:"gas_used"`
	Timestamp        uint64                  `json:"timestamp"`
	ExtraData        []byte                  `json:"extra_data" ssz-max:"32"`
	BaseFeePerGas    [RootSize]byte          `json:"base_fee_per_gas" ssz-size:"32"`
	BlockHash        [RootSize]byte          `json:"block_hash" ssz-size:"32"`
	TransactionsRoot [RootSize]byte          `json:"transactions_root" ssz-size:"32"`
	WithdrawalsRoot  [RootSize]byte          `json:"withdrawals_root" ssz-size:"32"`
	BlobGasUsed      uint64                  `json:"blob_gas_used"`
	ExcessBlobGas    uint64                  `json:"excess_blob_gas"`
}

// IsZero reports whether the payload is the all-zero value, which is what a
// block carries on a network with no execution layer. A nil payload hashes and
// serializes identically to the zero value, so it counts as zero too.
func (p *ExecutionPayload) IsZero() bool {
	if p == nil {
		return true
	}
	return p.ParentHash == ZeroRoot &&
		p.FeeRecipient == [AddressSize]byte{} &&
		p.StateRoot == ZeroRoot &&
		p.ReceiptsRoot == ZeroRoot &&
		p.LogsBloom == [BytesPerLogsBloom]byte{} &&
		p.PrevRandao == ZeroRoot &&
		p.BlockNumber == 0 &&
		p.GasLimit == 0 &&
		p.GasUsed == 0 &&
		p.Timestamp == 0 &&
		len(p.ExtraData) == 0 &&
		p.BaseFeePerGas == ZeroRoot &&
		p.BlockHash == ZeroRoot &&
		len(p.Transactions) == 0 &&
		len(p.Withdrawals) == 0 &&
		p.BlobGasUsed == 0 &&
		p.ExcessBlobGas == 0
}

// ToHeader projects the payload onto the header the state caches. The two list
// roots follow the SSZ rules for List[ByteList[MaxBytesPerTransaction],
// MaxTransactionsPerPayload] and List[Withdrawal, MaxWithdrawalsPerPayload];
// every other field is copied.
func (p *ExecutionPayload) ToHeader() (*ExecutionPayloadHeader, error) {
	if p == nil {
		p = &ExecutionPayload{}
	}
	transactionsRoot, err := TransactionsRoot(p.Transactions)
	if err != nil {
		return nil, err
	}
	withdrawalsRoot, err := WithdrawalsRoot(p.Withdrawals)
	if err != nil {
		return nil, err
	}
	return &ExecutionPayloadHeader{
		ParentHash:       p.ParentHash,
		FeeRecipient:     p.FeeRecipient,
		StateRoot:        p.StateRoot,
		ReceiptsRoot:     p.ReceiptsRoot,
		LogsBloom:        p.LogsBloom,
		PrevRandao:       p.PrevRandao,
		BlockNumber:      p.BlockNumber,
		GasLimit:         p.GasLimit,
		GasUsed:          p.GasUsed,
		Timestamp:        p.Timestamp,
		ExtraData:        append([]byte(nil), p.ExtraData...),
		BaseFeePerGas:    p.BaseFeePerGas,
		BlockHash:        p.BlockHash,
		TransactionsRoot: transactionsRoot,
		WithdrawalsRoot:  withdrawalsRoot,
		BlobGasUsed:      p.BlobGasUsed,
		ExcessBlobGas:    p.ExcessBlobGas,
	}, nil
}

// TransactionsRoot is the hash_tree_root of the transactions list on its own,
// which is what the header commits to.
func TransactionsRoot(transactions [][]byte) ([RootSize]byte, error) {
	if len(transactions) > MaxTransactionsPerPayload {
		return ZeroRoot, ssz.ErrIncorrectListSize
	}
	hh := ssz.NewHasher()
	listIndex := hh.Index()
	for _, tx := range transactions {
		if len(tx) > MaxBytesPerTransaction {
			return ZeroRoot, ssz.ErrIncorrectListSize
		}
		elemIndex := hh.Index()
		hh.Append(tx)
		hh.MerkleizeWithMixin(elemIndex, uint64(len(tx)), (MaxBytesPerTransaction+31)/32)
	}
	hh.MerkleizeWithMixin(listIndex, uint64(len(transactions)), MaxTransactionsPerPayload)
	return hh.HashRoot()
}

// WithdrawalsRoot is the hash_tree_root of the withdrawals list on its own.
func WithdrawalsRoot(withdrawals []*Withdrawal) ([RootSize]byte, error) {
	if len(withdrawals) > MaxWithdrawalsPerPayload {
		return ZeroRoot, ssz.ErrIncorrectListSize
	}
	hh := ssz.NewHasher()
	listIndex := hh.Index()
	for _, w := range withdrawals {
		if w == nil {
			w = &Withdrawal{}
		}
		if err := w.HashTreeRootWith(hh); err != nil {
			return ZeroRoot, err
		}
	}
	hh.MerkleizeWithMixin(listIndex, uint64(len(withdrawals)), MaxWithdrawalsPerPayload)
	return hh.HashRoot()
}
