package ws

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/33TU/ews/codec"
)

// sender validates and encodes outgoing frames. Use Init. Calls must be
// serialized. Output is borrowed until the next call.
type sender struct {
	role      Role
	enc       codec.Encoder
	key       [4]byte
	closeBuf  [125]byte
	closeSent bool
}

// Init prepares the sender for a connection, retaining encoder storage.
func (s *sender) Init(role Role) {
	s.role = role
	s.enc.Reset()
	s.closeSent = false
}

// CloseSent reports whether a close frame has been encoded.
func (s *sender) CloseSent() bool { return s.closeSent }

// Encode prepares one complete frame. After a close frame only pongs are
// accepted. Client frames are masked with a fresh key.
func (s *sender) Encode(op codec.Opcode, payload []byte) (header, body []byte, err error) {
	return s.encode(op, payload, false)
}

// EncodeCompressed prepares one complete text or binary frame carrying
// already-compressed payload, with RSV1 set.
func (s *sender) EncodeCompressed(op codec.Opcode, payload []byte) (header, body []byte, err error) {
	if op != codec.Text && op != codec.Binary {
		return nil, nil, ErrProtocol
	}
	return s.encode(op, payload, true)
}

// EncodeFragment prepares one frame of a fragmented data message: the first
// carries the message opcode, later ones Continuation, and only the last is
// final. compressed sets RSV1 on a first frame.
func (s *sender) EncodeFragment(op codec.Opcode, final bool, payload []byte, compressed bool) (header, body []byte, err error) {
	if op != codec.Text && op != codec.Binary && op != codec.Continuation {
		return nil, nil, ErrProtocol
	}
	return s.encodeFrame(op, final, payload, compressed)
}

func (s *sender) encode(op codec.Opcode, payload []byte, compressed bool) (header, body []byte, err error) {
	if s.closeSent && op != codec.Pong {
		return nil, nil, ErrClosing
	}
	switch op {
	case codec.Text, codec.Binary:
	case codec.Ping, codec.Pong:
		if len(payload) > 125 {
			return nil, nil, ErrProtocol
		}
	case codec.Close:
		if err := validateClose(payload); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, ErrProtocol
	}

	return s.encodeFrame(op, true, payload, compressed)
}

func (s *sender) encodeFrame(op codec.Opcode, final bool, payload []byte, compressed bool) (header, body []byte, err error) {
	if s.closeSent && op != codec.Pong {
		return nil, nil, ErrClosing
	}
	var key *[4]byte
	if s.role == Client {
		rand.Read(s.key[:])
		key = &s.key
	}
	if compressed {
		err = s.enc.EncodeCompressed(final, op, payload, key)
	} else {
		err = s.enc.Encode(final, op, payload, key)
	}
	if err != nil {
		return nil, nil, err
	}
	if op == codec.Close {
		s.closeSent = true
	}
	return s.enc.HeaderBytes(), s.enc.PayloadBytes(), nil
}

// EncodeClose prepares a close frame. Code zero sends an empty payload and
// requires an empty reason.
func (s *sender) EncodeClose(code uint16, reason string) (header, body []byte, err error) {
	if code == 0 {
		if reason != "" {
			return nil, nil, ErrProtocol
		}
		return s.Encode(codec.Close, nil)
	}
	if len(reason) > 123 {
		return nil, nil, ErrProtocol
	}
	binary.BigEndian.PutUint16(s.closeBuf[:2], code)
	copy(s.closeBuf[2:], reason)
	return s.Encode(codec.Close, s.closeBuf[:2+len(reason)])
}
