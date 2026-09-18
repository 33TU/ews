package ws_test

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/handshake"
	"github.com/33TU/ews/ws"
	"github.com/klauspost/compress/flate"
)

var errBoom = errors.New("boom")

// faultRW is a transport that serves scripted reads, then a read error, and
// allows a budget of writes before failing them.
type faultRW struct {
	mu      sync.Mutex
	reads   [][]byte
	readErr error
	writes  int // Writes still allowed.
	written [][]byte
}

func (f *faultRW) Read(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reads) == 0 {
		if f.readErr != nil {
			return 0, f.readErr
		}
		return 0, io.EOF
	}
	n := copy(b, f.reads[0])
	f.reads[0] = f.reads[0][n:]
	if len(f.reads[0]) == 0 {
		f.reads = f.reads[1:]
	}
	return n, nil
}

func (f *faultRW) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writes == 0 {
		return 0, errBoom
	}
	f.writes--
	f.written = append(f.written, bytes.Clone(b))
	return len(b), nil
}

// vecRW looks like a kernel socket to Conn, so writes take the writev path.
type vecRW struct{ *faultRW }

func (vecRW) SyscallConn() (syscall.RawConn, error) { return nil, errors.New("fake") }

// blockingRW holds its first write until released, so a queue accumulates
// frames behind it; later writes are recorded.
type blockingRW struct {
	faultRW
	release chan struct{}
	first   sync.Once
}

func (b *blockingRW) Write(p []byte) (int, error) {
	b.first.Do(func() { <-b.release })
	return b.faultRW.Write(p)
}

// vecBlockingRW is a blockingRW that looks like a kernel socket.
type vecBlockingRW struct{ *blockingRW }

func (vecBlockingRW) SyscallConn() (syscall.RawConn, error) { return nil, errors.New("fake") }

