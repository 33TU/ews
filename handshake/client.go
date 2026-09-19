package handshake

import "strings"

// NewRequest builds a client's handshake request with a fresh key.
func NewRequest(opts Options) (Request, error) {
	if err := validOptions(opts); err != nil {
		return Request{}, err
	}

	req := Request{
		Method:     "GET",
		Upgrade:    "websocket",
		Connection: "Upgrade",
		Version:    Version,
		Key:        NewKey(),
		Protocols:  strings.Join(opts.Protocols, ", "),
	}
	if c := opts.Compression; c != nil {
		// Offering client_max_window_bits lets the server shrink our window,
		// which we can honor at any size.
		req.Extensions = deflateName + "; client_max_window_bits"
		if !c.ContextTakeover {
			req.Extensions += "; client_no_context_takeover; server_no_context_takeover"
		}
	}
	return req, nil
}

// Confirm validates the server's response to req and reports what was agreed.
func Confirm(req Request, resp Response, opts Options) (Result, error) {
	if resp.Status != 101 {
		return Result{}, ErrBadStatus
	}
	if !hasToken(resp.Upgrade, "websocket") || !hasToken(resp.Connection, "Upgrade") {
		return Result{}, ErrNotWebSocket
	}
	if resp.Accept != Accept(req.Key) {
		return Result{}, ErrBadAccept
	}
	if resp.Protocol != "" && !hasToken(req.Protocols, resp.Protocol) {
		return Result{}, ErrBadProtocol
	}
	res := Result{Protocol: resp.Protocol}

	exts, err := parseExtensions(resp.Extensions)
	if err != nil {
		return Result{}, err
	}
	if len(exts) == 0 {
		return res, nil
	}
	if len(exts) != 1 || !strings.EqualFold(exts[0].name, deflateName) || opts.Compression == nil {
		return Result{}, ErrBadExtension
	}
	p, ok := parseDeflate(exts[0])
	if !ok {
		return Result{}, ErrBadExtension
	}

	takeover := opts.Compression.ContextTakeover
	res.Compression = &Compression{
		Level:                  opts.Compression.Level,
		MinSize:                opts.Compression.MinSize,
		SendContextTakeover:    takeover && !p.clientNoContextTakeover,
		ReceiveContextTakeover: takeover && !p.serverNoContextTakeover,
		SendWindowBits:         p.clientMaxWindowBits, // The server's choice; zero keeps the full window.
		ReceiveWindowBits:      p.serverMaxWindowBits, // The server's declared window, if any.
	}
	return res, nil
}
