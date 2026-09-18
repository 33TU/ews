package ws_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
)

func TestServe(t *testing.T) {
	server, client := pair(t, ws.Config{}, ws.Config{})
	wait := run(t, func() error {
		// Over net.Pipe each echo must be read before the next write.
		for _, m := range messages() {
			if err := client.Write(m.op, m.payload); err != nil {
				return err
			}
			op, p, err := client.ReadMessage()
			if err != nil || op != m.op || !bytes.Equal(p, m.payload) {
				return errors.New("echo mismatch")
			}
		}
		if err := client.Close(1000, "done"); err != nil {
			return err
		}
		_, _, err := client.ReadMessage() // The server's close echo, or its write blocks.
		if _, ok := errors.AsType[*ws.CloseError](err); !ok {
			return err
		}
		return nil
	})
	var seen int
	err := ws.Serve(server, ws.MessageFunc(func(c *ws.Conn, op codec.Opcode, payload []byte) error {
		seen++
		return c.Write(op, payload)
	}))
	if ce, ok := errors.AsType[*ws.CloseError](err); !ok || ce.Code != 1000 || ce.Reason != "done" || seen != len(messages()) {
		t.Fatalf("Serve returned %v after %d messages", err, seen)
	}
	wait()

	// A handler error stops the loop and is returned as is.
	server, client = pair(t, ws.Config{}, ws.Config{})
	wait = run(t, func() error { return client.Write(codec.Text, []byte("stop")) })
	stop := errors.New("stop")
	if err := ws.Serve(server, ws.MessageFunc(func(*ws.Conn, codec.Opcode, []byte) error { return stop })); err != stop {
		t.Fatalf("handler error: %v", err)
	}
	wait()
}
