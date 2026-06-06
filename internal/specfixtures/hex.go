package specfixtures

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/geanlabs/gean/internal/types"
)

func ParseHexRoot(s string) ([32]byte, error) {
	var root [32]byte
	hexData := trimHexPrefix(s)
	if hexData == "" {
		return root, fmt.Errorf("empty hex root")
	}
	b, err := hex.DecodeString(hexData)
	if err != nil {
		return root, fmt.Errorf("invalid hex root %q: %w", s, err)
	}
	if len(b) > len(root) {
		return root, fmt.Errorf("root too long: got %d bytes, want %d", len(b), len(root))
	}
	copy(root[:], b)
	return root, nil
}

func ParseHexBytes(s string) ([]byte, error) {
	b, err := hex.DecodeString(trimHexPrefix(s))
	if err != nil {
		return nil, fmt.Errorf("invalid hex %q: %w", s, err)
	}
	return b, nil
}

func ParseHexPubkey(s string) ([types.PubkeySize]byte, error) {
	var pk [types.PubkeySize]byte
	if trimHexPrefix(s) == "" {
		return pk, fmt.Errorf("empty pubkey")
	}
	b, err := ParseHexBytes(s)
	if err != nil {
		return pk, err
	}
	if len(b) > types.PubkeySize {
		return pk, fmt.Errorf("pubkey too long: got %d bytes, want %d", len(b), types.PubkeySize)
	}
	copy(pk[:], b)
	return pk, nil
}

func ParseHexSignature(s string) ([types.SignatureSize]byte, error) {
	var sig [types.SignatureSize]byte
	if trimHexPrefix(s) == "" {
		return sig, fmt.Errorf("empty signature")
	}
	b, err := ParseHexBytes(s)
	if err != nil {
		return sig, err
	}
	if len(b) > types.SignatureSize {
		return sig, fmt.Errorf("signature too long: got %d bytes, want %d", len(b), types.SignatureSize)
	}
	copy(sig[:], b)
	return sig, nil
}

func trimHexPrefix(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}
