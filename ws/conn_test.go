package ws_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"testing"
	"testing/iotest"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/deflate"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

// pair connects a server and a client Conn over net.Pipe.
func pair(t testing.TB, serverCfg, clientCfg ws.Config) (server, client *ws.Conn) {
	t.Helper()
	sc, cc := net.Pipe()
	t.Cleanup(func() { sc.Close(); cc.Close() })
	serverCfg.Role, clientCfg.Role = ws.Server, ws.Client
	var err error
	if server, err = ws.NewConn(sc, serverCfg); err != nil {
		t.Fatal(err)
	}
	if client, err = ws.NewConn(cc, clientCfg); err != nil {
		t.Fatal(err)
	}
	return server, client
}

// raw connects a server Conn to a bare pipe end for hand-built frames.
func raw(t testing.TB, cfg ws.Config) (*ws.Conn, net.Conn) {
	t.Helper()
	sc, cc := net.Pipe()
	t.Cleanup(func() { sc.Close(); cc.Close() })
	cfg.Role = ws.Server
	server, err := ws.NewConn(sc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return server, cc
}

// frame builds one masked frame and overwrites its first byte.
func frame(t testing.TB, first byte, payload []byte) []byte {
	t.Helper()
	var enc codec.Encoder
	if err := enc.Encode(true, codec.Binary, payload, &[4]byte{17, 28, 39, 40}); err != nil {
		t.Fatal(err)
	}
	out := append(bytes.Clone(enc.HeaderBytes()), enc.PayloadBytes()...)
	out[0] = first
	return out
}

// readFrame reads one complete unmasked frame from a bare pipe end.
func readFrame(t testing.TB, r io.Reader) (codec.Header, []byte) {
	t.Helper()
	var dec codec.Decoder
	var buf [512]byte
	var payload []byte
	for {
		h, ok, err := dec.NextHeader()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			for {
				p, done := dec.Payload()
				payload = append(payload, p...)
				if done {
					return h, payload
				}
				dec.Preserve()
				n, err := r.Read(buf[:])
				if err != nil {
					t.Fatal(err)
				}
				dec.Feed(buf[:n])
			}
		}
		dec.Preserve()
		n, err := r.Read(buf[:])
		if err != nil {
			t.Fatal(err)
		}
		dec.Feed(buf[:n])
	}
}

// run executes f in a goroutine and reports its error through the returned function.
func run(t testing.TB, f func() error) func() {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	return func() {
		t.Helper()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("goroutine timed out")
		}
	}
}

type message struct {
	op      codec.Opcode
	payload []byte
}

func messages() []message {
	return []message{
		{codec.Text, nil},
		{codec.Text, []byte("hello 😀")},
		{codec.Binary, bytes.Repeat([]byte("message"), 1000)},
		{codec.Text, bytes.Repeat([]byte("κόσμε"), 1000)},
		{codec.Binary, bytes.Repeat([]byte("x"), 100000)},
		{codec.Binary, []byte{255, 0, 254}},
	}
}

func TestRoundTrip(t *testing.T) {
	messages := messages()
	for _, direction := range []string{"client->server", "server->client"} {
		for _, bufSize := range []int{0, 1, 7, 4096, 65536} {
			t.Run(fmt.Sprintf("%s/buf=%d", direction, bufSize), func(t *testing.T) {
				server, client := pair(t, ws.Config{ReadBufferSize: 64}, ws.Config{ReadBufferSize: 64})
				src, dst := client, server
				if direction != "client->server" {
					src, dst = server, client
				}
				wait := run(t, func() error {
					for _, m := range messages {
						if err := src.Write(m.op, m.payload); err != nil {
							return err
						}
					}
					return nil
				})
				for i, m := range messages {
					want, wantOp := m.payload, m.op
					var op codec.Opcode
					var got []byte
					var err error
					if bufSize == 0 {
						op, got, err = dst.ReadMessage()
						if err != nil {
							t.Fatal(err)
						}
					} else {
						op, err = dst.NextMessage()
						if err != nil {
							t.Fatal(err)
						}
						buf := make([]byte, bufSize)
						got = got[:0]
						for {
							n, err := dst.Read(buf)
							if err == io.EOF {
								break
							}
							if err != nil {
								t.Fatal(err)
							}
							if n == 0 {
								t.Fatal("Read returned 0, nil")
							}
							got = append(got, buf[:n]...)
						}
						if n, err := dst.Read(buf); n != 0 || err != io.EOF {
							t.Fatal("second EOF read failed")
						}
					}
					if op != wantOp || !bytes.Equal(got, want) {
						t.Fatalf("message %d: opcode %d, %d bytes", i, op, len(got))
					}
				}
				wait()
			})
		}
	}
}

func TestFragmentsAndControls(t *testing.T) {
	server, peer := raw(t, ws.Config{ReadBufferSize: 16})
	payload := []byte("A😀B€C")
	var frames [][]byte
	for i := range payload {
		op := byte(codec.Continuation)
		if i == 0 {
			op = byte(codec.Text)
		}
		if i == len(payload)-1 {
			op |= 0x80
		}
		frames = append(frames, frame(t, op, payload[i:i+1]))
	}
	wait := run(t, func() error {
		for i, f := range frames {
			if _, err := peer.Write(f); err != nil {
				return err
			}
			if i == 1 {
				if _, err := peer.Write(frame(t, 0x89, []byte("ping"))); err != nil {
					return err
				}
				h, p := readFrame(t, peer)
				if h.Opcode() != codec.Pong || string(p) != "ping" || h.Masked() {
					return fmt.Errorf("expected pong, got %d %q", h.Opcode(), p)
				}
			}
		}
		return nil
	})
	op, err := server.NextMessage()
	if err != nil || op != codec.Text {
		t.Fatal(op, err)
	}
	var got []byte
	buf := make([]byte, 3)
	for {
		n, err := server.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, buf[:n]...)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
	wait()
}

