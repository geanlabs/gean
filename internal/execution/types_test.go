package execution

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/beacon/engine"
	gethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/geanlabs/gean/internal/types"
)

func samplePayload() *types.ExecutionPayload {
	p := &types.ExecutionPayload{
		ParentHash:    [32]byte{1},
		FeeRecipient:  [types.AddressSize]byte{2},
		StateRoot:     [32]byte{3},
		ReceiptsRoot:  [32]byte{4},
		PrevRandao:    [32]byte{5},
		BlockNumber:   6,
		GasLimit:      30_000_000,
		GasUsed:       21_000,
		Timestamp:     1_700_000_004,
		ExtraData:     []byte("gean"),
		BaseFeePerGas: [32]byte{7},
		BlockHash:     [32]byte{8},
		Transactions:  [][]byte{{0x02, 0x01}, {}},
		Withdrawals:   []*types.Withdrawal{{Index: 1, ValidatorIndex: 2, Address: [types.AddressSize]byte{9}, Amount: 10}},
		BlobGasUsed:   11,
		ExcessBlobGas: 12,
	}
	p.LogsBloom[255] = 0xff
	return p
}

func TestExecutableDataRoundTrip(t *testing.T) {
	original := samplePayload()
	data := ToExecutableData(original)

	// SSZ little-endian 7 is the integer 7; the Cancun pointers are present.
	if data.BaseFeePerGas.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("base fee: %s", data.BaseFeePerGas)
	}
	if data.BlobGasUsed == nil || *data.BlobGasUsed != 11 || data.ExcessBlobGas == nil || *data.ExcessBlobGas != 12 {
		t.Fatal("blob gas pointers must be set")
	}
	if len(data.Withdrawals) != 1 || data.Withdrawals[0].Validator != 2 {
		t.Fatalf("withdrawals: %+v", data.Withdrawals)
	}

	// geth's own codec produces the wire form the remote client sends.
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"blockNumber":"0x6"`, `"extraData":"0x6765616e"`, `"baseFeePerGas":"0x7"`, `"transactions":["0x0201","0x"]`, `"validatorIndex":"0x2"`, `"prevRandao":"0x05`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("executable data json lacks %s: %s", want, encoded)
		}
	}

	back, err := FromExecutableData(data)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := original.HashTreeRoot()
	gotRoot, _ := back.HashTreeRoot()
	if wantRoot != gotRoot {
		t.Fatal("payload changed across the conversion round trip")
	}
	if len(back.Transactions) != 2 || len(back.Transactions[1]) != 0 {
		t.Fatalf("transactions not preserved: %v", back.Transactions)
	}

	// Neither direction may alias the other's slices.
	data.ExtraData[0] = 'x'
	if original.ExtraData[0] == 'x' {
		t.Fatal("executable data shares extra data with the consensus payload")
	}
}

func TestFromExecutableDataRejectsUnrepresentable(t *testing.T) {
	good := ToExecutableData(samplePayload())
	for name, mutate := range map[string]func(d *engine.ExecutableData){
		"short bloom":         func(d *engine.ExecutableData) { d.LogsBloom = d.LogsBloom[:255] },
		"extra data too long": func(d *engine.ExecutableData) { d.ExtraData = make([]byte, types.MaxExtraDataBytes+1) },
		"base fee too wide":   func(d *engine.ExecutableData) { d.BaseFeePerGas = new(big.Int).Lsh(big.NewInt(1), 256) },
		"nil withdrawal":      func(d *engine.ExecutableData) { d.Withdrawals = append(d.Withdrawals, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			d := *good
			d.LogsBloom = append([]byte(nil), good.LogsBloom...)
			d.Withdrawals = append([]*gethtypes.Withdrawal(nil), good.Withdrawals...)
			mutate(&d)
			if _, err := FromExecutableData(&d); err == nil {
				t.Fatal("expected a conversion error")
			}
		})
	}
	if _, err := FromExecutableData(nil); err == nil {
		t.Fatal("nil data must be rejected")
	}
	// Missing optional fields decode to zero rather than failing.
	minimal := &engine.ExecutableData{LogsBloom: make([]byte, types.BytesPerLogsBloom)}
	p, err := FromExecutableData(minimal)
	if err != nil || !p.IsZero() {
		t.Fatalf("minimal data should be the zero payload: %v %v", p, err)
	}
}

func TestNewPayloadAttributesIsCancunShaped(t *testing.T) {
	attrs := NewPayloadAttributes(1_700_000_004, [types.AddressSize]byte{0xaa}, [32]byte{0xbb})
	if attrs.Withdrawals == nil || attrs.BeaconRoot == nil || attrs.BeaconRoot[0] != 0xbb {
		t.Fatalf("attributes: %+v", attrs)
	}
	encoded, err := json.Marshal(attrs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"timestamp":"0x6553f104"`, `"withdrawals":[]`, `"suggestedFeeRecipient":"0xaa`, `"parentBeaconBlockRoot":"0xbb`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("attributes json lacks %s: %s", want, encoded)
		}
	}
}
