package zkhost_test

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/zkhost"
)

// fakeHost writes a host that prints out and exits with code for every command.
func fakeHost(t *testing.T, out string, code int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "host")
	script := "#!/bin/sh\necho '" + out + "'\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerify(t *testing.T) {
	pv := zkstf.PublicValues{PreStateRoot: [32]byte{1}, BlockRoot: [32]byte{2}, PostStateRoot: [32]byte{3}}
	pvBytes := pv.Bytes()
	vk := [32]byte{9}
	line := "vk=" + hex.EncodeToString(vk[:]) + " pv=" + hex.EncodeToString(pvBytes[:])
	proof := zkstf.Proof{ZKVM: zkstf.SP1, VKHash: vk, PublicValues: pv}

	otherVM := proof
	otherVM.ZKVM = zkstf.ZisK
	otherVK := proof
	otherVK.VKHash = [32]byte{8}
	otherPV := proof
	otherPV.PublicValues.PostStateRoot = [32]byte{4}

	cases := []struct {
		name    string
		out     string
		code    int
		proof   zkstf.Proof
		wantErr error
	}{
		{"valid", line, 0, proof, nil},
		{"host rejects", "", 4, proof, zkstf.ErrInvalidProof},
		{"other zkvm", line, 0, otherVM, zkstf.ErrInvalidProof},
		{"other program", line, 0, otherVK, zkstf.ErrInvalidProof},
		{"other public values", line, 0, otherPV, zkstf.ErrInvalidProof},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := zkhost.Prover{VM: zkstf.SP1, Bin: fakeHost(t, c.out, c.code), ELF: "guest.elf"}
			got, err := p.Verify(context.Background(), &c.proof)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if err == nil && got != pv {
				t.Fatalf("public values = %s, want %s", got, pv)
			}
		})
	}
}

func TestExecuteGuestFailure(t *testing.T) {
	p := zkhost.Prover{VM: zkstf.OpenVM, Bin: fakeHost(t, "", 3), ELF: "guest.elf"}
	_, err := p.Execute(context.Background(), []byte("input"))
	if !errors.Is(err, zkstf.ErrGuestFailed) {
		t.Fatalf("err = %v, want ErrGuestFailed", err)
	}
}

func TestNoHost(t *testing.T) {
	p := zkhost.Prover{VM: zkstf.ZisK, ELF: "guest.elf"}
	_, err := p.Prove(context.Background(), []byte("input"))
	if !errors.Is(err, zkstf.ErrUnsupported) || !strings.Contains(err.Error(), "make zk-host ZKVM=zisk") {
		t.Fatalf("err = %v, want ErrUnsupported naming the build target", err)
	}
}
