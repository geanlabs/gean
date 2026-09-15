package types

import "errors"

var (
	ErrExecutionWithdrawalsUnsupported = errors.New("execution withdrawals are not supported")
	ErrExecutionBlobsUnsupported       = errors.New("execution blob transactions are not supported")
)

// ValidateExecutionFeatures enforces the features supported by this consensus
// protocol, independently of execution validity. An execution client cannot
// authorize withdrawals for us or verify availability of blobs we do not carry.
func (p *ExecutionPayload) ValidateExecutionFeatures() error {
	if p == nil {
		return errors.New("execution payload is nil")
	}
	if len(p.Withdrawals) != 0 {
		return ErrExecutionWithdrawalsUnsupported
	}
	if p.BlobGasUsed != 0 {
		return ErrExecutionBlobsUnsupported
	}
	for _, tx := range p.Transactions {
		// EIP-2718 places the transaction type before its encoded payload.
		if len(tx) > 0 && tx[0] == 0x03 {
			return ErrExecutionBlobsUnsupported
		}
	}
	return nil
}
