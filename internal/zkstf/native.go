package zkstf

import (
	"context"
	"fmt"
	"time"
)

// NativeProver runs Apply on the host. It cannot prove; it is the reference
// every zkVM backend is compared against.
type NativeProver struct{}

func (NativeProver) ZKVM() ZKVM { return Native }

func (NativeProver) Execute(ctx context.Context, input []byte) (*ExecResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	pv, err := Apply(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGuestFailed, err)
	}
	return &ExecResult{PublicValues: pv, Duration: time.Since(start)}, nil
}

func (NativeProver) Prove(context.Context, []byte) (*Proof, error) {
	return nil, fmt.Errorf("%w: the native backend executes but does not prove", ErrUnsupported)
}

func (NativeProver) Verify(context.Context, *Proof) (PublicValues, error) {
	return PublicValues{}, fmt.Errorf("%w: the native backend executes but does not verify", ErrUnsupported)
}
