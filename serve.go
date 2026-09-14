package ews

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/33TU/ews/handshake"
)

// Server accepts WebSocket connections on a listener without net/http: one
// accept loop, a small parser for the upgrade request, and a goroutine per
// connection. It keeps no per-connection state of its own beyond what the
// handler holds, where net/http keeps about 10 KB of buffers alive for the
// life of a hijacked connection.
type Server struct {
	// Handshake is what to offer and accept: compression and subprotocols.
	Handshake handshake.Options
	// Handler runs on its own goroutine with the negotiated result. The
	// connection is closed when it returns; the handler owns deadlines.
	Handler func(conn net.Conn, res handshake.Result, req *Request)
	// HandshakeTimeout bounds reading the upgrade request. Zero means 10 seconds.
	HandshakeTimeout time.Duration
	// MaxHeaderBytes caps the request head. Zero means 8 KiB.
	MaxHeaderBytes int
	// Accept, when set, sees every parsed request before negotiation and may
	// refuse it by returning an HTTP status, for origin checks and routing.
	// Zero accepts.
	Accept func(req *Request) int

	mu        sync.Mutex
	listeners map[net.Listener]struct{}
	closed    bool
}

// Request is the parsed upgrade request: the request line and the headers
// the handshake and an Accept hook may want.
type Request struct {
	Method, Path, Host string
	// Header holds every header with its canonical name; repeated headers
	// are joined with commas, as the handshake rules expect.
	Header map[string]string
	// RemoteAddr is the connection's peer.
	RemoteAddr net.Addr
}

var errHeadTooLarge = errors.New("ews: request head too large")

// Serve accepts connections from ln until it is closed or the server is,
// then returns the accept error, or nil after Close.
func (s *Server) Serve(ln net.Listener) error {
	if s.Handler == nil {
		return errors.New("ews: Server.Handler is nil")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return net.ErrClosed
	}
	if s.listeners == nil {
		s.listeners = map[net.Listener]struct{}{}
	}
	s.listeners[ln] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.listeners, ln)
		s.mu.Unlock()
	}()

	var delay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				// Transient accept failure: back off briefly and keep serving.
				delay = min(max(2*delay, 5*time.Millisecond), time.Second)
				time.Sleep(delay)
				continue
			}
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		delay = 0
		go s.serve(conn)
	}
}

// Close stops every Serve loop. Connections already handed to the Handler
// are untouched; the handler owns them.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var first error
	for ln := range s.listeners {
		if err := ln.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// serve runs one connection: read and negotiate the upgrade, then hand off.
func (s *Server) serve(conn net.Conn) {
	timeout := s.HandshakeTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	conn.SetDeadline(time.Now().Add(timeout))
	br := bufio.NewReaderSize(conn, 4<<10)
	req, err := readRequest(br, s.maxHeaderBytes())
	if err != nil {
		if errors.Is(err, errHeadTooLarge) {
			writeError(conn, 431, "request head too large")
		} else if !isNetErr(err) {
			writeError(conn, 400, "malformed request")
		}
		conn.Close()
		return
	}
	req.RemoteAddr = conn.RemoteAddr()
	if s.Accept != nil {
		if status := s.Accept(req); status != 0 {
			writeError(conn, status, "")
			conn.Close()
			return
		}
	}
	resp, res, err := handshake.Negotiate(handshake.Request{
		Method:     req.Method,
		Upgrade:    req.Header["Upgrade"],
		Connection: req.Header["Connection"],
		Version:    req.Header["Sec-Websocket-Version"],
		Key:        req.Header["Sec-Websocket-Key"],
		Extensions: req.Header["Sec-Websocket-Extensions"],
		Protocols:  req.Header["Sec-Websocket-Protocol"],
	}, s.Handshake)
	if err != nil {
		status := handshake.StatusCode(err)
		extra := ""
		if status == 426 {
			extra = "Sec-WebSocket-Version: " + handshake.Version + "\r\n"
		}
		writeErrorWith(conn, status, err.Error(), extra)
		conn.Close()
		return
	}
	if _, err := conn.Write(responseBytes(resp)); err != nil {
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})
	var c net.Conn = conn
	if br.Buffered() != 0 {
		c = &bufferedConn{Conn: conn, r: br} // The client sent frames right behind the request.
	}
	defer conn.Close()
	s.Handler(c, res, req)
}