func TestCloseHandshake(t *testing.T) {
	server, client := pair(t, ws.Config{}, ws.Config{})
	wait := run(t, func() error {
		if err := client.Close(1000, "bye"); err != nil {
			return err
		}
		if err := client.Close(1000, ""); err != ws.ErrClosing {
			return fmt.Errorf("second close: %v", err)
		}
		if err := client.Write(codec.Text, []byte("late")); err != ws.ErrClosing {
			return fmt.Errorf("write after close: %v", err)
		}
		_, _, err := client.ReadMessage()
		if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1000 || ce.Reason != "" {
			return fmt.Errorf("client read: %v", err)
		}
		return nil
	})
	_, _, err := server.ReadMessage()
	ce, ok := errors.AsType[*ws.CloseError](err)
	if !ok || ce.Code != 1000 || ce.Reason != "bye" {
		t.Fatalf("server read: %v", err)
	}
	if !server.CloseSent() {
		t.Fatal("default handler did not echo close")
	}
	if _, err := server.NextMessage(); err != error(ce) {
		t.Fatal("close error not sticky")
	}
	if err := server.Write(codec.Binary, nil); err != ws.ErrClosing {
		t.Fatal("data after close accepted")
	}
	wait()
}

func TestCloseWithoutStatus(t *testing.T) {
	server, peer := raw(t, ws.Config{})
	wait := run(t, func() error {
		if _, err := peer.Write(frame(t, 0x88, nil)); err != nil {
			return err
		}
		h, p := readFrame(t, peer)
		if h.Opcode() != codec.Close || len(p) != 0 {
			return fmt.Errorf("expected empty close echo, got %d %x", h.Opcode(), p)
		}
		return nil
	})
	_, _, err := server.ReadMessage()
	if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != ws.NoStatus {
		t.Fatal(err)
	}
	wait()
}

func TestProtocolError(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		code uint16
	}{
		{"rsv1", frame(t, 0xc1, nil), 1002},
		{"unmasked", []byte{0x81, 1, 'a'}, 1002},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, peer := raw(t, ws.Config{})
			wait := run(t, func() error {
				if _, err := peer.Write(tt.wire); err != nil {
					return err
				}
				h, p := readFrame(t, peer)
				if h.Opcode() != codec.Close || len(p) != 2 || uint16(p[0])<<8|uint16(p[1]) != tt.code {
					return fmt.Errorf("expected close %d, got %d %x", tt.code, h.Opcode(), p)
				}
				return nil
			})
			_, _, err := server.ReadMessage()
			we, ok := errors.AsType[*ws.Error](err)
			if !ok || we.Code != tt.code {
				t.Fatalf("got %v, want code %d", err, tt.code)
			}
			if _, err := server.NextMessage(); err != error(we) {
				t.Fatal("failure not sticky")
			}
			wait()
		})
	}
}

func TestTextValidation(t *testing.T) {
	// Off by default: invalid text is delivered like binary.
	server, peer := raw(t, ws.Config{})
	wait := run(t, func() error {
		_, err := peer.Write(frame(t, 0x81, []byte{255}))
		return err
	})
	if op, p, err := server.ReadMessage(); err != nil || op != codec.Text || !bytes.Equal(p, []byte{255}) {
		t.Fatalf("default must not validate: %d %x %v", op, p, err)
	}
	wait()

	server, peer = raw(t, ws.Config{ValidateUTF8: true})
	wire := append(frame(t, 0x01, []byte{0xe2, 0x82}), frame(t, 0x80, []byte{0x41})...) // Broken rune across frames.
	wait = run(t, func() error {
		if _, err := peer.Write(wire); err != nil {
			return err
		}
		h, p := readFrame(t, peer)
		if h.Opcode() != codec.Close || len(p) != 2 || p[0] != 3 || p[1] != 239 {
			return fmt.Errorf("expected close 1007, got %d %x", h.Opcode(), p)
		}
		return nil
	})
	_, _, err := server.ReadMessage()
	if we, ok := errors.AsType[*ws.Error](err); !ok || we.Code != 1007 || !errors.Is(err, ws.ErrInvalidUTF8) {
		t.Fatal(err)
	}
	wait()

	// Chunked reads deliver text unvalidated even when enabled; only whole
	// messages can be checked.
	server, peer = raw(t, ws.Config{ValidateUTF8: true})
	wait = run(t, func() error {
		_, err := peer.Write(frame(t, 0x81, []byte{255, 254}))
		return err
	})
	if op, err := server.NextMessage(); err != nil || op != codec.Text {
		t.Fatal(op, err)
	}
	if p, err := io.ReadAll(server); err != nil || !bytes.Equal(p, []byte{255, 254}) {
		t.Fatalf("%x %v", p, err)
	}
	wait()
}

func TestMessageTooLarge(t *testing.T) {
	// The failed server never reads again, so the client must not echo the close.
	server, client := pair(t, ws.Config{MaxMessageSize: 10}, ws.Config{ControlHandler: silentClose{}})
	big := bytes.Repeat([]byte("y"), 11)
	wait := run(t, func() error { return client.Write(codec.Binary, big) })
	op, err := server.NextMessage()
	if err != nil || op != codec.Binary {
		t.Fatal(op, err)
	}
	got, err := io.ReadAll(server)
	if err != nil || !bytes.Equal(got, big) {
		t.Fatalf("chunked read must ignore the limit: %v", err)
	}
	wait()
	// The server fails on the header and never reads the body, so over net.Pipe
	// this write completes only when Cleanup closes the pipe.
	go client.Write(codec.Binary, big)
	wait = run(t, func() error {
		_, _, err := client.ReadMessage()
		if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1009 {
			return fmt.Errorf("client expected close 1009, got %v", err)
		}
		return nil
	})
	_, _, err = server.ReadMessage()
	if we, ok := errors.AsType[*ws.Error](err); !ok || we.Code != 1009 || !errors.Is(err, ws.ErrMessageTooLarge) {
		t.Fatal(err)
	}
	wait()
}

