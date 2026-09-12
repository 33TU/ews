package proto

import "errors"

// Error strings carry the ws prefix because that is the package callers see.
var (
	ErrClosing         = errors.New("ews/ws: close frame already sent")
	ErrProtocol        = errors.New("ews/ws: invalid frame or message sequence")
	ErrInvalidUTF8     = errors.New("ews/ws: invalid UTF-8")
	ErrMessageTooLarge = errors.New("ews/ws: message exceeds limit")
	ErrInvalidData     = errors.New("ews/ws: invalid compressed message data")
)

// Error is a terminal receive failure and the close code that describes it.
type Error struct {
	Code uint16
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Role identifies the local endpoint.
type Role uint8

const (
	Server Role = iota
	Client
)
