package ws_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
)

// replay serves the same wire bytes forever, like a socket that never idles.
type replay struct {
	wire []byte
	off  int
}

func (r *replay) Read(b []byte) (int, error) {
	if r.off == len(r.wire) {
		r.off = 0
	}
	n := copy(b, r.wire[r.off:])
	r.off += n
	return n, nil
}

func (r *replay) Write(b []byte) (int, error) { return len(b), nil }

type discard struct{ io.Reader }

func (discard) Write(b []byte) (int, error) { return len(b), nil }

// encodeWire captures one frame as the given role would send it.
func encodeWire(b *testing.B, role ws.Role, payload []byte) []byte {
	var buf bytes.Buffer
	c, err := ws.NewConn(struct {
		io.Reader
		io.Writer
	}{nil, &buf}, ws.Config{Role: role})
	if err != nil {
		b.Fatal(err)
	}
	if err := c.Write(codec.Binary, payload); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func BenchmarkRead(b *testing.B) {
	for _, size := range []int{125, 4096, 65536} {
		for _, role := range []ws.Role{ws.Server, ws.Client} {
			payload := bytes.Repeat([]byte("x"), size)
			wire := encodeWire(b, 1-role, payload)
			name := fmt.Sprintf("size=%d/role=%d", size, role)
			b.Run("ReadMessage/"+name, func(b *testing.B) {
				c, err := ws.NewConn(&replay{wire: wire}, ws.Config{Role: role})
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					if _, p, err := c.ReadMessage(); err != nil || len(p) != size {
						b.Fatal(len(p), err)
					}
				}
			})
			b.Run("Read/"+name, func(b *testing.B) {
				c, err := ws.NewConn(&replay{wire: wire}, ws.Config{Role: role})
				if err != nil {
					b.Fatal(err)
				}
				buf := make([]byte, 64<<10)
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					if _, err := c.NextMessage(); err != nil {
						b.Fatal(err)
					}
					total := 0
					for {
						n, err := c.Read(buf)
						if err == io.EOF {
							break
						}
						if err != nil {
							b.Fatal(err)
						}
						total += n
					}
					if total != size {
						b.Fatal(total)
					}
				}
			})
		}
	}
}

func BenchmarkWrite(b *testing.B) {
	for _, size := range []int{125, 4096, 65536} {
		for _, role := range []ws.Role{ws.Server, ws.Client} {
			b.Run(fmt.Sprintf("size=%d/role=%d", size, role), func(b *testing.B) {
				c, err := ws.NewConn(discard{}, ws.Config{Role: role})
				if err != nil {
					b.Fatal(err)
				}
				payload := bytes.Repeat([]byte("x"), size)
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					if err := c.Write(codec.Binary, payload); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