func TestNextMessageDiscards(t *testing.T) {
	server, client := pair(t, ws.Config{ReadBufferSize: 8}, ws.Config{})
	wait := run(t, func() error {
		if err := client.Write(codec.Binary, bytes.Repeat([]byte("a"), 1000)); err != nil {
			return err
		}
		return client.Write(codec.Text, []byte("second"))
	})
	if _, err := server.NextMessage(); err != nil {
		t.Fatal(err)
	}
	if n, err := server.Read(make([]byte, 1)); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	op, p, err := server.ReadMessage()
	if err != nil || op != codec.Text || string(p) != "second" {
		t.Fatalf("%d %q %v", op, p, err)
	}
	if n, err := server.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatal("Read outside a message must return EOF")
	}
	wait()
}

// failingWriter accepts limit bytes, then fails.
type failingWriter struct {
	bytes.Buffer
	limit int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.Len()+len(p) > w.limit {
		return 0, errors.New("writer full")
	}
	return w.Buffer.Write(p)
}

func TestWriteTo(t *testing.T) {
	// A small read buffer exercises both the borrowed path and the direct
	// read of long remainders; a small fragment size makes those pieces many.
	// Pings go unanswered: over net.Pipe a pong written while the client is
	// still writing would deadlock both ends.
	server, client := pair(t, ws.Config{ReadBufferSize: 8, FragmentSize: 1000, ControlHandler: pingFails{}}, ws.Config{})
	wait := run(t, func() error {
		for _, m := range messages() {
			if err := client.Write(m.op, m.payload); err != nil {
				return err
			}
		}
		// A fragmented message with a ping in the middle.
		if err := client.BeginMessage(codec.Binary); err != nil {
			return err
		}
		for i := range 3 {
			if err := client.WriteChunk(bytes.Repeat([]byte{byte('a' + i)}, 5000)); err != nil {
				return err
			}
			if i == 1 {
				if err := client.Ping([]byte("mid")); err != nil {
					return err
				}
			}
		}
		if err := client.EndMessage(); err != nil {
			return err
		}
		if err := client.Write(codec.Binary, bytes.Repeat([]byte("z"), 3000)); err != nil {
			return err
		}
		if err := client.Write(codec.Text, []byte("after")); err != nil {
			return err
		}
		if _, p, err := client.ReadMessage(); err != nil || string(p) != "done" {
			return fmt.Errorf("final message: %q %v", p, err)
		}
		return nil
	})
	// Nothing open: nothing written.
	if n, err := server.WriteTo(io.Discard); n != 0 || err != nil {
		t.Fatal(n, err)
	}
	var buf bytes.Buffer
	for i, m := range messages() {
		op, err := server.NextMessage()
		if err != nil || op != m.op {
			t.Fatalf("message %d: %d %v", i, op, err)
		}
		buf.Reset()
		// io.Copy picks WriteTo up through io.WriterTo.
		n, err := io.Copy(&buf, server)
		if err != nil || n != int64(len(m.payload)) || !bytes.Equal(buf.Bytes(), m.payload) {
			t.Fatalf("message %d: %d bytes, %v", i, n, err)
		}
		if n, err := server.WriteTo(&buf); n != 0 || err != nil {
			t.Fatalf("message %d: WriteTo after the end wrote %d, %v", i, n, err)
		}
	}
	if _, err := server.NextMessage(); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if n, err := server.WriteTo(&buf); err != nil || n != 15000 {
		t.Fatal(n, err)
	}
	for i := range 3 {
		if !bytes.Equal(buf.Bytes()[i*5000:(i+1)*5000], bytes.Repeat([]byte{byte('a' + i)}, 5000)) {
			t.Fatalf("fragment %d corrupted", i)
		}
	}
	// A failing writer stops the message; NextMessage discards the rest.
	if _, err := server.NextMessage(); err != nil {
		t.Fatal(err)
	}
	fw := &failingWriter{limit: 1500}
	if _, err := server.WriteTo(fw); err == nil || err.Error() != "writer full" {
		t.Fatalf("writer error not returned: %v", err)
	}
	if op, p, err := server.ReadMessage(); err != nil || op != codec.Text || string(p) != "after" {
		t.Fatalf("%d %q %v", op, p, err)
	}
	if err := server.Write(codec.Text, []byte("done")); err != nil {
		t.Fatal(err)
	}
	wait()

	// Compressed messages are inflated whole and written in one call.
	cserver, cclient := compressionPair(t, true, true, 1)
	payload := bytes.Repeat([]byte("compressed stream payload "), 4000)
	cwait := run(t, func() error { return cclient.Write(codec.Text, payload) })
	if _, err := cserver.NextMessage(); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if n, err := cserver.WriteTo(&buf); err != nil || n != int64(len(payload)) || !bytes.Equal(buf.Bytes(), payload) {
		t.Fatal(n, err)
	}
	cwait()
}

