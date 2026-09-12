package handshake

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"strings"
)

var (
	ErrNotWebSocket   = errors.New("ews/handshake: not a WebSocket upgrade")
	ErrBadVersion     = errors.New("ews/handshake: unsupported WebSocket version")
	ErrBadKey         = errors.New("ews/handshake: invalid Sec-WebSocket-Key")
	ErrBadExtension   = errors.New("ews/handshake: invalid Sec-WebSocket-Extensions")
	ErrBadAccept      = errors.New("ews/handshake: Sec-WebSocket-Accept mismatch")
	ErrBadStatus      = errors.New("ews/handshake: unexpected response status")
	ErrBadProtocol    = errors.New("ews/handshake: unexpected subprotocol")
	ErrInvalidOptions = errors.New("ews/handshake: invalid options")
)

// Version is the only WebSocket protocol version.
const Version = "13"

// Options configures what an endpoint offers or accepts.
type Options struct {
	// Compression offers or accepts permessage-deflate. Nil disables it.
	Compression *Compress
	// Protocols lists supported subprotocols in preference order.
	Protocols []string
}

// Compress configures permessage-deflate with the default 32 KB window.
type Compress struct {
	// Level is the flate level for outgoing messages.
	Level int
	// MinSize leaves smaller payloads uncompressed.
	MinSize int
	// ContextTakeover allows compression history to carry between messages
	// when the peer agrees. Disabled, both directions run without takeover,
	// which needs no per-connection compressor state.
	ContextTakeover bool
}

// Compression is the negotiated permessage-deflate configuration for one
// connection. Directions are named from the local endpoint's point of view.
type Compression struct {
	Level                  int
	MinSize                int
	SendContextTakeover    bool
	ReceiveContextTakeover bool
}

// Result is what a completed handshake agreed on.
type Result struct {
	// Compression is nil when permessage-deflate was not negotiated.
	Compression *Compression
	// Protocol is the selected subprotocol, or empty.
	Protocol string
}

// Request holds the handshake fields of an HTTP request. Header fields with
// several values are joined with commas.
type Request struct {
	Method     string
	Upgrade    string
	Connection string
	Version    string
	Key        string
	Extensions string
	Protocols  string
}

// Response holds the handshake fields of an HTTP response.
type Response struct {
	Status     int
	Upgrade    string
	Connection string
	Accept     string
	Extensions string
	Protocol   string
}

// Accept computes Sec-WebSocket-Accept for a Sec-WebSocket-Key.
func Accept(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// NewKey returns a fresh Sec-WebSocket-Key.
func NewKey() string {
	var b [16]byte
	rand.Read(b[:])
	return base64.StdEncoding.EncodeToString(b[:])
}

// StatusCode returns the HTTP status a server should answer a rejected
// request with: 426 for a version mismatch, otherwise 400.
func StatusCode(err error) int {
	if errors.Is(err, ErrBadVersion) {
		return 426
	}
	return 400
}

func validKey(key string) bool {
	b, err := base64.StdEncoding.DecodeString(key)
	return err == nil && len(b) == 16
}

// hasToken reports whether a comma-separated header value contains token.
func hasToken(header, token string) bool {
	for _, t := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(t), token) {
			return true
		}
	}
	return false
}

func validOptions(opts Options) error {
	if c := opts.Compression; c != nil && (c.Level < -2 || c.Level > 9 || c.MinSize < 0) {
		return ErrInvalidOptions
	}
	return nil
}
