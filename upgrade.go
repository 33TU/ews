package ews

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/33TU/ews/handshake"
)

// ErrNotHijackable means the ResponseWriter cannot hand over its connection,
// for example under HTTP/2.
var ErrNotHijackable = errors.New("ews: response writer does not support hijacking")

// Upgrade performs the server side of the opening handshake and returns the
// hijacked connection with any deadlines cleared. Wrap it with ws.NewConn
// using ws.Server and the returned compression. On a rejected request an
// HTTP error has been written and the error explains why. Headers set on w
// before the call, such as cookies, are sent with the 101 response.
// Origin checks are the caller's business before calling Upgrade.
func Upgrade(w http.ResponseWriter, r *http.Request, opts handshake.Options) (net.Conn, handshake.Result, error) {
	req := handshake.Request{
		Method:     r.Method,
		Upgrade:    r.Header.Get("Upgrade"),
		Connection: strings.Join(r.Header.Values("Connection"), ", "),
		Version:    r.Header.Get("Sec-WebSocket-Version"),
		Key:        r.Header.Get("Sec-WebSocket-Key"),
		Extensions: strings.Join(r.Header.Values("Sec-WebSocket-Extensions"), ", "),
		Protocols:  strings.Join(r.Header.Values("Sec-WebSocket-Protocol"), ", "),
	}
	resp, res, err := handshake.Negotiate(req, opts)
	if err != nil {
		status := handshake.StatusCode(err)
		if status == http.StatusUpgradeRequired {
			w.Header().Set("Sec-WebSocket-Version", handshake.Version)
		}
		http.Error(w, err.Error(), status)
		return nil, handshake.Result{}, err
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, ErrNotHijackable.Error(), http.StatusInternalServerError)
		return nil, handshake.Result{}, ErrNotHijackable
	}
	extra := w.Header().Clone()
	conn, rw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, handshake.Result{}, err
	}
	conn.SetDeadline(time.Time{})

	var b []byte
	b = append(b, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "...)
	b = append(b, resp.Accept...)
	b = append(b, "\r\n"...)
	if resp.Extensions != "" {
		b = append(b, "Sec-WebSocket-Extensions: "...)
		b = append(b, resp.Extensions...)
		b = append(b, "\r\n"...)
	}
	if resp.Protocol != "" {
		b = append(b, "Sec-WebSocket-Protocol: "...)
		b = append(b, resp.Protocol...)
		b = append(b, "\r\n"...)
	}
	for name, values := range extra {
		if reserved(name) {
			continue
		}
		for _, v := range values {
			b = append(b, name...)
			b = append(b, ": "...)
			b = append(b, v...)
			b = append(b, "\r\n"...)
		}
	}
	b = append(b, "\r\n"...)
	if _, err := conn.Write(b); err != nil {
		conn.Close()
		return nil, handshake.Result{}, err
	}

	// The server may have read past the request into its buffer.
	if rw.Reader.Buffered() != 0 {
		conn = &bufferedConn{Conn: conn, r: rw.Reader}
	}
	return conn, res, nil
}

// reserved reports headers Upgrade writes itself or that make no sense on a 101.
func reserved(name string) bool {
	switch strings.ToLower(name) {
	case "upgrade", "connection", "content-length", "transfer-encoding", "content-type":
		return true
	}
	return strings.HasPrefix(strings.ToLower(name), "sec-websocket-")
}

// bufferedConn drains bytes the HTTP server buffered before reading the socket.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if c.r != nil {
		if n := c.r.Buffered(); n != 0 {
			return c.r.Read(p[:min(len(p), n)])
		}
		c.r = nil
	}
	return c.Conn.Read(p)
}
