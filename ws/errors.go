package ws

import (
	"errors"
	"strconv"

	"github.com/33TU/ews/internal/proto"
)

var (
	// ErrClosing means a close frame has already been sent.
	ErrClosing = proto.ErrClosing
	// ErrProtocol means the peer violated the framing or message rules.
	ErrProtocol = proto.ErrProtocol
	// ErrInvalidUTF8 means a text message or close reason is not valid UTF-8.
	ErrInvalidUTF8 = proto.ErrInvalidUTF8
	// ErrMessageTooLarge means a message exceeds Config.MaxMessageSize.
	ErrMessageTooLarge = proto.ErrMessageTooLarge
	// ErrInvalidData means a compressed message could not be decompressed.
	ErrInvalidData = proto.ErrInvalidData
	// ErrInvalidConfig means NewConn received an invalid Config.
	ErrInvalidConfig = errors.New("ews/ws: invalid configuration")
	// ErrMessageOpen means a fragmented message is in progress, so Write and
	// BeginMessage must wait for EndMessage.
	ErrMessageOpen = errors.New("ews/ws: fragmented message in progress")
	// ErrNoMessage means WriteChunk or EndMessage was called without BeginMessage.
	ErrNoMessage = errors.New("ews/ws: no fragmented message in progress")
	// ErrQueueFull means a Queue has reached its byte limit.
	ErrQueueFull = errors.New("ews/ws: send queue full")
)

// Error is a terminal protocol failure. A close frame carrying Code has been
// sent when possible. Every later read returns the same error.
type Error struct {
	Code uint16
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// CloseError is returned by reads after the peer's close frame has been
// handled. Code is NoStatus when the frame carried no status code.
type CloseError struct {
	Code   uint16
	Reason string
}

func (e *CloseError) Error() string {
	s := "ews/ws: peer closed with code " + strconv.Itoa(int(e.Code))
	if e.Reason != "" {
		s += ": " + e.Reason
	}
	return s
}
