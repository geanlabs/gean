package reqresp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// Timeout is the idle deadline applied to each individual req/resp read and
// write. It bounds how long a single stalled operation may block, not the whole
// exchange — a transfer that keeps making progress is never cut.
const Timeout = 15 * time.Second

// Req/resp streams are bounded by an idle deadline, reset before every read and
// write: a peer making progress is never cut, one that stalls is aborted within
// Timeout. A single total deadline would instead sever a slow-but-valid
// large backfill mid-transfer. The context passed to NewStream covers only stream
// establishment, so without these the response read is unbounded and one stalled
// peer can wedge the fetch loop indefinitely.

func ArmReadDeadline(s network.Stream) {
	_ = s.SetReadDeadline(time.Now().Add(Timeout))
}

func ArmWriteDeadline(s network.Stream) {
	_ = s.SetWriteDeadline(time.Now().Add(Timeout))
}

// IsStreamTimeout reports whether err is a read/write deadline expiry, so a stalled
// peer is attributed distinctly from other transport failures.
func IsStreamTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// ReadChunk arms the idle read deadline, decodes one response frame of at most
// maxLen bytes, and on a deadline expiry records the stall and returns a clear error.
func ReadChunk(s network.Stream, r io.Reader, protocol string, maxLen uint32) (byte, []byte, error) {
	ArmReadDeadline(s)
	code, data, err := DecodeResponse(r, maxLen)
	if err != nil && IsStreamTimeout(err) {
		OnTimeout(protocol, "read")
		return code, data, fmt.Errorf("%s: response read stalled beyond %s: %w", protocol, Timeout, err)
	}
	return code, data, err
}
