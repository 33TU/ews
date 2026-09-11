package protocol

import (
	"encoding/binary"
	"unicode/utf8"
)

func validateClose(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if len(payload) < 2 || len(payload) > 125 {
		return ErrProtocol
	}
	code := binary.BigEndian.Uint16(payload)
	if !validCloseCode(code) {
		return ErrProtocol
	}
	if !utf8.Valid(payload[2:]) {
		return ErrInvalidUTF8
	}
	return nil
}

func validCloseCode(code uint16) bool {
	switch code {
	case 1000, 1001, 1002, 1003, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014:
		return true
	default:
		return code >= 3000 && code <= 4999
	}
}
