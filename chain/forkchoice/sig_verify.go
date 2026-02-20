//go:build !skip_sig_verify

package forkchoice

func (c *Store) shouldVerifySignatures() bool { return true }
