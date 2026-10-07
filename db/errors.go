package db

import "errors"

// ErrBatchClosed is returned by a WriteBatch used after Commit.
var ErrBatchClosed = errors.New("storage write batch is closed")
