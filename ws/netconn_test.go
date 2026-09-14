package ws_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
)

func TestNetConn(t *testing.T) {
	server, client := pair(t, ws.Config{ReadBufferSize: 64}, ws.Config{})
	ns, nc := ws.NetConn(server, codec.Binary), ws.NetConn(client, codec.Binary)
	if _, ok := ns.(interface{ Unwrap() }); ok {
		t.Fatal("unexpected")
	}
	if ns.LocalAddr() == nil || ns.RemoteAddr().Network() != "pipe" {
		t.Fatalf("addresses not the transport's: %v", ns.RemoteAddr())
	}

	// Bytes tunnel in order across message boundaries, in both directions.
	chunks := [][]byte{[]byte("ab"), bytes.Repeat([]byte("c"), 5000), []byte("d"), bytes.Repeat([]byte("e"), 100000)}
	var want []byte
	for _, c := range chunks {
		want = append(want, c...)
	}
	wait := run(t, func() error {
		for _, c := range chunks {
			if n, err := nc.Write(c); err != nil || n != len(c) {
				return err
			}
		}
		if n, err := nc.Write(nil); n != 0 || err != nil {
			return errors.New("empty write must be a no-op")
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(nc, buf); err != nil || string(buf) != "reply" {
			return err
		}
		return nil
	})
	// The first read stops at the first message's end.
	buf := make([]byte, len(want))
	n, err := ns.Read(buf)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if _, err := io.ReadFull(ns, buf[2:]); err != nil || !bytes.Equal(buf, want) {
		t.Fatal(err)
	}
	if _, err := ns.Write([]byte("reply")); err != nil {
		t.Fatal(err)
	}
	wait()

	// A read deadline interrupts and leaves the connection usable.
	if err := ns.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var ne net.Error
	if _, err := ns.Read(buf); !errors.As(err, &ne) || !ne.Timeout() || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	if err := ns.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	wait = run(t, func() error { _, err := nc.Write([]byte("after")); return err })
	if n, err := ns.Read(buf); err != nil || string(buf[:n]) != "after" {
		t.Fatal(n, err)
	}
	wait()

	// Close sends a normal close, which the peer reads as io.EOF, and closes
	// the transport. The peer reads concurrently: over net.Pipe the close
	// frame is delivered only when it does.
	wait = run(t, func() error {
		if _, err := nc.Read(buf); err != io.EOF {
			return fmt.Errorf("peer close not io.EOF: %v", err)
		}
		if _, err := nc.Read(buf); err != io.EOF {
			return fmt.Errorf("io.EOF not sticky: %v", err)
		}
		return nil
	})
	if err := ns.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ns.Close(); err != nil {
		t.Fatal("second Close:", err)
	}
	wait()
	if _, err := ns.Write([]byte("x")); err == nil {
		t.Fatal("write after Close succeeded")
	}
}

func TestNetConnUnexpectedType(t *testing.T) {
	// The client does not echo the close: the server has stopped reading.
	server, client := pair(t, ws.Config{}, ws.Config{ControlHandler: silentClose{}})
	ns := ws.NetConn(server, codec.Binary)
	wait := run(t, func() error {
		if err := client.Write(codec.Text, []byte("text")); err != nil {
			return err
		}
		_, _, err := client.ReadMessage()
		var ce *ws.CloseError
		if !errors.As(err, &ce) || ce.Code != 1003 {
			return errors.New("peer did not see close 1003")
		}
		return nil
	})
	buf := make([]byte, 16)
	if _, err := ns.Read(buf); err != ws.ErrUnexpectedType {
		t.Fatalf("got %v", err)
	}
	if _, err := ns.Read(buf); err != ws.ErrUnexpectedType {
		t.Fatalf("error not sticky: %v", err)
	}
	wait()
}

func TestNetConnAbnormalClose(t *testing.T) {
	server, client := pair(t, ws.Config{}, ws.Config{})
	ns := ws.NetConn(server, codec.Text)
	wait := run(t, func() error {
		if err := client.Close(1011, "boom"); err != nil {
			return err
		}
		_, _, err := client.ReadMessage() // The echoed close.
		var ce *ws.CloseError
		if !errors.As(err, &ce) || ce.Code != 1011 {
			return err
		}
		return nil
	})
	var ce *ws.CloseError
	if _, err := ns.Read(make([]byte, 8)); !errors.As(err, &ce) || ce.Code != 1011 || ce.Reason != "boom" {
		t.Fatalf("got %v", err)
	}
	wait()
}

// TestNetConnNonNetTransport covers a transport that is only an io.ReadWriter.
func TestNetConnNonNetTransport(t *testing.T) {
	type rw struct{ io.ReadWriter }
	c, err := ws.NewConn(rw{&bytes.Buffer{}}, ws.Config{Role: ws.Server})
	if err != nil {
		t.Fatal(err)
	}
	nc := ws.NetConn(c, codec.Binary)
	if nc.RemoteAddr().Network() != "websocket" || nc.LocalAddr().String() != "websocket/unknown-addr" {
		t.Fatal(nc.RemoteAddr(), nc.LocalAddr())
	}
	if err := nc.SetDeadline(time.Now()); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := nc.Close(); err != nil {
		t.Fatal(err)
	}
}