func TestTransportEOF(t *testing.T) {
	server, peer := raw(t, ws.Config{})
	wait := run(t, func() error { return peer.Close() })
	if _, _, err := server.ReadMessage(); err != io.EOF {
		t.Fatalf("clean EOF: %v", err)
	}
	wait()

	server, peer = raw(t, ws.Config{})
	partial := frame(t, 0x82, []byte("abcdef"))
	wait = run(t, func() error {
		if _, err := peer.Write(partial[:len(partial)-2]); err != nil {
			return err
		}
		return peer.Close()
	})
	if _, _, err := server.ReadMessage(); err != io.ErrUnexpectedEOF {
		t.Fatalf("mid-frame EOF: %v", err)
	}
	wait()
}

func TestControlHandler(t *testing.T) {
	boom := errors.New("boom")
	server, peer := raw(t, ws.Config{ControlHandler: pingFails{boom}})
	wait := run(t, func() error {
		_, err := peer.Write(frame(t, 0x89, nil))
		return err
	})
	if _, _, err := server.ReadMessage(); err != boom {
		t.Fatal(err)
	}
	wait()

	server, peer = raw(t, ws.Config{ControlHandler: silentClose{}})
	wait = run(t, func() error {
		_, err := peer.Write(frame(t, 0x88, []byte{3, 233}))
		return err
	})
	_, _, err := server.ReadMessage()
	if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1001 || server.CloseSent() {
		t.Fatal(err)
	}
	wait()
}

type pingFails struct{ err error }

func (h pingFails) OnPing(*ws.Conn, []byte) error        { return h.err }
func (pingFails) OnPong(*ws.Conn, []byte) error          { return nil }
func (pingFails) OnClose(*ws.Conn, uint16, []byte) error { return nil }

type silentClose struct{ ws.DefaultControlHandler }

func (silentClose) OnClose(*ws.Conn, uint16, []byte) error { return nil }

func TestConcurrentWriters(t *testing.T) {
	server, client := pair(t, ws.Config{}, ws.Config{})
	const writers, each = 4, 50
	waitServer := run(t, func() error {
		for range writers * each {
			if _, _, err := server.ReadMessage(); err != nil {
				return err
			}
		}
		return nil
	})
	waitClient := run(t, func() error {
		for range writers * each {
			if _, _, err := client.ReadMessage(); err != nil {
				return err
			}
		}
		return nil
	})
	var waits []func()
	for w := range writers {
		payload := bytes.Repeat([]byte{byte(w)}, 100+w)
		waits = append(waits, run(t, func() error {
			for range each {
				if err := client.Write(codec.Binary, payload); err != nil {
					return err
				}
				if err := server.Write(codec.Binary, payload); err != nil {
					return err
				}
			}
			return nil
		}))
	}
	for _, w := range waits {
		w()
	}
	waitServer()
	waitClient()
}

// recorder is a writer without writev that records each Write call.
type recorder struct{ writes [][]byte }

func (r *recorder) Read([]byte) (int, error) { return 0, io.EOF }
func (r *recorder) Write(p []byte) (int, error) {
	r.writes = append(r.writes, bytes.Clone(p))
	return len(p), nil
}

// TestWriteCoalescing checks that transports without writev get one write
// per small frame and two per large one, and that sockets are detected.
func TestWriteCoalescing(t *testing.T) {
	rec := new(recorder)
	c, err := ws.NewConn(rec, ws.Config{Role: ws.Server})
	if err != nil {
		t.Fatal(err)
	}
	small := bytes.Repeat([]byte("s"), 1000)
	large := bytes.Repeat([]byte("l"), 100000)
	if err := c.Write(codec.Binary, small); err != nil {
		t.Fatal(err)
	}
	if err := c.Write(codec.Binary, large); err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 4 || len(rec.writes[0]) != 4+len(small) || len(rec.writes[1]) != 10 || len(rec.writes[2]) != len(large) || len(rec.writes[3]) != 2 {
		sizes := make([]int, len(rec.writes))
		for i, w := range rec.writes {
			sizes[i] = len(w)
		}
		t.Fatalf("write sizes %v", sizes)
	}
	if !bytes.Equal(rec.writes[0][4:], small) {
		t.Fatal("coalesced payload corrupted")
	}

	// Frames are unchanged on the wire: a raw peer decodes both.
	server, peer := raw(t, ws.Config{})
	wait := run(t, func() error {
		for i, want := range [][]byte{small, large} {
			h, p := readFrame(t, peer)
			if h.Opcode() != codec.Binary || !bytes.Equal(p, want) {
				return fmt.Errorf("frame %d: %d bytes", i, len(p))
			}
		}
		return nil
	})
	server.Write(codec.Binary, small)
	server.Write(codec.Binary, large)
	wait()
}

