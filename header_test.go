package ews

import (
	"bytes"
	"testing"
)

func TestHeaderLengthsAndMasking(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		want uint64
	}{
		{"empty", []byte{0x82, 0}, 0},
		{"short", []byte{0x82, 5}, 5},
		{"short_max", []byte{0x82, 125}, 125},
		{"extended16_min", []byte{0x82, 126, 0, 126}, 126},
		{"extended16_byte_order", []byte{0x82, 126, 0x12, 0x34}, 0x1234},
		{"extended16_max", []byte{0x82, 126, 0xff, 0xff}, 65535},
		{"extended64_min", []byte{0x82, 127, 0, 0, 0, 0, 0, 1, 0, 0}, 65536},
		{"extended64_byte_order", []byte{0x82, 127, 1, 2, 3, 4, 5, 6, 7, 8}, 0x0102030405060708},
		{"extended64_max", []byte{0x82, 127, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 1<<63 - 1},
	}
	for _, tt := range tests {
		for _, masked := range []bool{false, true} {
			name := tt.name + "/unmasked"
			if masked {
				name = tt.name + "/masked"
			}
			t.Run(name, func(t *testing.T) {
				wire := append([]byte(nil), tt.wire...)
				var wantKey []byte
				if masked {
					wire[1] |= 0x80
					wantKey = []byte{0x37, 0xfa, 0x21, 0x3d}
					wire = append(wire, wantKey...)
				}
				var h Header
				copy(h.raw[:], wire)
				h.len = uint8(len(wire))
				if got := h.PayloadLen(); got != tt.want {
					t.Errorf("PayloadLen() = %d, want %d", got, tt.want)
				}
				if h.Masked() != masked {
					t.Errorf("Masked() = %v, want %v", h.Masked(), masked)
				}
				if !bytes.Equal(h.Bytes(), wire) || h.Len() != len(wire) {
					t.Errorf("header bytes/length = %x/%d, want %x/%d", h.Bytes(), h.Len(), wire, len(wire))
				}
				key := h.MaskKey()
				if !bytes.Equal(key, wantKey) || (!masked && key != nil) {
					t.Fatalf("MaskKey() = %x, want %x", key, wantKey)
				}
				if masked {
					key[0] ^= 0xff
					if h.Bytes()[len(wire)-4] != key[0] {
						t.Error("MaskKey() does not borrow header storage")
					}
				}
			})
		}
	}
}

func TestHeaderFlagsAndOpcode(t *testing.T) {
	// Exercise every combination of FIN, reserved bits, and opcode.
	for first := 0; first <= 255; first++ {
		h := Header{len: 2}
		h.raw[0] = byte(first)
		if got, want := h.Final(), first >= 128; got != want {
			t.Errorf("first byte %#x: Final() = %v, want %v", first, got, want)
		}
		if got, want := h.Opcode(), Opcode(first%16); got != want {
			t.Errorf("first byte %#x: Opcode() = %v, want %v", first, got, want)
		}
	}
}

func TestHeaderZeroValue(t *testing.T) {
	var h Header
	if h.Len() != 0 || len(h.Bytes()) != 0 || h.PayloadLen() != 0 ||
		h.Final() || h.Opcode() != Continuation || h.Masked() || h.MaskKey() != nil {
		t.Fatal("unexpected zero-value header fields")
	}
}
