package handshake_test

import (
	"testing"

	"github.com/33TU/ews/handshake"
)

func TestNewRequest(t *testing.T) {
	req, err := handshake.NewRequest(handshake.Options{Protocols: []string{"a", "b"}})
	if err != nil || req.Method != "GET" || req.Version != "13" || req.Extensions != "" || req.Protocols != "a, b" || req.Key == "" {
		t.Fatalf("%+v %v", req, err)
	}
	req, _ = handshake.NewRequest(handshake.Options{Compression: &handshake.Compress{}})
	if req.Extensions != "permessage-deflate; client_max_window_bits; client_no_context_takeover; server_no_context_takeover" {
		t.Fatalf("offer %q", req.Extensions)
	}
	req, _ = handshake.NewRequest(handshake.Options{Compression: &handshake.Compress{ContextTakeover: true}})
	if req.Extensions != "permessage-deflate; client_max_window_bits" {
		t.Fatalf("offer %q", req.Extensions)
	}
	if _, err := handshake.NewRequest(handshake.Options{Compression: &handshake.Compress{MinSize: -1}}); err != handshake.ErrInvalidOptions {
		t.Fatal("bad options accepted")
	}
}

// exchange runs a full client and server negotiation in memory.
func exchange(t *testing.T, client, server handshake.Options) (handshake.Result, handshake.Result) {
	t.Helper()
	req, err := handshake.NewRequest(client)
	if err != nil {
		t.Fatal(err)
	}
	resp, serverRes, err := handshake.Negotiate(req, server)
	if err != nil {
		t.Fatal(err)
	}
	clientRes, err := handshake.Confirm(req, resp, client)
	if err != nil {
		t.Fatal(err)
	}
	return clientRes, serverRes
}

func TestRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		name           string
		client, server bool // Context takeover preference.
		wantClientSend bool
		wantServerSend bool
	}{
		{"neither", false, false, false, false},
		{"client only", true, false, false, false},
		{"server only", false, true, false, false},
		{"both", true, true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, s := exchange(t,
				handshake.Options{Compression: &handshake.Compress{Level: 1, ContextTakeover: tt.client}, Protocols: []string{"chat"}},
				handshake.Options{Compression: &handshake.Compress{Level: 9, ContextTakeover: tt.server}, Protocols: []string{"chat"}})
			if c.Compression == nil || s.Compression == nil || c.Protocol != "chat" || s.Protocol != "chat" {
				t.Fatalf("client %+v server %+v", c, s)
			}
			// Each side's send direction is the other's receive direction.
			if c.Compression.SendContextTakeover != tt.wantClientSend || s.Compression.ReceiveContextTakeover != tt.wantClientSend ||
				s.Compression.SendContextTakeover != tt.wantServerSend || c.Compression.ReceiveContextTakeover != tt.wantServerSend {
				t.Fatalf("client %+v server %+v", c.Compression, s.Compression)
			}
			if c.Compression.Level != 1 || s.Compression.Level != 9 {
				t.Fatal("levels are local settings")
			}
		})
	}
	c, s := exchange(t, handshake.Options{Compression: &handshake.Compress{}}, handshake.Options{})
	if c.Compression != nil || s.Compression != nil {
		t.Fatal("server without compression must not negotiate it")
	}
}

func TestConfirmRejects(t *testing.T) {
	opts := handshake.Options{Compression: &handshake.Compress{ContextTakeover: true}, Protocols: []string{"chat"}}
	req, _ := handshake.NewRequest(opts)
	good := handshake.Response{Status: 101, Upgrade: "websocket", Connection: "Upgrade", Accept: handshake.Accept(req.Key)}
	tests := []struct {
		name string
		edit func(*handshake.Response)
		opts handshake.Options
		err  error
	}{
		{"status", func(r *handshake.Response) { r.Status = 200 }, opts, handshake.ErrBadStatus},
		{"upgrade", func(r *handshake.Response) { r.Upgrade = "" }, opts, handshake.ErrNotWebSocket},
		{"accept", func(r *handshake.Response) { r.Accept = "AAAA" }, opts, handshake.ErrBadAccept},
		{"protocol", func(r *handshake.Response) { r.Protocol = "other" }, opts, handshake.ErrBadProtocol},
		{"unrequested extension", func(r *handshake.Response) { r.Extensions = "permessage-deflate" }, handshake.Options{}, handshake.ErrBadExtension},
		{"unknown extension", func(r *handshake.Response) { r.Extensions = "x-foo" }, opts, handshake.ErrBadExtension},
		{"two extensions", func(r *handshake.Response) { r.Extensions = "permessage-deflate, permessage-deflate" }, opts, handshake.ErrBadExtension},
		{"client window out of range", func(r *handshake.Response) { r.Extensions = "permessage-deflate; client_max_window_bits=16" }, opts, handshake.ErrBadExtension},
		{"unknown param", func(r *handshake.Response) { r.Extensions = "permessage-deflate; foo" }, opts, handshake.ErrBadExtension},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := good
			tt.edit(&resp)
			if _, err := handshake.Confirm(req, resp, tt.opts); err != tt.err {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
		})
	}
	resp := good
	resp.Extensions = "permessage-deflate; client_max_window_bits=15; client_max_window_bits"
	if _, err := handshake.Confirm(req, resp, opts); err != handshake.ErrBadExtension {
		t.Fatal("duplicate parameter accepted")
	}
	resp.Extensions = "permessage-deflate; server_max_window_bits=12; client_max_window_bits=12" // gws's default response.
	res, err := handshake.Confirm(req, resp, opts)
	if err != nil || res.Compression.SendWindowBits != 12 {
		t.Fatalf("server-chosen client window: %+v %v", res, err)
	}
	resp.Extensions = "permessage-deflate; server_max_window_bits=12; server_no_context_takeover"
	res, err = handshake.Confirm(req, resp, opts)
	if err != nil || res.Compression == nil || res.Compression.ReceiveContextTakeover || !res.Compression.SendContextTakeover || res.Compression.SendWindowBits != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}
