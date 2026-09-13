package ws_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
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
		var ce *ws.CloseError
		if !errors.As(err, &ce) || ce.Code != 1000 || ce.Reason != "" {
			return fmt.Errorf("client read: %v", err)
		}
		return nil
	})
	_, _, err := server.ReadMessage()
	var ce *ws.CloseError
	if !errors.As(err, &ce) || ce.Code != 1000 || ce.Reason != "bye" {
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
	var ce *ws.CloseError
	if !errors.As(err, &ce) || ce.Code != ws.NoStatus {
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
		{"text utf8", frame(t, 0x81, []byte{255}), 1007},
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
			var we *ws.Error
			if !errors.As(err, &we) || we.Code != tt.code {
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
	server, peer := raw(t, ws.Config{})
	wire := append(frame(t, 0x01, []byte{0xe2, 0x82}), frame(t, 0x80, []byte{0x41})...) // Broken rune across frames.
	wait := run(t, func() error {
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
	var we *ws.Error
	if !errors.As(err, &we) || we.Code != 1007 || !errors.Is(err, ws.ErrInvalidUTF8) {
		t.Fatal(err)
	}
	wait()

	// Chunked reads deliver text unvalidated; only whole messages can be checked.
	server, peer = raw(t, ws.Config{})
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
		var ce *ws.CloseError
		if !errors.As(err, &ce) || ce.Code != 1009 {
			return fmt.Errorf("client expected close 1009, got %v", err)
		}
		return nil
	})
	_, _, err = server.ReadMessage()
	var we *ws.Error
	if !errors.As(err, &we) || we.Code != 1009 || !errors.Is(err, ws.ErrMessageTooLarge) {
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
	var ce *ws.CloseError
	if !errors.As(err, &ce) || ce.Code != 1001 || server.CloseSent() {
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
		for i := 0; i < writers*each; i++ {
			if _, _, err := server.ReadMessage(); err != nil {
				return err
			}
		}
		return nil
	})
	waitClient := run(t, func() error {
		for i := 0; i < writers*each; i++ {
			if _, _, err := client.ReadMessage(); err != nil {
				return err
			}
		}
		return nil
	})
	var waits []func()
	for w := 0; w < writers; w++ {
		payload := bytes.Repeat([]byte{byte(w)}, 100+w)
		waits = append(waits, run(t, func() error {
			for i := 0; i < each; i++ {
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
		{CompressionIdle: -1},
	} {
		if _, err := ws.NewConn(sc, cfg); err != ws.ErrInvalidConfig {
			t.Fatalf("%+v accepted", cfg)
		}
	}
	if _, err := ws.NewConn(nil, ws.Config{}); err != ws.ErrInvalidConfig {
		t.Fatal("nil transport accepted")
	}
}

func TestReset(t *testing.T) {
	server, client := pair(t, ws.Config{}, ws.Config{})
	wait := run(t, func() error {
		if err := client.Close(1000, ""); err != nil {
			return err
		}
		_, _, err := client.ReadMessage()
		var ce *ws.CloseError
		if !errors.As(err, &ce) {
			return err
		}
		return nil
	})
	if _, _, err := server.ReadMessage(); err == nil {
		t.Fatal("expected close")
	}
	wait()
	sc, cc := net.Pipe()
	defer sc.Close()
	defer cc.Close()
	if err := server.Reset(sc, ws.Config{Role: ws.Server}); err != nil {
		t.Fatal(err)
	}
	client2, err := ws.NewConn(cc, ws.Config{Role: ws.Client})
	if err != nil {
		t.Fatal(err)
	}
	wait = run(t, func() error {
		if err := client2.Write(codec.Text, []byte("again")); err != nil {
			return err
		}
		_, p, err := client2.ReadMessage()
		if err != nil || len(p) != 0 {
			return fmt.Errorf("client2 read %q, %v", p, err)
		}
		return nil
	})
	if _, p, err := server.ReadMessage(); err != nil || string(p) != "again" {
		t.Fatal(p, err)
	}
	if err := server.Write(codec.Text, nil); err != nil {
		t.Fatal("Reset did not clear close state:", err)
	}
	wait()
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
	sc := &handshake.Compression{Level: flate.BestSpeed, MinSize: minSize, SendContextTakeover: serverTakeover, ReceiveContextTakeover: clientTakeover}
	cc := &handshake.Compression{Level: flate.BestSpeed, MinSize: minSize, SendContextTakeover: clientTakeover, ReceiveContextTakeover: serverTakeover}
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
							for round := 0; round < 2; round++ { // Takeover history spans messages.
								for _, m := range msgs {
									if err := src.Write(m.op, m.payload); err != nil {
										return err
									}
								}
							}
							return nil
						})
						buf := make([]byte, bufSize)
						for round := 0; round < 2; round++ {
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
	for _, streaming := range []bool{false, true} {
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
		if streaming {
			if _, err = server.NextMessage(); err == nil {
				_, err = server.Read(make([]byte, 16))
			}
		} else {
			_, _, err = server.ReadMessage()
		}
		var we *ws.Error
		if !errors.As(err, &we) || we.Code != 1007 || !errors.Is(err, ws.ErrInvalidData) {
			t.Fatalf("streaming=%t: %v", streaming, err)
		}
		wait()
	}

	// Compressed wire fits the budget but the message does not.
	server, client := pair(t, ws.Config{Compression: comp, MaxMessageSize: 100}, ws.Config{Compression: comp, ControlHandler: silentClose{}})
	big := bytes.Repeat([]byte("a"), 1000)
	go client.Write(codec.Binary, big) // Read by the server before failing.
	wait := run(t, func() error {
		_, _, err := client.ReadMessage()
		var ce *ws.CloseError
		if !errors.As(err, &ce) || ce.Code != 1009 {
			return fmt.Errorf("client expected close 1009, got %v", err)
		}
		return nil
	})
	_, _, err := server.ReadMessage()
	var we *ws.Error
	if !errors.As(err, &we) || we.Code != 1009 {
		t.Fatal(err)
	}
	wait()
}

// TestCompressionIdle releases the attached compressor after idle time and
// re-primes it from the window on the next write.
func TestCompressionIdle(t *testing.T) {
	sc := &handshake.Compression{Level: flate.BestSpeed, SendContextTakeover: true}
	cc := &handshake.Compression{Level: flate.BestSpeed, ReceiveContextTakeover: true}
	server, client := pair(t, ws.Config{Compression: sc, CompressionIdle: 20 * time.Millisecond}, ws.Config{Compression: cc})
	payload := bytes.Repeat([]byte("idle release keeps the window "), 300)
	wait := run(t, func() error {
		for i := 0; i < 4; i++ {
			if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, payload) {
				return fmt.Errorf("message %d: %v", i, err)
			}
		}
		return nil
	})
	for i := 0; i < 4; i++ {
		if err := server.Write(codec.Text, payload); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			time.Sleep(80 * time.Millisecond) // Past the idle release.
		}
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
		for i := 0; i < 5; i++ {
			if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, payload) {
				return fmt.Errorf("message %d: %v", i, err)
			}
		}
		return nil
	})
	for i := 0; i < 5; i++ {
		if err := server.Write(codec.Text, payload); err != nil {
			t.Fatal(err)
		}
	}
	wait()
}

// TestSmallWindow sends through reduced windows in both modes; the peer
// decodes with its full window.
func TestSmallWindow(t *testing.T) {
	for _, shared := range []bool{false, true} {
		sc := &handshake.Compression{Level: flate.BestSpeed, SendContextTakeover: true, SendWindowBits: 9}
		cc := &handshake.Compression{Level: flate.BestSpeed, ReceiveContextTakeover: true}
		server, client := pair(t, ws.Config{Compression: sc, CompressionShared: shared}, ws.Config{Compression: cc})
		payload := bytes.Repeat([]byte("a 512 byte window still compresses repeats "), 400)
		wait := run(t, func() error {
			for i := 0; i < 3; i++ {
				if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, payload) {
					return fmt.Errorf("message %d: %v", i, err)
				}
			}
			return nil
		})
		for i := 0; i < 3; i++ {
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
		for i := 0; i < 3; i++ {
			if _, p, err := client.ReadMessage(); err != nil || !bytes.Equal(p, whole) {
				return fmt.Errorf("message %d: %v", i, err)
			}
		}
		return nil
	})
	for i := 0; i < 3; i++ {
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

func BenchmarkCompressed(b *testing.B) {
	comp := &handshake.Compression{Level: flate.BestSpeed}
	payload := bytes.Repeat([]byte("compressible payload "), 4096/21)
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
}