func (s *Server) maxHeaderBytes() int {
	if s.MaxHeaderBytes > 0 {
		return s.MaxHeaderBytes
	}
	return 8 << 10
}

// readRequest parses an HTTP/1.1 request head: the request line and the
// headers up to the blank line, within limit bytes. Header names are
// canonicalized the way net/http does, so lookups use "Sec-Websocket-Key".
func readRequest(br *bufio.Reader, limit int) (*Request, error) {
	line, err := readLine(br, &limit)
	if err != nil {
		return nil, err
	}
	method, rest, ok := strings.Cut(line, " ")
	path, version, ok2 := strings.Cut(rest, " ")
	if !ok || !ok2 || method == "" || path == "" || version != "HTTP/1.1" {
		return nil, errors.New("ews: bad request line")
	}
	req := &Request{Method: method, Path: path, Header: make(map[string]string, 12)}
	for {
		line, err := readLine(br, &limit)
		if err != nil {
			return nil, err
		}
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			return nil, errors.New("ews: bad header line")
		}
		name = canonicalHeader(name)
		value = strings.TrimSpace(value)
		if name == "Host" {
			req.Host = value
			continue
		}
		if prev, dup := req.Header[name]; dup {
			value = prev + ", " + value
		}
		req.Header[name] = value
	}
	return req, nil
}

// readLine returns one CRLF- or LF-terminated line without its terminator,
// charging its length against limit.
func readLine(br *bufio.Reader, limit *int) (string, error) {
	line, err := br.ReadSlice('\n')
	if err == bufio.ErrBufferFull {
		return "", errHeadTooLarge
	}
	if err != nil {
		if err == io.EOF && len(line) == 0 {
			return "", io.ErrUnexpectedEOF
		}
		if err == io.EOF {
			return "", io.ErrUnexpectedEOF
		}
		return "", err
	}
	*limit -= len(line)
	if *limit < 0 {
		return "", errHeadTooLarge
	}
	return string(bytes.TrimRight(line, "\r\n")), nil
}

// canonicalHeader upper-cases the first letter of each dash-separated word
// and lower-cases the rest, in place on a copy, matching net/http's form.
func canonicalHeader(name string) string {
	b := []byte(name)
	upper := true
	for i, c := range b {
		switch {
		case upper && 'a' <= c && c <= 'z':
			b[i] = c - 'a' + 'A'
		case !upper && 'A' <= c && c <= 'Z':
			b[i] = c - 'A' + 'a'
		}
		upper = c == '-'
	}
	return string(b)
}

func responseBytes(resp handshake.Response) []byte {
	b := make([]byte, 0, 200)
	b = append(b, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "...)
	b = append(b, resp.Accept...)
	b = append(b, "\r\n"...)
	if resp.Extensions != "" {
		b = append(append(append(b, "Sec-WebSocket-Extensions: "...), resp.Extensions...), "\r\n"...)
	}
	if resp.Protocol != "" {
		b = append(append(append(b, "Sec-WebSocket-Protocol: "...), resp.Protocol...), "\r\n"...)
	}
	return append(b, "\r\n"...)
}

func writeError(conn net.Conn, status int, msg string) { writeErrorWith(conn, status, msg, "") }

func writeErrorWith(conn net.Conn, status int, msg, extra string) {
	if msg == "" {
		msg = statusText(status)
	}
	conn.Write([]byte("HTTP/1.1 " + strconv.Itoa(status) + " " + statusText(status) + "\r\n" + extra +
		"Content-Type: text/plain; charset=utf-8\r\nContent-Length: " + strconv.Itoa(len(msg)+1) + "\r\nConnection: close\r\n\r\n" + msg + "\n"))
}

func statusText(status int) string {
	switch status {
	case 400:
		return "Bad Request"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 426:
		return "Upgrade Required"
	case 431:
		return "Request Header Fields Too Large"
	default:
		return "Error"
	}
}

func isNetErr(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed)
}
