package codec_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/33TU/ews/codec"
)

func TestCodecRoundTrip(t *testing.T) {
	frames := []struct {
		final   bool
		opcode  codec.Opcode
		payload []byte
	}{
		{true, codec.Text, []byte("Hello")},
		{false, codec.Binary, make([]byte, 125)},
		{true, codec.Ping, nil},
		{true, codec.Continuation, make([]byte, 126)},
		{true, codec.Binary, make([]byte, 65535)},
		{true, codec.Binary, make([]byte, 65536)},
		{true, codec.Close, []byte{3, 232}},
	}
	for _, index := range []int{1, 3, 4, 5} {
		for i := range frames[index].payload {
			frames[index].payload[i] = byte(i*37 + index)
		}
	}

	for _, masked := range []bool{false, true} {
		for _, chunkSize := range []int{1, 7, 127, 4096, 1 << 20} {
			t.Run(fmt.Sprintf("masked=%t/chunk=%d", masked, chunkSize), func(t *testing.T) {
				var enc codec.Encoder
				var dec codec.Decoder
				for round := range 2 {
					var wire []byte
					for i, frame := range frames {
						var key *[4]byte
						if masked {
							key = &[4]byte{byte(i + 1), 0xfa, 0x21, byte(round + 1)}
						}
						if err := enc.Encode(frame.final, frame.opcode, frame.payload, key); err != nil {
							t.Fatal(err)
						}
						wire = append(wire, enc.HeaderBytes()...)
						wire = append(wire, enc.PayloadBytes()...)
					}

					feed := func() {
						t.Helper()
						if len(wire) == 0 {
							t.Fatal("decoder requested input beyond the encoded stream")
						}
						n := min(chunkSize, len(wire))
						dec.Feed(wire[:n])
						wire = wire[n:]
					}

					for i, frame := range frames {
						var header codec.Header
						for {
							h, ok, err := dec.NextHeader()
							if err != nil {
								t.Fatalf("frame %d: %v", i, err)
							}
							if ok {
								header = h
								break
							}
							feed()
						}
						if header.Final() != frame.final || header.Opcode() != frame.opcode ||
							header.PayloadLen() != uint64(len(frame.payload)) || header.Masked() != masked {
							t.Fatalf("frame %d: header mismatch", i)
						}

						var key [4]byte
						if masked {
							copy(key[:], header.MaskKey())
							if key != [4]byte{byte(i + 1), 0xfa, 0x21, byte(round + 1)} {
								t.Fatalf("frame %d: mask key mismatch", i)
							}
						}

						var payload []byte
						var offset uint8
						for {
							chunk, done := dec.Payload()
							if masked {
								offset = codec.Mask(chunk, key, offset)
							}
							payload = append(payload, chunk...)
							if done {
								break
							}
							feed()
						}
						if !bytes.Equal(payload, frame.payload) {
							t.Fatalf("frame %d: payload mismatch", i)
						}
					}

					if _, ok, err := dec.NextHeader(); ok || err != nil || len(wire) != 0 {
						t.Fatal("unexpected data after the final frame")
					}

					enc.Reset()
					dec.Reset()
				}
			})
		}
	}
}
