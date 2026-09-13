package handshake_test

import (
	"testing"

	"github.com/33TU/ews/handshake"
)

func request() handshake.Request {
	return handshake.Request{
		Method: "GET", Upgrade: "websocket", Connection: "keep-alive, Upgrade",
		Version: "13", Key: "dGhlIHNhbXBsZSBub25jZQ==",
	}
}

func TestNegotiate(t *testing.T) {
	resp, res, err := handshake.Negotiate(request(), handshake.Options{})
	if err != nil || resp.Status != 101 || resp.Accept != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" ||
		resp.Upgrade != "websocket" || resp.Connection != "Upgrade" || resp.Extensions != "" || res.Compression != nil {
		t.Fatalf("%+v %+v %v", resp, res, err)
	}
}

func TestNegotiateRejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(*handshake.Request)
		err  error
	}{
		{"method", func(r *handshake.Request) { r.Method = "POST" }, handshake.ErrNotWebSocket},
		{"upgrade", func(r *handshake.Request) { r.Upgrade = "h2c" }, handshake.ErrNotWebSocket},
		{"connection", func(r *handshake.Request) { r.Connection = "keep-alive" }, handshake.ErrNotWebSocket},
		{"version", func(r *handshake.Request) { r.Version = "8" }, handshake.ErrBadVersion},
		{"key length", func(r *handshake.Request) { r.Key = "c2hvcnQ=" }, handshake.ErrBadKey},
		{"key encoding", func(r *handshake.Request) { r.Key = "not base64!!" }, handshake.ErrBadKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := request()
			tt.edit(&req)
			if _, _, err := handshake.Negotiate(req, handshake.Options{}); err != tt.err {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
		})
	}
	if _, _, err := handshake.Negotiate(request(), handshake.Options{Compression: &handshake.Compress{Level: 10}}); err != handshake.ErrInvalidOptions {
		t.Fatal("bad level accepted")
	}
}

func TestNegotiateDeflate(t *testing.T) {
	tests := []struct {
		name     string
		offer    string
		takeover bool
		want     string // Expected response header; empty means no compression.
		send     bool
		recv     bool
		bits     int
	}{
		{"plain, no takeover", "permessage-deflate", false, "permessage-deflate; server_no_context_takeover; client_no_context_takeover", false, false, 0},
		{"plain, takeover", "permessage-deflate", true, "permessage-deflate", true, true, 0},
		{"client asks server no takeover", "permessage-deflate; server_no_context_takeover", true, "permessage-deflate; server_no_context_takeover", false, true, 0},
		{"client declares no takeover", "permessage-deflate; client_no_context_takeover", true, "permessage-deflate; client_no_context_takeover", true, false, 0},
		{"window bits accepted", "permessage-deflate; client_max_window_bits; server_max_window_bits=15", true, "permessage-deflate", true, true, 15},
		{"client window value", `permessage-deflate; client_max_window_bits="10"`, true, "permessage-deflate", true, true, 0},
		{"small server window honored", "permessage-deflate; server_max_window_bits=10", true, "permessage-deflate; server_max_window_bits=10", true, true, 10},
		{"small window with no takeover", "permessage-deflate; server_max_window_bits=8; server_no_context_takeover", true, "permessage-deflate; server_no_context_takeover; server_max_window_bits=8", false, true, 8},
		{"fallback offer", "permessage-deflate; foo, permessage-deflate; client_no_context_takeover", true, "permessage-deflate; client_no_context_takeover", true, false, 0},
		{"bad window bits skipped", "permessage-deflate; server_max_window_bits=7", true, "", false, false, 0},
		{"unknown param skipped", "permessage-deflate; foo=bar", true, "", false, false, 0},
		{"unknown extension ignored", "x-webkit-deflate-frame, permessage-deflate", true, "permessage-deflate", true, true, 0},
		{"duplicate param skipped", "permessage-deflate; client_no_context_takeover; client_no_context_takeover", true, "", false, false, 0},
		{"valued flag skipped", "permessage-deflate; server_no_context_takeover=1", true, "", false, false, 0},
		{"no offer", "", true, "", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := request()
			req.Extensions = tt.offer
			opts := handshake.Options{Compression: &handshake.Compress{Level: 6, MinSize: 64, ContextTakeover: tt.takeover}}
			resp, res, err := handshake.Negotiate(req, opts)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Extensions != tt.want {
				t.Fatalf("response %q, want %q", resp.Extensions, tt.want)
			}
			if tt.want == "" {
				if res.Compression != nil {
					t.Fatal("unexpected compression")
				}
				return
			}
			c := res.Compression
			if c == nil || c.Level != 6 || c.MinSize != 64 || c.SendContextTakeover != tt.send || c.ReceiveContextTakeover != tt.recv || c.SendWindowBits != tt.bits {
				t.Fatalf("compression %+v", c)
			}
		})
	}

	req := request()
	req.Extensions = "permessage-deflate"
	if resp, res, err := handshake.Negotiate(req, handshake.Options{}); err != nil || resp.Extensions != "" || res.Compression != nil {
		t.Fatal("compression negotiated without being enabled")
	}
	req.Extensions = "; broken"
	if _, _, err := handshake.Negotiate(req, handshake.Options{Compression: &handshake.Compress{}}); err != handshake.ErrBadExtension {
		t.Fatalf("malformed header: %v", err)
	}
}

func TestNegotiateProtocol(t *testing.T) {
	req := request()
	req.Protocols = "chat, superchat"
	resp, res, err := handshake.Negotiate(req, handshake.Options{Protocols: []string{"superchat", "chat"}})
	if err != nil || res.Protocol != "superchat" || resp.Protocol != "superchat" {
		t.Fatalf("%+v %v", res, err)
	}
	resp, res, err = handshake.Negotiate(req, handshake.Options{Protocols: []string{"other"}})
	if err != nil || res.Protocol != "" || resp.Protocol != "" {
		t.Fatalf("%+v %v", res, err)
	}
}