func TestInvalidConfig(t *testing.T) {
	sc, _ := net.Pipe()
	defer sc.Close()
	for _, cfg := range []ws.Config{
		{Role: 2},
		{ReadBufferSize: -1},
		{MaxMessageSize: -1},
		{Compression: &handshake.Compression{Level: 10}},
		{Compression: &handshake.Compression{MinSize: -1}},
		{Compression: &handshake.Compression{SendWindowBits: 7}},
		{Compression: &handshake.Compression{ReceiveWindowBits: 16}},
		{FragmentSize: -1},
	} {
		if _, err := ws.NewConn(sc, cfg); err != ws.ErrInvalidConfig {
			t.Fatalf("%+v accepted", cfg)
		}
	}
	if _, err := ws.NewConn(nil, ws.Config{}); err != ws.ErrInvalidConfig {
		t.Fatal("nil transport accepted")
	}
}

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
			b.Run("WriteTo/"+name, func(b *testing.B) {
				c, err := ws.NewConn(&replay{wire: wire}, ws.Config{Role: role})
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					if _, err := c.NextMessage(); err != nil {
						b.Fatal(err)
					}
					if n, err := c.WriteTo(io.Discard); err != nil || n != int64(size) {
						b.Fatal(n, err)
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

func compressionPair(t *testing.T, clientTakeover, serverTakeover bool, minSize int) (server, client *ws.Conn) {
	t.Helper()
	sc := &handshake.Compression{Level: flate.BestSpeed, MinSize: max(minSize, 1), SendContextTakeover: serverTakeover, ReceiveContextTakeover: clientTakeover}
	cc := &handshake.Compression{Level: flate.BestSpeed, MinSize: max(minSize, 1), SendContextTakeover: clientTakeover, ReceiveContextTakeover: serverTakeover}
	return pair(t, ws.Config{Compression: sc, ReadBufferSize: 64}, ws.Config{Compression: cc, ReadBufferSize: 64})
}

func TestCompressedRoundTrip(t *testing.T) {
	msgs := messages()
	for _, takeover := range []string{"none", "client", "server", "both"} {
		for _, minSize := range []int{0, 1000} {
			for _, direction := range []string{"client->server", "server->client"} {
				for _, bufSize := range []int{0, 1, 7, 65536} {
					t.Run(fmt.Sprintf("takeover=%s/min=%d/%s/buf=%d", takeover, minSize, direction, bufSize), func(t *testing.T) {
						server, client := compressionPair(t, takeover == "client" || takeover == "both", takeover == "server" || takeover == "both", minSize)
						src, dst := client, server
						if direction != "client->server" {
							src, dst = server, client
						}
						wait := run(t, func() error {
							for range 2 { // Takeover history spans messages.
								for _, m := range msgs {
									if err := src.Write(m.op, m.payload); err != nil {
										return err
									}
								}
							}
							return nil
						})
						buf := make([]byte, bufSize)
						for round := range 2 {
							for i, m := range msgs {
								var op codec.Opcode
								var got []byte
								var err error
								if bufSize == 0 {
									op, got, err = dst.ReadMessage()
								} else {
									op, err = dst.NextMessage()
									for err == nil {
										var n int
										n, err = dst.Read(buf)
										got = append(got, buf[:n]...)
									}
									if err == io.EOF {
										err = nil
									}
								}
								if err != nil || op != m.op || !bytes.Equal(got, m.payload) {
									t.Fatalf("round %d message %d: op %d, %d bytes, %v", round, i, op, len(got), err)
								}
							}
						}
						wait()
					})
				}
			}
		}
	}
}

// TestCompressedWire checks the frames a compressing Conn emits and that it
// accepts a fragmented compressed message with a ping in the middle.
func TestCompressedWire(t *testing.T) {
	comp := &handshake.Compression{Level: flate.BestSpeed, MinSize: 8}
	server, peer := raw(t, ws.Config{Compression: comp, ReadBufferSize: 16})
	payload := bytes.Repeat([]byte("compress me "), 100)

	wait := run(t, func() error {
		h, p := readFrame(t, peer)
		if !h.RSV1() || h.Opcode() != codec.Text {
			return fmt.Errorf("expected compressed text frame, got %x", h.Bytes())
		}
		var d deflate.Decompressor
		out, err := d.Decompress(p, len(payload), nil)
		if err != nil || !bytes.Equal(out, payload) {
			return fmt.Errorf("wire payload: %v", err)
		}
		h, p = readFrame(t, peer)
		if h.RSV1() || string(p) != "tiny" {
			return fmt.Errorf("short message must go uncompressed: %x", h.Bytes())
		}

		c, err := deflate.NewCompressor(flate.BestSpeed)
		if err != nil {
			return err
		}
		compressed, err := c.Compress(payload, nil)
		if err != nil {
			return err
		}
		var enc codec.Encoder
		key := &[4]byte{9, 8, 7, 6}
		for i := 0; i < len(compressed); i += 5 {
			end := min(i+5, len(compressed))
			op, final := codec.Continuation, end == len(compressed)
			if i == 0 {
				op = codec.Text
			}
			if err := enc.EncodeCompressed(final, op, compressed[i:end], key); err != nil {
				return err
			}
			if _, err := peer.Write(append(bytes.Clone(enc.HeaderBytes()), enc.PayloadBytes()...)); err != nil {
				return err
			}
			if i == 5 {
				if _, err := peer.Write(frame(t, 0x89, []byte("ping"))); err != nil {
					return err
				}
				if h, p := readFrame(t, peer); h.Opcode() != codec.Pong || string(p) != "ping" {
					return fmt.Errorf("expected pong")
				}
			}
		}
		return nil
	})
	if err := server.Write(codec.Text, payload); err != nil {
		t.Fatal(err)
	}
	if err := server.Write(codec.Text, []byte("tiny")); err != nil {
		t.Fatal(err)
	}
	// With MinSize zero the default of 128 applies: a 100-byte text stays plain.
	def, defPeer := raw(t, ws.Config{Compression: &handshake.Compression{Level: flate.BestSpeed}})
	waitDef := run(t, func() error {
		if h, _ := readFrame(t, defPeer); h.RSV1() {
			return fmt.Errorf("default MinSize compressed a 100-byte message")
		}
		return nil
	})
	if err := def.Write(codec.Text, bytes.Repeat([]byte("d"), 100)); err != nil {
		t.Fatal(err)
	}
	waitDef()
	op, err := server.NextMessage()
	if err != nil || op != codec.Text {
		t.Fatal(op, err)
	}
	got, err := io.ReadAll(server)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("%d bytes, %v", len(got), err)
	}
	wait()
}

func TestCompressedErrors(t *testing.T) {
	comp := &handshake.Compression{Level: flate.BestSpeed}
	for _, chunked := range []bool{false, true} {
		server, peer := raw(t, ws.Config{Compression: comp})
		wait := run(t, func() error {
			if _, err := peer.Write(frame(t, 0xc1, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})); err != nil {
				return err
			}
			if h, p := readFrame(t, peer); h.Opcode() != codec.Close || len(p) < 2 || uint16(p[0])<<8|uint16(p[1]) != 1007 {
				return fmt.Errorf("expected close 1007, got %d %x", h.Opcode(), p)
			}
			return nil
		})
		var err error
		if chunked {
			if _, err = server.NextMessage(); err == nil {
				_, err = server.Read(make([]byte, 16))
			}
		} else {
			_, _, err = server.ReadMessage()
		}
		if we, ok := errors.AsType[*ws.Error](err); !ok || we.Code != 1007 || !errors.Is(err, ws.ErrInvalidData) {
			t.Fatalf("chunked=%t: %v", chunked, err)
		}
		wait()
	}

	// Compressed wire fits the budget but the message does not.
	server, client := pair(t, ws.Config{Compression: comp, MaxMessageSize: 100}, ws.Config{Compression: comp, ControlHandler: silentClose{}})
	big := bytes.Repeat([]byte("a"), 1000)
	go client.Write(codec.Binary, big) // Read by the server before failing.
	wait := run(t, func() error {
		_, _, err := client.ReadMessage()
		if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1009 {
			return fmt.Errorf("client expected close 1009, got %v", err)
		}
		return nil
	})
	_, _, err := server.ReadMessage()
	if we, ok := errors.AsType[*ws.Error](err); !ok || we.Code != 1009 {
		t.Fatal(err)
	}
	wait()
}

// TestCompressionShared re-primes a pooled compressor from the window on every
// message and must stay decodable by a peer keeping its own history.
func TestCompressionShared(t *testing.T) {
	sc := &handshake.Compression{Level: flate.BestSpeed, SendContextTakeover: true}
	cc := &handshake.Compression{Level: flate.BestSpeed, ReceiveContextTakeover: true}
	server, client := pair(t, ws.Config{Compression: sc, CompressionShared: true}, ws.Config{Compression: cc})
	payload := bytes.Repeat([]byte("shared compressor, private window "), 200)
	wait := run(t, func() error {
		for i := range 5 {
			if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, payload) {
				return fmt.Errorf("message %d: %v", i, err)
			}
		}
		return nil
	})
	for range 5 {
		if err := server.Write(codec.Text, payload); err != nil {
			t.Fatal(err)
		}
	}
	wait()
}

