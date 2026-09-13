package handshake

import "strings"

// Negotiate validates a client's request and builds the server's response.
// Errors map to HTTP statuses through StatusCode. An offer this server cannot
// honor is skipped rather than rejected; the connection then proceeds
// without that extension or subprotocol.
func Negotiate(req Request, opts Options) (Response, Result, error) {
	if err := validOptions(opts); err != nil {
		return Response{}, Result{}, err
	}
	if req.Method != "GET" || !hasToken(req.Upgrade, "websocket") || !hasToken(req.Connection, "Upgrade") {
		return Response{}, Result{}, ErrNotWebSocket
	}
	if req.Version != Version {
		return Response{}, Result{}, ErrBadVersion
	}
	if !validKey(req.Key) {
		return Response{}, Result{}, ErrBadKey
	}

	resp := Response{Status: 101, Upgrade: "websocket", Connection: "Upgrade", Accept: Accept(req.Key)}
	var res Result

	if opts.Compression != nil {
		exts, err := parseExtensions(req.Extensions)
		if err != nil {
			return Response{}, Result{}, err
		}
		for _, e := range exts {
			if !strings.EqualFold(e.name, deflateName) {
				continue
			}
			p, ok := parseDeflate(e)
			if !ok {
				continue
			}
			takeover := opts.Compression.ContextTakeover
			res.Compression = &Compression{
				Level:                  opts.Compression.Level,
				MinSize:                opts.Compression.MinSize,
				SendContextTakeover:    takeover && !p.serverNoContextTakeover,
				ReceiveContextTakeover: takeover && !p.clientNoContextTakeover,
				SendWindowBits:         p.serverMaxWindowBits, // Zero keeps the full window.
				ReceiveWindowBits:      p.clientMaxWindowBits, // The client's declared window, if any.
			}
			resp.Extensions = formatDeflate(res.Compression)
			break
		}
	}

	res.Protocol = selectProtocol(req.Protocols, opts.Protocols)
	resp.Protocol = res.Protocol
	return resp, res, nil
}

// selectProtocol picks the server's most preferred subprotocol that the
// client offered, or empty.
func selectProtocol(requested string, supported []string) string {
	for _, s := range supported {
		if hasToken(requested, s) {
			return s
		}
	}
	return ""
}
