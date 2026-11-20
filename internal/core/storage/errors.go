package storage

import "errors"

var errBatchClosed = errors.New("storage write batch is closed")
