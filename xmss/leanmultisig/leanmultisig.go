// Package leanmultisig provides Go bindings for devnet-2 recursive XMSS
// signature aggregation via the leanmultisig Rust FFI library.
package leanmultisig

/*
#cgo CFLAGS: -I${SRCDIR}/../leanmultisig-ffi/include
#cgo LDFLAGS: ${SRCDIR}/../leanmultisig-ffi/target/release/deps/libleanmultisig_ffi.a -lm -ldl -lpthread
#include "leanmultisig_ffi.h"
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

// MessageHashLength is the fixed hash size accepted by leanMultisig APIs.
const MessageHashLength = 32

// Result codes matching the LeanmultisigResult C enum.
const (
	ResultOK                    = C.LEANMULTISIG_RESULT_OK
	ResultNullPointer           = C.LEANMULTISIG_RESULT_NULL_POINTER
	ResultInvalidLength         = C.LEANMULTISIG_RESULT_INVALID_LENGTH
	ResultLengthMismatch        = C.LEANMULTISIG_RESULT_LENGTH_MISMATCH
	ResultDeserializationFailed = C.LEANMULTISIG_RESULT_DESERIALIZATION_FAILED
	ResultAggregationFailed     = C.LEANMULTISIG_RESULT_AGGREGATION_FAILED
	ResultVerificationFailed    = C.LEANMULTISIG_RESULT_VERIFICATION_FAILED
)

var setupOnce sync.Once

func setup() {
	// The Rust FFI uses two separate Once guards that both call the same
	// xmss_setup_aggregation_program() function. If both SetupProver and
	// SetupVerifier are called, it can invoke the setup twice and crash.
	// Guard it once on the Go side as well.
	C.leanmultisig_setup_prover()
}

// SetupProver initializes prover-side aggregation artifacts. It is idempotent.
func SetupProver() {
	setupOnce.Do(setup)
}

// SetupVerifier initializes verifier-side aggregation artifacts. It is idempotent.
func SetupVerifier() {
	setupOnce.Do(setup)
}

// Aggregate aggregates individual XMSS signatures into a devnet-2 proof blob.
func Aggregate(pubkeys, signatures [][]byte, messageHash [MessageHashLength]byte, epoch uint32) ([]byte, error) {
	if len(pubkeys) == 0 || len(signatures) == 0 {
		return nil, fmt.Errorf("pubkeys and signatures must be non-empty")
	}
	if len(pubkeys) != len(signatures) {
		return nil, fmt.Errorf("pubkeys/signatures length mismatch: %d/%d", len(pubkeys), len(signatures))
	}

	cPubkeys, numPubkeys, freePubkeys, err := makeCBytesViews(pubkeys)
	if err != nil {
		return nil, err
	}
	defer freePubkeys()

	cSignatures, numSignatures, freeSignatures, err := makeCBytesViews(signatures)
	if err != nil {
		return nil, err
	}
	defer freeSignatures()

	var outData *C.uint8_t
	var outLen C.size_t
	result := C.leanmultisig_aggregate(
		cPubkeys,
		numPubkeys,
		cSignatures,
		numSignatures,
		(*C.uint8_t)(unsafe.Pointer(&messageHash[0])),
		C.size_t(MessageHashLength),
		C.uint32_t(epoch),
		&outData,
		&outLen,
	)
	if result != ResultOK {
		return nil, resultError("leanmultisig_aggregate", result)
	}
	if outData == nil || outLen == 0 {
		return nil, fmt.Errorf("leanmultisig_aggregate returned empty proof")
	}

	proof := C.GoBytes(unsafe.Pointer(outData), C.int(outLen))
	C.leanmultisig_bytes_free(outData, outLen)
	return proof, nil
}

// VerifyAggregated verifies a devnet-2 aggregated proof against public keys.
func VerifyAggregated(pubkeys [][]byte, messageHash [MessageHashLength]byte, proofData []byte, epoch uint32) error {
	if len(pubkeys) == 0 {
		return fmt.Errorf("pubkeys must be non-empty")
	}
	if len(proofData) == 0 {
		return fmt.Errorf("proof data must be non-empty")
	}

	cPubkeys, numPubkeys, freePubkeys, err := makeCBytesViews(pubkeys)
	if err != nil {
		return err
	}
	defer freePubkeys()

	result := C.leanmultisig_verify_aggregated(
		cPubkeys,
		numPubkeys,
		(*C.uint8_t)(unsafe.Pointer(&messageHash[0])),
		C.size_t(MessageHashLength),
		(*C.uint8_t)(unsafe.Pointer(&proofData[0])),
		C.size_t(len(proofData)),
		C.uint32_t(epoch),
	)
	if result != ResultOK {
		return resultError("leanmultisig_verify_aggregated", result)
	}
	return nil
}

func makeCBytesViews(chunks [][]byte) (*C.LeanmultisigBytes, C.size_t, func(), error) {
	if len(chunks) == 0 {
		return nil, 0, nil, fmt.Errorf("empty byte chunks")
	}

	viewSize := unsafe.Sizeof(C.LeanmultisigBytes{})
	viewsPtr := C.malloc(C.size_t(len(chunks)) * C.size_t(viewSize))
	if viewsPtr == nil {
		return nil, 0, nil, fmt.Errorf("allocate C LeanmultisigBytes array")
	}
	views := unsafe.Slice((*C.LeanmultisigBytes)(viewsPtr), len(chunks))

	allocated := make([]unsafe.Pointer, len(chunks))
	for i := range chunks {
		if len(chunks[i]) == 0 {
			for j := 0; j < i; j++ {
				C.free(allocated[j])
			}
			C.free(viewsPtr)
			return nil, 0, nil, fmt.Errorf("empty byte chunk at index %d", i)
		}
		data := C.CBytes(chunks[i])
		allocated[i] = data
		views[i] = C.LeanmultisigBytes{
			data: (*C.uint8_t)(data),
			len:  C.size_t(len(chunks[i])),
		}
	}

	freeFn := func() {
		for _, data := range allocated {
			if data != nil {
				C.free(data)
			}
		}
		C.free(viewsPtr)
	}

	return (*C.LeanmultisigBytes)(viewsPtr), C.size_t(len(chunks)), freeFn, nil
}

func resultError(op string, result C.enum_LeanmultisigResult) error {
	switch result {
	case ResultNullPointer:
		return fmt.Errorf("%s failed: null pointer", op)
	case ResultInvalidLength:
		return fmt.Errorf("%s failed: invalid length", op)
	case ResultLengthMismatch:
		return fmt.Errorf("%s failed: length mismatch", op)
	case ResultDeserializationFailed:
		return fmt.Errorf("%s failed: deserialization failed", op)
	case ResultAggregationFailed:
		return fmt.Errorf("%s failed: aggregation failed", op)
	case ResultVerificationFailed:
		return fmt.Errorf("%s failed: verification failed", op)
	default:
		return fmt.Errorf("%s failed with code %d", op, result)
	}
}