// TestSmallWindow sends through reduced windows in both modes; the peer
// keeps a matching 9-bit receive window.
func TestSmallWindow(t *testing.T) {
	for _, shared := range []bool{false, true} {
		sc := &handshake.Compression{Level: flate.BestSpeed, MinSize: 1, SendContextTakeover: true, SendWindowBits: 9}
		cc := &handshake.Compression{Level: flate.BestSpeed, ReceiveContextTakeover: true, ReceiveWindowBits: 9}
		server, client := pair(t, ws.Config{Compression: sc, CompressionShared: shared}, ws.Config{Compression: cc})
		payload := bytes.Repeat([]byte("a 512 byte window still compresses repeats "), 400)
		wait := run(t, func() error {
			for i := range 3 {
				if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, payload) {
					return fmt.Errorf("message %d: %v", i, err)
				}
			}
			return nil
		})
		for range 3 {
			if err := server.Write(codec.Binary, payload); err != nil {
				t.Fatal(err)
			}
		}
		wait()
	}
}

// TestFragmentedSend streams messages in chunks, plain and compressed, in
// attached and shared modes, with a ping between chunks, and checks the frame
// structure through a raw peer.
func TestFragmentedSend(t *testing.T) {
	chunks := [][]byte{bytes.Repeat([]byte("first chunk "), 300), []byte("middle"), nil, bytes.Repeat([]byte("last chunk "), 500)}
	var whole []byte
	for _, ch := range chunks {
		whole = append(whole, ch...)
	}
	for _, mode := range []string{"plain", "compressed", "compressed-shared", "compressed-takeover"} {
		t.Run(mode, func(t *testing.T) {
			var cfg ws.Config
			if mode != "plain" {
				cfg.Compression = &handshake.Compression{Level: flate.BestSpeed, MinSize: 1 << 20, SendContextTakeover: mode == "compressed-takeover"}
				cfg.CompressionShared = mode == "compressed-shared"
			}
			server, peer := raw(t, cfg)
			wait := run(t, func() error {
				var assembled []byte
				for i := 0; ; i++ {
					h, p := readFrame(t, peer)
					if h.Opcode() == codec.Ping {
						continue // Nobody reads on the server side, so do not answer.
					}
					if i == 0 && (h.Opcode() != codec.Text || h.RSV1() != (mode != "plain")) || i > 0 && h.Opcode() != codec.Continuation || h.RSV1() && i > 0 {
						return fmt.Errorf("frame %d header %x", i, h.Bytes())
					}
					assembled = append(assembled, p...)
					if h.Final() {
						// A compressed message's final fragment carries the
						// header byte of the trimmed sync-flush block.
						if len(p) > 1 || len(p) != 0 && mode == "plain" {
							return fmt.Errorf("final frame carried %d bytes", len(p))
						}
						break
					}
				}
				if mode != "plain" {
					var d deflate.Decompressor
					out, err := d.Decompress(assembled, len(whole), nil)
					if err != nil {
						return err
					}
					assembled = bytes.Clone(out)
				}
				if !bytes.Equal(assembled, whole) {
					return fmt.Errorf("assembled %d bytes, want %d", len(assembled), len(whole))
				}
				return nil
			})
			if err := server.BeginMessage(codec.Text); err != nil {
				t.Fatal(err)
			}
			if err := server.BeginMessage(codec.Text); err != ws.ErrMessageOpen {
				t.Fatal("nested message accepted")
			}
			if err := server.Write(codec.Text, []byte("x")); err != ws.ErrMessageOpen {
				t.Fatal("data write during fragmented message accepted")
			}
			for i, ch := range chunks {
				if err := server.WriteChunk(ch); err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					if err := server.Ping([]byte("mid")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := server.EndMessage(); err != nil {
				t.Fatal(err)
			}
			if err := server.EndMessage(); err != ws.ErrNoMessage {
				t.Fatal("EndMessage without BeginMessage accepted")
			}
			wait()
		})
	}

	// Through a real peer connection, across takeover history, as whole messages read.
	server, client := compressionPair(t, true, true, 1<<20)
	wait := run(t, func() error {
		for i := range 3 {
			if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, whole) {
				return fmt.Errorf("message %d: %v", i, err)
			}
		}
		return nil
	})
	for range 3 {
		if err := server.BeginMessage(codec.Binary); err != nil {
			t.Fatal(err)
		}
		for _, ch := range chunks {
			if err := server.WriteChunk(ch); err != nil {
				t.Fatal(err)
			}
		}
		if err := server.EndMessage(); err != nil {
			t.Fatal(err)
		}
	}
	wait()
}

// failingReader returns its content, then an error instead of EOF.
type failingReader struct {
	r   io.Reader
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, f.err
	}
	return n, err
}

