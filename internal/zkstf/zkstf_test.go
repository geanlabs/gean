package zkstf_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/zkvectors"
)

func TestInputRoundTripAndRejects(t *testing.T) {
	in, err := zkstf.EncodeInput([]byte("state"), []byte("block"))
	if err != nil {
		t.Fatal(err)
	}
	s, b, err := zkstf.DecodeInput(in)
	if err != nil || string(s) != "state" || string(b) != "block" {
		t.Fatalf("round trip: %q %q %v", s, b, err)
	}

	bad := map[string][]byte{
		"empty":     nil,
		"magic":     append([]byte("XSTF"), in[4:]...),
		"version":   append(append([]byte("GSTF"), 2, 0, 0, 0), in[8:]...),
		"trailing":  append(append([]byte{}, in...), 0),
		"truncated": in[:len(in)-1],
		"oversize":  append(append([]byte("GSTF"), 1, 0, 0, 0), 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f),
	}
	for name, input := range bad {
		if _, _, err := zkstf.DecodeInput(input); !errors.Is(err, zkstf.ErrMalformedInput) {
			t.Errorf("%s: err = %v, want ErrMalformedInput", name, err)
		}
	}
}

func TestParsePublicValues(t *testing.T) {
	pv := zkstf.PublicValues{PreStateRoot: [32]byte{1}, BlockRoot: [32]byte{2}, PostStateRoot: [32]byte{3}}
	b := pv.Bytes()
	if got, err := zkstf.ParsePublicValues(b[:]); err != nil || got != pv {
		t.Fatalf("exact: %v %v", got, err)
	}
	padded := append(b[:], make([]byte, 160)...)
	if got, err := zkstf.ParsePublicValues(padded); err != nil || got != pv {
		t.Fatalf("zero-padded: %v %v", got, err)
	}
	padded[200] = 1
	if _, err := zkstf.ParsePublicValues(padded); !errors.Is(err, zkstf.ErrMalformedPublicValues) {
		t.Fatalf("dirty padding accepted: %v", err)
	}
	for _, n := range []int{0, 95, 257} {
		if _, err := zkstf.ParsePublicValues(make([]byte, n)); err == nil {
			t.Errorf("%d bytes accepted", n)
		}
	}
}

func TestProofFileRoundTrip(t *testing.T) {
	p := &zkstf.Proof{
		ZKVM:         zkstf.ZisK,
		VKHash:       [32]byte{9},
		PublicValues: zkstf.PublicValues{PostStateRoot: [32]byte{7}},
		Data:         []byte("proof bytes"),
	}
	enc, err := p.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var got zkstf.Proof
	if err := got.UnmarshalBinary(enc); err != nil {
		t.Fatal(err)
	}
	if got.ZKVM != p.ZKVM || got.VKHash != p.VKHash ||
		got.PublicValues != p.PublicValues || !bytes.Equal(got.Data, p.Data) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if err := got.UnmarshalBinary(enc[:len(enc)-1]); err == nil {
		t.Fatal("truncated proof accepted")
	}
	renamed := bytes.Replace(enc, []byte("zisk"), []byte("zork"), 1)
	if err := got.UnmarshalBinary(renamed); err == nil || !strings.Contains(err.Error(), "unknown zkvm") {
		t.Fatalf("unknown zkvm accepted: %v", err)
	}
}

// TestApplyMatchesNativeTransition is the reference every zkVM backend must
// reproduce: Apply agrees with the Go transition on every vector, fails
// exactly on the reject cases, and consecutive blocks chain by root.
func TestApplyMatchesNativeTransition(t *testing.T) {
	cases, err := zkvectors.Suite()
	if err != nil {
		t.Fatal(err)
	}
	var prev *zkstf.PublicValues
	prevChain := ""
	for _, c := range cases {
		input, err := zkstf.NewInput(c.Pre, c.Block)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		pv, err := zkstf.Apply(input)
		if c.WantErr {
			if err == nil {
				t.Errorf("%s: Apply succeeded, want rejection", c.Name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}

		preRoot, _ := c.Pre.HashTreeRoot()
		blockRoot, _ := c.Block.HashTreeRoot()
		post, _ := c.Pre.Clone()
		if err := statetransition.StateTransition(post, c.Block); err != nil {
			t.Fatalf("%s: native: %v", c.Name, err)
		}
		postRoot, _ := post.HashTreeRoot()
		want := zkstf.PublicValues{PreStateRoot: preRoot, BlockRoot: blockRoot, PostStateRoot: postRoot}
		if pv != want {
			t.Fatalf("%s: Apply %v, native %v", c.Name, pv, want)
		}

		chain := c.Name[:strings.IndexByte(c.Name, '/')]
		if prev != nil && chain == prevChain && pv.PreStateRoot != prev.PostStateRoot {
			t.Fatalf("%s: pre root does not chain from the previous block's post root", c.Name)
		}
		prev, prevChain = &pv, chain
	}
}

func TestApplyRejectsMalformedSSZ(t *testing.T) {
	in, _ := zkstf.EncodeInput([]byte{1, 2, 3}, []byte{4})
	if _, err := zkstf.Apply(in); !errors.Is(err, zkstf.ErrMalformedInput) {
		t.Fatalf("err = %v, want ErrMalformedInput", err)
	}
}

func TestNativeProver(t *testing.T) {
	cases, err := zkvectors.Chain(zkvectors.ChainConfig{Name: "n", Validators: 4, Slots: 2})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := zkstf.NewInput(cases[1].Pre, cases[1].Block)
	var p zkstf.Prover = zkstf.NativeProver{}
	res, err := p.Execute(context.Background(), input)
	if err != nil || res.PublicValues.PostStateRoot != cases[1].Block.StateRoot {
		t.Fatalf("execute: %+v %v", res, err)
	}
	if _, err := p.Prove(context.Background(), input); !errors.Is(err, zkstf.ErrUnsupported) {
		t.Fatalf("prove: %v", err)
	}
	cases[1].Block.StateRoot[0] ^= 1
	bad, _ := zkstf.NewInput(cases[1].Pre, cases[1].Block)
	if _, err := p.Execute(context.Background(), bad); !errors.Is(err, zkstf.ErrGuestFailed) {
		t.Fatalf("bad block: %v", err)
	}
}