func newConn(t *testing.T, rw io.ReadWriter, cfg ws.Config) *ws.Conn {
	t.Helper()
	if cfg.Role != ws.Client {
		cfg.Role = ws.Server
	}
	c, err := ws.NewConn(rw, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestWriteErrors(t *testing.T) {
	large := bytes.Repeat([]byte("L"), 20<<10) // Above the coalescing limit: two writes.
	for _, tc := range []struct {
		name string
		rw   func(writes int) io.ReadWriter
	}{
		{"plain", func(n int) io.ReadWriter { return &faultRW{writes: n} }},
		{"vectored", func(n int) io.ReadWriter { return vecRW{&faultRW{writes: n}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConn(t, tc.rw(0), ws.Config{})
			if err := c.Write(codec.Binary, []byte("small")); err != errBoom {
				t.Fatalf("small write: %v", err)
			}
			if err := c.Write(codec.Binary, large); err != errBoom {
				t.Fatalf("large write: %v", err)
			}
			if err := c.Ping(nil); err != errBoom {
				t.Fatalf("header-only write: %v", err)
			}
			if err := c.Close(1000, ""); err != errBoom {
				t.Fatalf("close: %v", err)
			}
			// The second of two writes failing.
			c = newConn(t, tc.rw(1), ws.Config{})
			if err := c.Write(codec.Binary, large); err != errBoom {
				t.Fatalf("large write, second half: %v", err)
			}
			// Successful writes land whole in every mode.
			rw := &faultRW{writes: 10}
			c = newConn(t, tc.rw(10), ws.Config{})
			_ = rw
			if err := c.Write(codec.Binary, []byte("ok")); err != nil {
				t.Fatal(err)
			}
			if err := c.Write(codec.Binary, large); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQueueFailure(t *testing.T) {
	c := newConn(t, &faultRW{writes: 0}, ws.Config{})
	q := c.NewQueue(0)
	if err := q.Send(codec.Binary, []byte("queued")); err != nil {
		t.Fatal("first Send must accept: the failure happens on the writer")
	}
	if err := q.Wait(); err != errBoom {
		t.Fatalf("Wait: %v", err)
	}
	if err := q.Send(codec.Binary, []byte("again")); err != errBoom || q.Err() != errBoom {
		t.Fatalf("failed queue accepted a message: %v", err)
	}
	p, _ := ws.Prepare(codec.Binary, []byte("prepared"))
	if err := q.SendPrepared(p); err != errBoom {
		t.Fatalf("SendPrepared on a failed queue: %v", err)
	}
	if err := c.WritePrepared(p); err != errBoom {
		t.Fatalf("WritePrepared joining a failed queue: %v", err)
	}
	if err := c.Write(codec.Binary, []byte("sync")); err != errBoom {
		t.Fatalf("Write joining a failed queue: %v", err)
	}
	if q.Pending() != 0 {
		t.Fatal("failed queue holds bytes")
	}
}

func TestQueueCoalesces(t *testing.T) {
	for _, vectored := range []bool{false, true} {
		rw := &blockingRW{writes: 100, release: make(chan struct{})}
		var transport io.ReadWriter = rw
		if vectored {
			transport = vecBlockingRW{rw}
		}
		c := newConn(t, transport, ws.Config{})
		q := c.NewQueue(0)
		q.Send(codec.Binary, []byte("one")) // The writer takes this and blocks in Write.
		for q.Pending() != 0 {
		}
		q.Send(codec.Binary, []byte("two"))
		q.Send(codec.Text, []byte("three"))
		p, _ := ws.Prepare(codec.Binary, bytes.Repeat([]byte("p"), 200))
		q.SendPrepared(p)
		close(rw.release)
		if err := q.Wait(); err != nil {
			t.Fatal(err)
		}
		// Everything written decodes back to the four messages, in order.
		var d codec.Decoder
		rw.mu.Lock()
		for _, w := range rw.written {
			d.Feed(bytes.Clone(w))
			d.Preserve()
		}
		rw.mu.Unlock()
		var got []string
		for {
			h, ok, err := d.NextHeader()
			if err != nil || !ok {
				break
			}
			p, _ := d.Payload()
			got = append(got, string(p))
			_ = h
		}
		if len(got) != 4 || got[0] != "one" || got[1] != "two" || got[2] != "three" || len(got[3]) != 200 {
			t.Fatalf("vectored=%t: %q", vectored, got)
		}
	}
}

// shortReader returns n bytes and then err.
type shortReader struct {
	n   int
	err error
}

func (r *shortReader) Read(p []byte) (int, error) {
	if r.n == 0 {
		return 0, r.err
	}
	n := min(len(p), r.n)
	r.n -= n
	return n, nil
}

func TestFragmentErrors(t *testing.T) {
	rw := &faultRW{writes: 1}
	c := newConn(t, rw, ws.Config{})
	if err := c.BeginMessage(codec.Binary); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteChunk([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteChunk([]byte("second")); err != errBoom {
		t.Fatalf("failing chunk: %v", err)
	}
	// The failure ended the message: a plain write is no longer refused for it.
	if err := c.Write(codec.Binary, []byte("after")); err != errBoom {
		t.Fatalf("after failed fragment: %v", err)
	}

	// WriteFrom: a reader that fails at once, and one that fails mid-stream.
	c = newConn(t, &faultRW{writes: 100}, ws.Config{FragmentSize: 1024})
	if _, err := c.WriteFrom(codec.Binary, &shortReader{err: errBoom}); err != errBoom {
		t.Fatalf("failing reader: %v", err)
	}
	if n, err := c.WriteFrom(codec.Binary, &shortReader{n: 3000, err: errBoom}); err != errBoom || n == 0 {
		t.Fatalf("reader failing mid-stream: %d, %v", n, err)
	}
	// A reader failure leaves the fragmented message open, as documented: it
	// cannot be withdrawn, so the connection is for closing.
	if _, err := c.WriteFrom(codec.Binary, &shortReader{n: 3000, err: io.EOF}); err != ws.ErrMessageOpen {
		t.Fatalf("WriteFrom with a message left open: %v", err)
	}
	if err := c.Close(1000, ""); err != nil {
		t.Fatal(err)
	}
	c = newConn(t, &faultRW{writes: 100}, ws.Config{})
	if err := c.WriteChunk([]byte("x")); err != ws.ErrNoMessage {
		t.Fatalf("chunk without message: %v", err)
	}
	if err := c.Close(1000, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.WriteFrom(codec.Binary, &shortReader{n: 3000, err: io.EOF}); err != ws.ErrClosing {
		t.Fatalf("WriteFrom after Close: %v", err)
	}
}

// wire encodes messages as a client would send them, for scripted reads.
func wire(t *testing.T, comp *handshake.Compression, msgs ...[]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	c, err := ws.NewConn(struct {
		io.Reader
		io.Writer
	}{nil, &buf}, ws.Config{Role: ws.Client, Compression: comp})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if err := c.Write(codec.Binary, m); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

func TestReadErrors(t *testing.T) {
	msg := bytes.Repeat([]byte("m"), 3000)
	full := wire(t, nil, msg)
	for _, read := range []string{"ReadMessage", "Read", "WriteTo"} {
		// The transport fails halfway through the frame.
		rw := &faultRW{reads: [][]byte{full[:1000]}, readErr: errBoom, writes: 100}
		c := newConn(t, rw, ws.Config{ReadBufferSize: 256})
		var err error
		switch read {
		case "ReadMessage":
			_, _, err = c.ReadMessage()
		case "Read":
			if _, err = c.NextMessage(); err == nil {
				_, err = io.ReadAll(c)
			}
		case "WriteTo":
			if _, err = c.NextMessage(); err == nil {
				_, err = c.WriteTo(io.Discard)
			}
		}
		if err != errBoom {
			t.Fatalf("%s: %v", read, err)
		}
	}
	// EOF inside a frame is unexpected; EOF between frames is plain EOF.
	c := newConn(t, &faultRW{reads: [][]byte{full[:10]}}, ws.Config{})
	if _, _, err := c.ReadMessage(); err != io.ErrUnexpectedEOF {
		t.Fatalf("EOF mid-frame: %v", err)
	}
	c = newConn(t, &faultRW{reads: [][]byte{full}}, ws.Config{})
	if _, p, err := c.ReadMessage(); err != nil || len(p) != len(msg) {
		t.Fatal(err)
	}
	if _, _, err := c.ReadMessage(); err != io.EOF {
		t.Fatalf("EOF between frames: %v", err)
	}
}

func TestCompressedReadTransportError(t *testing.T) {
	comp := &handshake.Compression{Level: flate.BestSpeed, MinSize: 1}
	msg := bytes.Repeat([]byte("compressible "), 500)
	full := wire(t, comp, msg)
	// The transport dies inside the compressed message: the inflater cannot
	// resume, so the error is terminal for the connection.
	rw := &faultRW{reads: [][]byte{full[:len(full)/2]}, readErr: errBoom, writes: 100}
	c := newConn(t, rw, ws.Config{Compression: comp, ReadBufferSize: 64})
	if _, err := c.NextMessage(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(c); err != errBoom {
		t.Fatalf("mid-inflate transport error: %v", err)
	}
	if _, err := c.Read(make([]byte, 8)); err != errBoom {
		t.Fatalf("not sticky: %v", err)
	}
	if _, _, err := c.ReadMessage(); err != errBoom {
		t.Fatalf("not sticky for ReadMessage: %v", err)
	}
	// Discarding an unread compressed message under takeover inflates it; a
	// transport error during that discard surfaces from NextMessage.
	take := &handshake.Compression{Level: flate.BestSpeed, MinSize: 1, SendContextTakeover: true, ReceiveContextTakeover: true}
	full = wire(t, take, msg)
	rw = &faultRW{reads: [][]byte{full[:len(full)/2]}, readErr: errBoom, writes: 100}
	c = newConn(t, rw, ws.Config{Compression: take, ReadBufferSize: 64})
	if _, err := c.NextMessage(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NextMessage(); err != errBoom {
		t.Fatalf("discard through the inflater: %v", err)
	}
}

func TestErrorStrings(t *testing.T) {
	e := &ws.Error{Code: 1002, Err: ws.ErrProtocol}
	if e.Error() != ws.ErrProtocol.Error() || !errors.Is(e, ws.ErrProtocol) {
		t.Fatal(e)
	}
	if s := (&ws.CloseError{Code: 1001}).Error(); s != "ews/ws: peer closed with code 1001" {
		t.Fatal(s)
	}
	if s := (&ws.CloseError{Code: 1008, Reason: "policy"}).Error(); s != "ews/ws: peer closed with code 1008: policy" {
		t.Fatal(s)
	}
	if err := (ws.DefaultControlHandler{}).OnPong(nil, nil); err != nil {
		t.Fatal(err)
	}
}

// closeFails is a pipe end whose Close reports an error.
type closeFails struct{ net.Conn }

func (c closeFails) Close() error { c.Conn.Close(); return errBoom }

func TestNetConnEdges(t *testing.T) {
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("NetConn accepted a control opcode")
			}
		}()
		ws.NetConn(nil, codec.Ping)
	}()
	sc, cc := net.Pipe()
	defer cc.Close()
	server, _ := ws.NewConn(closeFails{sc}, ws.Config{Role: ws.Server})
	nc := ws.NetConn(server, codec.Binary)
	if err := nc.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	go io.Copy(io.Discard, cc) // Absorb the close frame.
	if err := nc.Close(); err != errBoom {
		t.Fatalf("transport close error not reported: %v", err)
	}

	// A protocol failure is terminal and sticky through NetConn.
	server, peer := raw(t, ws.Config{})
	nc = ws.NetConn(server, codec.Binary)
	wait := run(t, func() error {
		if _, err := peer.Write(frame(t, 0x83, []byte("reserved opcode"))); err != nil {
			return err
		}
		readFrame(t, peer) // The close frame.
		return nil
	})
	_, err := nc.Read(make([]byte, 8))
	if we, ok := errors.AsType[*ws.Error](err); !ok || we.Code != 1002 {
		t.Fatalf("protocol failure: %v", err)
	}
	_, err = nc.Read(make([]byte, 8))
	if _, ok := errors.AsType[*ws.Error](err); !ok {
		t.Fatalf("not sticky: %v", err)
	}
	wait()
}
