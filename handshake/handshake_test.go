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