func TestWriteFrom(t *testing.T) {
	small := bytes.Repeat([]byte("s"), 1000)
	large := bytes.Repeat([]byte("L"), 100000)

	// Frame structure through a raw peer with 32 KiB fragments: one frame
	// when it fits, else fragments with the last carrying FIN.
	// iotest.OneByteReader forces ReadFull to assemble chunks from tiny reads.
	server, peer := raw(t, ws.Config{FragmentSize: 32 << 10})
	wait := run(t, func() error {
		h, p := readFrame(t, peer)
		if !h.Final() || h.Opcode() != codec.Text || !bytes.Equal(p, small) {
			return fmt.Errorf("small: header %x, %d bytes", h.Bytes(), len(p))
		}
		var sizes []int
		var assembled []byte
		for {
			h, p := readFrame(t, peer)
			sizes = append(sizes, len(p))
			assembled = append(assembled, p...)
			if h.Final() {
				break
			}
		}
		want := []int{32768, 32768, 32768, 100000 - 3*32768} // The last chunk carries FIN.
		if fmt.Sprint(sizes) != fmt.Sprint(want) || !bytes.Equal(assembled, large) {
			return fmt.Errorf("large: fragment sizes %v", sizes)
		}
		return nil
	})
	if n, err := server.WriteFrom(codec.Text, bytes.NewReader(small)); err != nil || n != int64(len(small)) {
		t.Fatal(n, err)
	}
	if n, err := server.WriteFrom(codec.Binary, iotest.OneByteReader(bytes.NewReader(large))); err != nil || n != int64(len(large)) {
		t.Fatal(n, err)
	}
	wait()

	// Content within the default fragment size is a single frame.
	server, peer = raw(t, ws.Config{})
	fits := large[:ws.DefaultFragmentSize]
	wait = run(t, func() error {
		if h, p := readFrame(t, peer); !h.Final() || len(p) != len(fits) {
			return fmt.Errorf("default: final %v, %d bytes", h.Final(), len(p))
		}
		return nil
	})
	if _, err := server.WriteFrom(codec.Binary, bytes.NewReader(fits)); err != nil {
		t.Fatal(err)
	}
	wait()

	// Compressed, through a real peer.
	server, client := compressionPair(t, true, true, 0)
	wait = run(t, func() error {
		for _, want := range [][]byte{small, large} {
			if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, want) {
				return fmt.Errorf("compressed: %d bytes, %v", len(p), err)
			}
		}
		return nil
	})
	for _, msg := range [][]byte{small, large} {
		if _, err := server.WriteFrom(codec.Binary, bytes.NewReader(msg)); err != nil {
			t.Fatal(err)
		}
	}
	wait()

	// A reader failure leaves the message open and reports only bytes sent.
	// Here the lookahead read fails before the first chunk goes out, so the
	// peer sees nothing but the ping.
	server, peer = raw(t, ws.Config{FragmentSize: 32 << 10})
	wait = run(t, func() error {
		if h, _ := readFrame(t, peer); h.Opcode() != codec.Ping {
			return fmt.Errorf("expected only a ping, got opcode %d", h.Opcode())
		}
		return nil
	})
	boom := errors.New("boom")
	n, err := server.WriteFrom(codec.Binary, &failingReader{bytes.NewReader(large[:50000]), boom})
	if err != boom || n != 0 {
		t.Fatal(n, err)
	}
	if err := server.Write(codec.Binary, nil); err != ws.ErrMessageOpen {
		t.Fatal("message not left open after reader failure")
	}
	if err := server.Ping(nil); err != nil {
		t.Fatal("control frames must still work")
	}
	wait()
}

func TestNextMessageDiscardsCompressed(t *testing.T) {
	server, client := compressionPair(t, true, true, 0)
	wait := run(t, func() error {
		if err := client.Write(codec.Binary, bytes.Repeat([]byte("a"), 10000)); err != nil {
			return err
		}
		return client.Write(codec.Text, []byte("second"))
	})
	if _, err := server.NextMessage(); err != nil {
		t.Fatal(err)
	}
	if n, err := server.Read(make([]byte, 1)); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	op, p, err := server.ReadMessage()
	if err != nil || op != codec.Text || string(p) != "second" {
		t.Fatalf("%d %q %v", op, p, err)
	}
	wait()
}

