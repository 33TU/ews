package handshake_test

import (
	"encoding/base64"
	"testing"

	"github.com/33TU/ews/handshake"
)

func TestAccept(t *testing.T) {
	// RFC 6455 section 1.3.
	if got := handshake.Accept("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("Accept = %q", got)
	}
}

func TestNewKey(t *testing.T) {
	k1, k2 := handshake.NewKey(), handshake.NewKey()
	if b, err := base64.StdEncoding.DecodeString(k1); err != nil || len(b) != 16 || k1 == k2 {
		t.Fatalf("keys %q %q", k1, k2)
	}
}

func TestStatusCode(t *testing.T) {
	if handshake.StatusCode(handshake.ErrBadVersion) != 426 || handshake.StatusCode(handshake.ErrBadKey) != 400 {
		t.Fatal("status mapping")
	}
}

func TestExtensionParseErrors(t *testing.T) {
	req := request()
	for _, ext := range []string{"; permessage-deflate", "permessage-deflate; =3", "permessage-deflate; client_no_context_takeover=1"} {
		req.Extensions = ext
		resp, res, err := handshake.Negotiate(req, handshake.Options{Compression: &handshake.Compress{Level: 1}})
		if err != nil && ext != "; permessage-deflate" && ext != "permessage-deflate; =3" {
			t.Fatalf("%q: %v", ext, err)
		}
		if err == nil && res.Compression != nil {
			t.Fatalf("%q: bad offer negotiated: %+v", ext, resp)
		}
	}
	// The client side rejects a malformed extensions header outright.
	creq, err := handshake.NewRequest(handshake.Options{Compression: &handshake.Compress{Level: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handshake.Confirm(creq, handshake.Response{Status: 101, Upgrade: "websocket", Connection: "Upgrade", Accept: handshake.Accept(creq.Key), Extensions: "; bogus"}, handshake.Options{Compression: &handshake.Compress{Level: 1}}); err == nil {
		t.Fatal("malformed extensions accepted by Confirm")
	}
}
