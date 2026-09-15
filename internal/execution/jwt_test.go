package execution

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testSecretHex = "0102030405060708091011121314151617181920212223242526272829303132"

// Reference token computed outside Go (HS256 over {"iat":1700000000}).
const testReferenceToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpYXQiOjE3MDAwMDAwMDB9.UHpQFtHy4sYhdeA9x22Fu5W1S4_Wt2TUQ5LVNOWvcAw"

func TestJWTTokenMatchesReference(t *testing.T) {
	secret, err := ParseJWTSecret("0x" + testSecretHex)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := secret.Token(time.Unix(1_700_000_000, 0)); got != testReferenceToken {
		t.Fatalf("token mismatch:\n got  %s\n want %s", got, testReferenceToken)
	}
	if secret.Token(time.Unix(1_700_000_001, 0)) == testReferenceToken {
		t.Fatal("a different issued-at must change the token")
	}
}

func TestJWTSecretParsing(t *testing.T) {
	bare, err := ParseJWTSecret(testSecretHex)
	if err != nil {
		t.Fatalf("bare hex: %v", err)
	}
	prefixed, err := ParseJWTSecret("  0x" + testSecretHex + "\n")
	if err != nil {
		t.Fatalf("prefixed hex: %v", err)
	}
	if bare != prefixed {
		t.Fatal("prefix and whitespace must not change the secret")
	}
	for _, bad := range []string{"", "0x1234", "zz", testSecretHex + "00"} {
		if _, err := ParseJWTSecret(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestLoadJWTSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jwt.hex")
	if err := os.WriteFile(path, []byte(testSecretHex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := LoadJWTSecret(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want, _ := ParseJWTSecret(testSecretHex)
	if secret != want {
		t.Fatal("loaded secret differs from parsed secret")
	}
	if _, err := LoadJWTSecret(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file should error")
	}
}
