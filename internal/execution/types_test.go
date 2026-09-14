package execution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestQuantityEncoding(t *testing.T) {
	for _, tt := range []struct {
		value uint64
		wire  string
	}{
		{0, `"0x0"`},
		{1, `"0x1"`},
		{255, `"0xff"`},
		{1_700_000_004, `"0x6553f104"`},
	} {
		got, err := json.Marshal(Quantity(tt.value))
		if err != nil || string(got) != tt.wire {
			t.Fatalf("marshal %d: got %s (%v), want %s", tt.value, got, err, tt.wire)
		}
		var back Quantity
		if err := json.Unmarshal([]byte(tt.wire), &back); err != nil || uint64(back) != tt.value {
			t.Fatalf("unmarshal %s: got %d (%v)", tt.wire, back, err)
		}
	}
	var q Quantity
	if err := json.Unmarshal([]byte(`"0x10000000000000000"`), &q); err == nil {
		t.Fatal("2^64 must not fit a quantity")
	}
}

func TestU256Encoding(t *testing.T) {
	// SSZ little-endian 7 == wire "0x7".
	var u U256
	u[0] = 7
	got, _ := json.Marshal(u)
	if string(got) != `"0x7"` {
		t.Fatalf("got %s", got)
	}
	var back U256
	if err := json.Unmarshal([]byte(`"0x0100000000000000000000000000000000000000000000000000000000000000"`), &back); err != nil {
		t.Fatal(err)
	}
	if back[31] != 1 || back[0] != 0 {
		t.Fatalf("big-endian wire value must land in the high little-endian byte: %x", back)
	}
}

func TestFixedFieldsRejectWrongWidth(t *testing.T) {
	var h Hash
	if err := json.Unmarshal([]byte(`"0x1234"`), &h); err == nil {
		t.Fatal("short hash accepted")
	}
	var id PayloadID
	if err := json.Unmarshal([]byte(`"0x0123456789abcdef"`), &id); err != nil {
		t.Fatalf("payload id: %v", err)
	}
	if id.String() != "0x0123456789abcdef" {
		t.Fatalf("payload id string: %s", id.String())
	}
}

func TestPayloadStatusAndForkchoiceResultShapes(t *testing.T) {
	var result ForkchoiceUpdatedResult
	raw := `{"payloadStatus":{"status":"SYNCING","latestValidHash":null,"validationError":null},"payloadId":null}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.PayloadStatus.Status != StatusSyncing || result.PayloadID != nil {
		t.Fatalf("unexpected decode: %+v", result)
	}
	raw = `{"payloadStatus":{"status":"VALID","latestValidHash":"0x` + strings.Repeat("ab", 32) + `","validationError":null},"payloadId":"0x0000000000000001"}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.PayloadID == nil || result.PayloadID[7] != 1 || result.PayloadStatus.LatestValidHash == nil {
		t.Fatalf("unexpected decode: %+v", result)
	}
	var status PayloadStatus
	if err := json.Unmarshal([]byte(`{"status":"INVALID_BLOCK_HASH","latestValidHash":null,"validationError":"bad hash"}`), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusInvalidBlockHash || status.ValidationError == nil || *status.ValidationError != "bad hash" {
		t.Fatalf("unexpected decode: %+v", status)
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
