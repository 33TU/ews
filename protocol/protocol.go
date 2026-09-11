package protocol

import (
	"errors"

	"github.com/33TU/ews/codec"
)

// Role identifies the local endpoint.
type Role uint8

const (
	Server Role = iota
	Client
)

const DefaultMaxMessageSize = 8 << 20

var (
	ErrClosing         = errors.New("ews/protocol: closing handshake started")
	ErrProtocol        = errors.New("ews/protocol: invalid frame or message sequence")
	ErrInvalidUTF8     = errors.New("ews/protocol: invalid UTF-8")
	ErrMessageTooLarge = errors.New("ews/protocol: message exceeds limit")
	ErrInvalidConfig   = errors.New("ews/protocol: invalid configuration")
)

// Error is a terminal receive failure and its suggested WebSocket close code.
type Error struct {
	Code uint16
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Event contains a complete message or a ping, pong, or close frame.
// Payload is borrowed until the next NextEvent or Reset call.
// Close payloads contain an optional two-byte code followed by a UTF-8 reason.
type Event struct {
	Opcode  codec.Opcode
	Payload []byte
}
