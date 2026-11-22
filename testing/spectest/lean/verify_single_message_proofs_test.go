//go:build spectests

package lean

import (
	"testing"

	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
)

type singleMessageProofFixture struct {
	specfixtures.Header
	AttestationData specfixtures.TestAttData  `json:"attestationData"`
	PublicKeys      []string                  `json:"publicKeys"`
	AggregationBits specfixtures.TestDataList `json:"aggregationBits"`
	Message         string                    `json:"message"`
	Slot            uint64                    `json:"slot"`
	Proof           specfixtures.FCProofData  `json:"proof"`
	RejectionReason string                    `json:"rejectionReason"`
}

func runVerifySingleMessageProof(t *testing.T, fixture *singleMessageProofFixture) {
	message := mustParse(t, specfixtures.ParseHexRoot, fixture.Message)
	data, err := fixture.AttestationData.ToAttestationData()
	if err != nil {
		t.Fatalf("attestation data: %v", err)
	}
	if root, err := data.HashTreeRoot(); err != nil || root != message {
		t.Fatalf("attestation data root: got 0x%x (%v), want the signed message 0x%x", root, err, message)
	}

	keys := make([]xmss.CPubKey, 0, len(fixture.PublicKeys))
	defer func() {
		for _, key := range keys {
			xmss.FreePublicKey(key)
		}
	}()
	for _, hexKey := range fixture.PublicKeys {
		key, err := xmss.ParsePublicKey(mustParse(t, specfixtures.ParseHexPubkey, hexKey))
		if err != nil {
			t.Fatalf("public key: %v", err)
		}
		keys = append(keys, key)
	}

	err = xmss.VerifyAggregatedSignature(mustParse(t, specfixtures.ParseHexBytes, fixture.Proof.Data), keys, message, fixture.Slot)
	checkRejection(t, err, fixture.RejectionReason)
}

// checkRejection checks err against a fixture's expected rejection: an error
// when it names one, none when it is empty.
func checkRejection(t *testing.T, err error, rejectionReason string) {
	t.Helper()
	switch {
	case rejectionReason != "" && err == nil:
		t.Fatalf("accepted; want rejection %s", rejectionReason)
	case rejectionReason == "" && err != nil:
		t.Fatalf("rejected: %v", err)
	}
}
