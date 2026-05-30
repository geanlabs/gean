package rlp

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestEncodeExamples(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want string
	}{
		{name: "empty bytes", got: EncodeBytes(nil), want: "80"},
		{name: "single byte", got: EncodeBytes([]byte{0x7f}), want: "7f"},
		{name: "dog", got: EncodeBytes([]byte("dog")), want: "83646f67"},
		{name: "small uint", got: EncodeUint64(15), want: "0f"},
		{name: "larger uint", got: EncodeUint64(1024), want: "820400"},
		{name: "list", got: EncodeList(EncodeBytes([]byte("cat")), EncodeBytes([]byte("dog"))), want: "c88363617483646f67"},
	}

	for _, tt := range tests {
		if hex.EncodeToString(tt.got) != tt.want {
			t.Fatalf("%s: got %x, want %s", tt.name, tt.got, tt.want)
		}
	}
}

func TestSplitListDecode(t *testing.T) {
	encoded := EncodeList(EncodeBytes([]byte("udp")), EncodeUint16(30303))
	items, err := SplitList(encoded)
	if err != nil {
		t.Fatalf("SplitList: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	key, err := String(items[0])
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	port, err := Uint16(items[1])
	if err != nil {
		t.Fatalf("Uint16: %v", err)
	}
	if key != "udp" || port != 30303 {
		t.Fatalf("got (%q, %d), want (udp, 30303)", key, port)
	}
}

func TestEncodeLongString(t *testing.T) {
	payload := bytes.Repeat([]byte{'a'}, 56)
	got := EncodeBytes(payload)
	if len(got) != 58 || got[0] != 0xb8 || got[1] != 56 {
		t.Fatalf("got long string prefix %x, want b838", got[:2])
	}
	if !bytes.Equal(got[2:], payload) {
		t.Fatalf("long string payload mismatch")
	}
}

func TestRejectNonCanonicalForms(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "non-minimal string", fn: func() error { _, err := Bytes([]byte{0x81, 0x7f}); return err }},
		{name: "non-minimal integer", fn: func() error { _, err := Uint64([]byte{0x82, 0x00, 0x01}); return err }},
		{name: "non-minimal long string", fn: func() error { _, err := Bytes([]byte{0xb8, 0x01, 0x00}); return err }},
		{name: "non-minimal length", fn: func() error { _, err := Bytes([]byte{0xb9, 0x00, 0x38}); return err }},
	}

	for _, tt := range tests {
		if err := tt.fn(); err == nil {
			t.Fatalf("%s: expected error", tt.name)
		}
	}
}
