package execution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestU256Encoding(t *testing.T) {
	// SSZ little-endian 7 == wire "0x7".
	var u U256
	u[0] = 7
	got, _ := json.Marshal(u)
	if string(got) != `"0x7"` {
		t.Fatalf("got %s", got)
	}
	var back U256
	if err := json.Unmarshal([]byte(`"0x100000000000000000000000000000000000000000000000000000000000000"`), &back); err != nil {
		t.Fatal(err)
	}
	if back[31] != 1 || back[0] != 0 {
		t.Fatalf("big-endian wire value must land in the high little-endian byte: %x", back)
	}
}

func TestFixedFieldsRejectWrongWidth(t *testing.T) {
	for _, field := range []any{new(Bloom), new(PayloadID)} {
		if err := json.Unmarshal([]byte(`"0x1234"`), field); err == nil {
			t.Fatalf("short %T accepted", field)
		}
	}
}

func TestPayloadAttributesWire(t *testing.T) {
	attrs := PayloadAttributes{
		Timestamp:             Quantity(1_700_000_004),
		SuggestedFeeRecipient: Address{0xaa},
		Withdrawals:           []Withdrawal{},
		ParentBeaconBlockRoot: Hash{0xbb},
	}
	got, err := json.Marshal(attrs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"timestamp":"0x6553f104"`,
		`"prevRandao":"0x` + strings.Repeat("00", 32) + `"`,
		`"suggestedFeeRecipient":"0xaa` + strings.Repeat("00", 19) + `"`,
		`"withdrawals":[]`,
		`"parentBeaconBlockRoot":"0xbb` + strings.Repeat("00", 31) + `"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("attributes json %s lacks %s", got, want)
		}
	}
}

func TestPayloadWireRoundTrip(t *testing.T) {
	original := &types.ExecutionPayload{
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
	original.LogsBloom[255] = 0xff

	wire := PayloadToWire(original)
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"blockNumber":"0x6"`, `"extraData":"0x6765616e"`, `"baseFeePerGas":"0x7"`, `"transactions":["0x0201","0x"]`, `"validatorIndex":"0x2"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("payload json lacks %s: %s", want, encoded)
		}
	}
	var decoded Payload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	back := PayloadFromWire(&decoded)
	wantRoot, _ := original.HashTreeRoot()
	gotRoot, _ := back.HashTreeRoot()
	if wantRoot != gotRoot {
		t.Fatal("payload changed across the wire round trip")
	}
	if len(back.Transactions) != 2 || len(back.Transactions[1]) != 0 {
		t.Fatalf("transactions not preserved: %v", back.Transactions)
	}

	// The conversion must not alias the consensus payload's slices.
	wire.ExtraData[0] = 'x'
	if original.ExtraData[0] == 'x' {
		t.Fatal("wire payload shares extra data with the consensus payload")
	}
}
