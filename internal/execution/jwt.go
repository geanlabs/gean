package execution

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// JWTSecret is the 32-byte HMAC key shared out of band with the execution
// client. Every request to the authenticated engine port carries a fresh
// HS256 token over an issued-at claim; the execution client accepts a token
// whose issued-at is within a minute of its own clock, so tokens are minted
// per request rather than cached.
type JWTSecret [32]byte

// ParseJWTSecret decodes a hex secret, with or without a 0x prefix.
func ParseJWTSecret(text string) (JWTSecret, error) {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(strings.TrimPrefix(trimmed, "0x"), "0X")
	raw, err := hex.DecodeString(trimmed)
	if err != nil {
		return JWTSecret{}, fmt.Errorf("jwt secret is not hex: %w", err)
	}
	if len(raw) != len(JWTSecret{}) {
		return JWTSecret{}, fmt.Errorf("jwt secret is %d bytes, want %d", len(raw), len(JWTSecret{}))
	}
	var secret JWTSecret
	copy(secret[:], raw)
	return secret, nil
}

// LoadJWTSecret reads a secret file in the single-line hex format every
// execution client writes and reads.
func LoadJWTSecret(path string) (JWTSecret, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return JWTSecret{}, fmt.Errorf("read jwt secret: %w", err)
	}
	secret, err := ParseJWTSecret(string(data))
	if err != nil {
		return JWTSecret{}, fmt.Errorf("%s: %w", path, err)
	}
	return secret, nil
}

// jwtHeader is the fixed HS256 header, pre-encoded.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// Token mints a bearer token whose only claim is the issued-at second.
func (s JWTSecret) Token(issuedAt time.Time) string {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"iat":` + strconv.FormatInt(issuedAt.Unix(), 10) + `}`))
	signingInput := jwtHeader + "." + claims
	mac := hmac.New(sha256.New, s[:])
	mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
