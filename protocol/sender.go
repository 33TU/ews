package protocol

import (
	"crypto/rand"
	"encoding/binary"
	"slices"
	"unicode/utf8"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
)

// Compression configures negotiated permessage-deflate with a 32 KB window.
type Compression struct {
	Level           int
	ContextTakeover bool
}

// SenderConfig describes the local endpoint and negotiated send direction.
type SenderConfig struct {
	Role        Role
	Compression *Compression
}

// Sender appends frames to caller-owned output. Use NewSender.
// Calls on the same sender must be serialized.
type Sender struct {
	role       Role
	encoder    codec.Encoder
	compressor *deflate.Compressor
	closeSent  bool
}

func NewSender(config SenderConfig) (*Sender, error) {
	if config.Role != Server && config.Role != Client {
		return nil, ErrInvalidConfig
	}
	s := &Sender{role: config.Role}
	if c := config.Compression; c != nil {
		var err error
		s.compressor, err = deflate.NewCompressor(c.Level)
		if err != nil {
			return nil, err
		}
		s.compressor.ContextTakeover = c.ContextTakeover
	}
	return s, nil
}

// Reset clears connection state, retaining storage and configuration.
func (s *Sender) Reset() {
	s.encoder.Reset()
	if s.compressor != nil {
		s.compressor.Reset()
	}
	s.closeSent = false
}

// CloseSent reports whether a close frame was appended, not whether it was written.
func (s *Sender) CloseSent() bool { return s.closeSent }

// Append appends one complete message or control frame, growing dst as needed.
// compress requires negotiated compression and a text or binary opcode.
// Errors return dst unchanged. The sender never retains dst or payload.
func (s *Sender) Append(dst []byte, opcode codec.Opcode, payload []byte, compress bool) ([]byte, error) {
	if s.closeSent && opcode != codec.Pong {
		return dst, ErrClosing
	}
	switch opcode {
	case codec.Text, codec.Binary:
		if opcode == codec.Text && !utf8.Valid(payload) {
			return dst, ErrInvalidUTF8
		}
		if compress && s.compressor == nil {
			return dst, ErrProtocol
		}
	case codec.Ping, codec.Pong, codec.Close:
		if compress || len(payload) > 125 {
			return dst, ErrProtocol
		}
		if opcode == codec.Close {
			if err := validateClose(payload); err != nil {
				return dst, err
			}
		}
	default:
		return dst, ErrProtocol
	}
	var maskKey [4]byte
	var key *[4]byte
	if s.role == Client {
		rand.Read(maskKey[:])
		key = &maskKey
	}
	if compress {
		var err error
		payload, err = s.compressor.Compress(payload)
		if err != nil {
			return dst, err
		}
		if err := s.encoder.EncodeCompressed(true, opcode, payload, key); err != nil {
			return dst, err
		}
	} else if err := s.encoder.Encode(true, opcode, payload, key); err != nil {
		return dst, err
	}
	header, body := s.encoder.HeaderBytes(), s.encoder.PayloadBytes()
	start := len(dst)
	dst = slices.Grow(dst, len(header)+len(body))[:start+len(header)+len(body)]
	// Copy payload first so it can overlap dst's unused capacity.
	copy(dst[start+len(header):], body)
	copy(dst[start:], header)
	s.encoder.Reset()
	if opcode == codec.Close {
		s.closeSent = true
	}
	return dst, nil
}

// AppendClose appends a close frame. Code zero sends an empty close payload.
func (s *Sender) AppendClose(dst []byte, code uint16, reason string) ([]byte, error) {
	if code == 0 {
		if reason != "" {
			return dst, ErrProtocol
		}
		return s.Append(dst, codec.Close, nil, false)
	}
	if len(reason) > 123 {
		return dst, ErrProtocol
	}
	var payload [125]byte
	binary.BigEndian.PutUint16(payload[:2], code)
	copy(payload[2:], reason)
	return s.Append(dst, codec.Close, payload[:2+len(reason)], false)
}
