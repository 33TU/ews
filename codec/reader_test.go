package codec

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

type benchmarkReaderSource struct {
	data   []byte
	offset int
}

func (r *benchmarkReaderSource) Read(b []byte) (int, error) {
	n := copy(b, r.data[r.offset:])
	r.offset += n
	if r.offset == len(r.data) {
		r.offset = 0
	}
	return n, nil
}

func BenchmarkReader(b *testing.B) {
	for _, size := range []int{125, 4096, 65536} {
		for _, bufferSize := range []int{1, 128, 4096} {
			b.Run(fmt.Sprintf("size=%d/buffer=%d", size, bufferSize), func(b *testing.B) {
				frame := benchmarkFrame(size, true)
				src := benchmarkReaderSource{data: bytes.Repeat(frame, 64)}
				r := NewReader(&src)
				buf := make([]byte, bufferSize)
				r.d.scratch = make([]byte, 0, MaxHeaderSize)
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					h, err := r.ReadHeader(buf)
					if err != nil || h.PayloadLen() != uint64(size) {
						b.Fatal("header:", err)
					}
					total := 0
					for {
						chunk, done, err := r.ReadPayload(buf)
						if err != nil {
							b.Fatal(err)
						}
						total += len(chunk)
						if done {
							break
						}
					}
					if total != size {
						b.Fatalf("payload size = %d, want %d", total, size)
					}
				}
			})
		}
	}
}

type eofReader struct{ *bytes.Reader }

func (r eofReader) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	if r.Len() == 0 {
		err = io.EOF
	}
	return n, err
}

func TestReaderPayload(t *testing.T) {
	for _, size := range []int{1, 3, 32} {
		r := NewReader(eofReader{bytes.NewReader([]byte{0x82, 3, 'a', 'b', 'c', 0x82, 1, 'd'})})
		b := make([]byte, size)
		for _, want := range []string{"abc", "d"} {
			if _, err := r.ReadHeader(b); err != nil {
				t.Fatalf("size %d: header: %v", size, err)
			}
			var got []byte
			for {
				chunk, done, err := r.ReadPayload(b)
				if err != nil {
					t.Fatalf("size %d: payload: %v", size, err)
				}
				got = append(got, chunk...)
				if done {
					break
				}
			}
			if string(got) != want {
				t.Fatalf("size %d: got %q, want %q", size, got, want)
			}
		}
		if _, err := r.ReadHeader(b); err != io.EOF {
			t.Fatalf("size %d: final error = %v", size, err)
		}
	}
}

func TestReaderHeaderEOF(t *testing.T) {
	wire := []byte{0x82, 0xfe, 0, 126, 1, 2, 3, 4}
	for end := 0; end <= len(wire); end++ {
		for _, together := range []bool{false, true} {
			var src io.Reader = bytes.NewReader(wire[:end])
			if together {
				src = eofReader{src.(*bytes.Reader)}
			}
			r := NewReader(src)
			_, err := r.ReadHeader(make([]byte, 16))
			var want error
			if end == 0 {
				want = io.EOF
			} else if end < len(wire) {
				want = io.ErrUnexpectedEOF
			}
			if err != want {
				t.Fatalf("end=%d together=%v: got %v, want %v", end, together, err, want)
			}
		}
	}
}

func TestReaderTruncatedPayload(t *testing.T) {
	r := NewReader(eofReader{bytes.NewReader([]byte{0x82, 2, 'a'})})
	b := make([]byte, 16)
	if _, err := r.ReadHeader(b); err != nil {
		t.Fatal(err)
	}
	chunk, done, err := r.ReadPayload(b)
	if string(chunk) != "a" || done || err != nil {
		t.Fatalf("got %q, %v, %v", chunk, done, err)
	}
	if _, done, err := r.ReadPayload(b); done || err != io.ErrUnexpectedEOF {
		t.Fatalf("got done=%v, err=%v", done, err)
	}
}
