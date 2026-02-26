package leanmultisig_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/geanlabs/gean/xmss/leanmultisig"
	"github.com/geanlabs/gean/xmss/leansig"
)

const testActivationEpoch = 0
const testNumActiveEpochs = 8

type multisigFixture struct {
	pubkeys [][]byte
	sigs    [][]byte
	message [leanmultisig.MessageHashLength]byte
	epoch   uint32
}

var sharedFixture multisigFixture

func TestMain(m *testing.M) {
	var err error
	sharedFixture, err = createMultisigFixture()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: create multisig fixture: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func createMultisigFixture() (multisigFixture, error) {
	var out multisigFixture

	kp1, err := leansig.GenerateKeypair(101, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		return out, fmt.Errorf("generate keypair 1: %w", err)
	}
	defer kp1.Free()

	kp2, err := leansig.GenerateKeypair(202, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		return out, fmt.Errorf("generate keypair 2: %w", err)
	}
	defer kp2.Free()

	var message [leanmultisig.MessageHashLength]byte
	for i := range message {
		message[i] = byte(i + 1)
	}
	epoch := uint32(0)

	pk1, err := kp1.PublicKeyBytes()
	if err != nil {
		return out, fmt.Errorf("serialize pubkey 1: %w", err)
	}
	pk2, err := kp2.PublicKeyBytes()
	if err != nil {
		return out, fmt.Errorf("serialize pubkey 2: %w", err)
	}

	sig1, err := kp1.Sign(epoch, message)
	if err != nil {
		return out, fmt.Errorf("sign with keypair 1: %w", err)
	}
	sig2, err := kp2.Sign(epoch, message)
	if err != nil {
		return out, fmt.Errorf("sign with keypair 2: %w", err)
	}

	out = multisigFixture{
		pubkeys: [][]byte{pk1, pk2},
		sigs:    [][]byte{sig1, sig2},
		message: message,
		epoch:   epoch,
	}
	return out, nil
}

func TestAggregateAndVerifyRoundTrip(t *testing.T) {
	fx := sharedFixture

	leanmultisig.SetupProver()
	proof, err := leanmultisig.Aggregate(fx.pubkeys, fx.sigs, fx.message, fx.epoch)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(proof) == 0 {
		t.Fatal("aggregate returned empty proof")
	}

	leanmultisig.SetupVerifier()
	if err := leanmultisig.VerifyAggregated(fx.pubkeys, fx.message, proof, fx.epoch); err != nil {
		t.Fatalf("verify aggregated: %v", err)
	}
}

func TestVerifyRejectsWrongMessage(t *testing.T) {
	fx := sharedFixture

	leanmultisig.SetupProver()
	proof, err := leanmultisig.Aggregate(fx.pubkeys, fx.sigs, fx.message, fx.epoch)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	wrongMessage := fx.message
	wrongMessage[0] ^= 0xFF

	leanmultisig.SetupVerifier()
	if err := leanmultisig.VerifyAggregated(fx.pubkeys, wrongMessage, proof, fx.epoch); err == nil {
		t.Fatal("expected verification failure with wrong message")
	}
}

func TestAggregateRejectsLengthMismatch(t *testing.T) {
	fx := sharedFixture

	_, err := leanmultisig.Aggregate(fx.pubkeys[:1], fx.sigs, fx.message, fx.epoch)
	if err == nil {
		t.Fatal("expected aggregate to fail on pubkey/signature length mismatch")
	}
}
