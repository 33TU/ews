package transport

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/33TU/ews/handshake"
)

// DialOptions configures Dial. The zero value dials with defaults.
type DialOptions struct {
	// Handshake is what to offer: compression and subprotocols.
	Handshake handshake.Options
	// Header adds request headers such as Origin, Cookie, or Authorization.
	// Handshake headers set by Dial take precedence.
	Header http.Header
	// TLSConfig applies to wss URLs. Nil uses the defaults with the URL's host
	// as the server name.
	TLSConfig *tls.Config
	// Dialer opens the TCP connection. Nil uses a zero net.Dialer.
	Dialer *net.Dialer
}

// HandshakeError reports a server response that did not complete the upgrade.
type HandshakeError struct {
	Status int
	Header http.Header
	Err    error
}

func (e *HandshakeError) Error() string {
	return "ews: handshake failed with status " + strconv.Itoa(e.Status) + ": " + e.Err.Error()
}

func (e *HandshakeError) Unwrap() error { return e.Err }

// Dial performs the client side of the opening handshake with a ws or wss URL
// and returns the connection with any deadlines cleared. Wrap it with
// ws.NewConn using ws.Client and the returned compression. ctx bounds the
// dial and the handshake; a bufferedConn is returned only when the server sent
// data right behind its response.
func Dial(ctx context.Context, rawURL string, opts DialOptions) (net.Conn, handshake.Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, handshake.Result{}, err
	}
	var useTLS bool
	switch u.Scheme {
	case "ws", "http":
		u.Scheme = "http"
	case "wss", "https":
		u.Scheme, useTLS = "https", true
	default:
		return nil, handshake.Result{}, errors.New("ews: unsupported URL scheme " + strconv.Quote(u.Scheme))
	}

	hreq, err := handshake.NewRequest(opts.Handshake)
	if err != nil {
		return nil, handshake.Result{}, err
	}

	addr := u.Host
	if _, _, err := net.SplitHostPort(addr); err != nil {
		port := "80"
		if useTLS {
			port = "443"
		}
		addr = net.JoinHostPort(u.Hostname(), port)
	}
	dialer := opts.Dialer
	if dialer == nil {
		dialer = new(net.Dialer)
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, handshake.Result{}, err
	}

	// Abort the handshake if ctx ends while it is in progress. The socket
	// deadline is set only from here, after ctx is done, so a failure caused
	// by ctx always reports ctx.Err rather than a bare timeout.
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	fail := func(err error) (net.Conn, handshake.Result, error) {
		conn.Close()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, handshake.Result{}, err
	}

	if useTLS {
		cfg := opts.TLSConfig
		if cfg == nil {
			cfg = new(tls.Config)
		}
		if cfg.ServerName == "" {
			cfg = cfg.Clone()
			cfg.ServerName = u.Hostname()
		}
		tc := tls.Client(conn, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return fail(err)
		}
		conn = tc
	}

	req, err := http.NewRequestWithContext(ctx, hreq.Method, u.String(), nil)
	if err != nil {
		return fail(err)
	}
	for k, v := range opts.Header {
		req.Header[http.CanonicalHeaderKey(k)] = v
	}
	req.Header.Set("Upgrade", hreq.Upgrade)
	req.Header.Set("Connection", hreq.Connection)
	req.Header.Set("Sec-WebSocket-Version", hreq.Version)
	req.Header.Set("Sec-WebSocket-Key", hreq.Key)
	req.Header.Del("Sec-WebSocket-Extensions")
	req.Header.Del("Sec-WebSocket-Protocol")
	if hreq.Extensions != "" {
		req.Header.Set("Sec-WebSocket-Extensions", hreq.Extensions)
	}
	if hreq.Protocols != "" {
		req.Header.Set("Sec-WebSocket-Protocol", hreq.Protocols)
	}

	if err := req.Write(conn); err != nil {
		return fail(err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return fail(err)
	}
	res, err := handshake.Confirm(hreq, handshake.Response{
		Status:     resp.StatusCode,
		Upgrade:    resp.Header.Get("Upgrade"),
		Connection: resp.Header.Get("Connection"),
		Accept:     resp.Header.Get("Sec-WebSocket-Accept"),
		Extensions: resp.Header.Get("Sec-WebSocket-Extensions"),
		Protocol:   resp.Header.Get("Sec-WebSocket-Protocol"),
	}, opts.Handshake)
	if err != nil {
		resp.Body.Close()
		return fail(&HandshakeError{Status: resp.StatusCode, Header: resp.Header, Err: err})
	}

	stop()
	conn.SetDeadline(time.Time{})
	if br.Buffered() != 0 {
		conn = &bufferedConn{Conn: conn, r: br}
	}
	return conn, res, nil
}
