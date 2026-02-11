// errors.go contains sentinel errors exposed by the reqresp package.
package reqresp

import "errors"

// Request/response protocol errors. Callers may use errors.Is to check for them.
var (
	ErrInvalidStatus = errors.New("invalid peer status")
)