// TestDiscardKeepsReceiveHistory skips a compressed message without reading a
// byte of it. With receive context takeover the next message references the
// skipped one's content, so discard must inflate rather than skip.
// TestCompressedReadStreams shows a chunked read of a compressed message
// delivering the first fragment's bytes before the second fragment has been
// sent: over net.Pipe the client's second WriteChunk cannot even start until
// the server has read the first.
func TestCompressedReadStreams(t *testing.T) {
	server, client := compressionPair(t, true, true, 1)
	first := bytes.Repeat([]byte("first fragment "), 300)
	second := bytes.Repeat([]byte("second fragment "), 300)
	sentSecond := make(chan struct{})
	wait := run(t, func() error {
		if err := client.BeginMessage(codec.Text); err != nil {
			return err
		}
		if err := client.WriteChunk(first); err != nil {
			return err
		}
		<-sentSecond // The server has the first fragment's bytes by now.
		if err := client.WriteChunk(second); err != nil {
			return err
		}
		return client.EndMessage()
	})
	if _, err := server.NextMessage(); err != nil {
		t.Fatal(err)
	}
	var got []byte
	buf := make([]byte, 1000)
	for len(got) < len(first) {
		n, err := server.Read(buf)
		if err != nil {
			t.Fatalf("first fragment: %v after %d bytes", err, len(got))
		}
		got = append(got, buf[:n]...)
	}
	if !bytes.Equal(got[:len(first)], first) {
		t.Fatal("first fragment mismatch")
	}
	close(sentSecond)
	rest, err := io.ReadAll(server)
	if err != nil || !bytes.Equal(append(got[len(first):], rest...), second) {
		t.Fatalf("second fragment: %v", err)
	}
	wait()
}

func TestDiscardKeepsReceiveHistory(t *testing.T) {
	server, client := compressionPair(t, true, true, 1)
	client.ControlHandler = silentClose{}
	first := bytes.Repeat([]byte("history-bearing text that the second message repeats "), 200)
	second := first // Identical, so it compresses to back-references into the first.
	wait := run(t, func() error {
		if err := client.Write(codec.Text, first); err != nil {
			return err
		}
		if err := client.Write(codec.Text, second); err != nil {
			return err
		}
		_, p, err := client.ReadMessage() // Also lets a failing server deliver its close frame.
		if err != nil || string(p) != "done" {
			return fmt.Errorf("final message: %q %v", p, err)
		}
		return nil
	})
	if _, err := server.NextMessage(); err != nil { // Open the first, read nothing.
		t.Fatal(err)
	}
	op, p, err := server.ReadMessage() // Discards the first, reads the second.
	if err != nil || op != codec.Text || !bytes.Equal(p, second) {
		t.Fatalf("second message after discarding the first: %v (%d bytes)", err, len(p))
	}
	if err := server.Write(codec.Text, []byte("done")); err != nil {
		t.Fatal(err)
	}
	wait()
}

func BenchmarkCompressed(b *testing.B) {
	for _, size := range []int{4 << 10, 256 << 10} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			benchmarkCompressed(b, size)
		})
	}
}

// compressiblePayload is size bytes of records with random numbers, which
// deflate at level 1 to about half their size rather than to nothing.
func compressiblePayload(size int) []byte {
	r := rand.New(rand.NewPCG(1, 2))
	p := make([]byte, 0, size+64)
	for len(p) < size {
		p = fmt.Appendf(p, `{"id":%d,"value":%d,"text":"hello world"}`, r.Uint32(), r.Uint32())
	}
	return p[:size]
}

func benchmarkCompressed(b *testing.B, size int) {
	comp := &handshake.Compression{Level: flate.BestSpeed}
	payload := compressiblePayload(size)
	wire := func(role ws.Role) []byte {
		var buf bytes.Buffer
		c, err := ws.NewConn(struct {
			io.Reader
			io.Writer
		}{nil, &buf}, ws.Config{Role: role, Compression: comp})
		if err != nil {
			b.Fatal(err)
		}
		if err := c.Write(codec.Binary, payload); err != nil {
			b.Fatal(err)
		}
		return buf.Bytes()
	}
	b.Run("ReadMessage", func(b *testing.B) {
		c, _ := ws.NewConn(&replay{wire: wire(ws.Client)}, ws.Config{Role: ws.Server, Compression: comp})
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for b.Loop() {
			if _, p, err := c.ReadMessage(); err != nil || len(p) != len(payload) {
				b.Fatal(len(p), err)
			}
		}
	})
	b.Run("Read", func(b *testing.B) {
		c, _ := ws.NewConn(&replay{wire: wire(ws.Client)}, ws.Config{Role: ws.Server, Compression: comp})
		buf := make([]byte, 64<<10)
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for b.Loop() {
			if _, err := c.NextMessage(); err != nil {
				b.Fatal(err)
			}
			for {
				if _, err := c.Read(buf); err == io.EOF {
					break
				} else if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("Write", func(b *testing.B) {
		c, _ := ws.NewConn(discard{}, ws.Config{Role: ws.Server, Compression: comp})
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for b.Loop() {
			if err := c.Write(codec.Binary, payload); err != nil {
				b.Fatal(err)
			}
		}
	})
	// A pooled compressor per message, its output copied into the queue arena.
	b.Run("Send", func(b *testing.B) {
		c, _ := ws.NewConn(discard{}, ws.Config{Role: ws.Server, Compression: comp, CompressionShared: true})
		q := c.NewQueue(0)
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for b.Loop() {
			if err := q.Send(codec.Binary, payload); err != nil {
				b.Fatal(err)
			}
		}
		if err := q.Wait(); err != nil {
			b.Fatal(err)
		}
	})
}
