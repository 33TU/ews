package ws

import (
	"encoding/binary"
	"unicode/utf8"
)

// NoStatus is the close code reported when the peer sent none.
const NoStatus = 1005

// validateClose checks a close frame payload.
func validateClose(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if len(payload) < 2 || len(payload) > 125 || !validCloseCode(binary.BigEndian.Uint16(payload)) {
		return ErrProtocol
	}
	if !utf8.Valid(payload[2:]) {
		return ErrInvalidUTF8
	}
	return nil
}

// parseClose splits a validated close payload into code and reason.
// An empty payload reports NoStatus.
func parseClose(payload []byte) (code uint16, reason []byte) {
	if len(payload) < 2 {
		return NoStatus, nil
	}
	return binary.BigEndian.Uint16(payload), payload[2:]
}

func validCloseCode(code uint16) bool {
	switch {
	case code >= 1000 && code <= 1003, code >= 1007 && code <= 1014:
		return true
	default:
		return code >= 3000 && code <= 4999
	}
}
