package handshake

import (
	"strconv"
	"strings"
)

const deflateName = "permessage-deflate"

type param struct{ name, value string }

type extension struct {
	name   string
	params []param
}

// parseExtensions parses a Sec-WebSocket-Extensions value: offers separated
// by commas, each a name followed by semicolon-separated parameters with
// optional token or quoted-string values.
func parseExtensions(header string) ([]extension, error) {
	var out []extension
	for offer := range strings.SplitSeq(header, ",") {
		offer = strings.TrimSpace(offer)
		if offer == "" {
			continue
		}
		parts := strings.Split(offer, ";")
		e := extension{name: strings.TrimSpace(parts[0])}
		if e.name == "" {
			return nil, ErrBadExtension
		}
		for _, p := range parts[1:] {
			name, value, _ := strings.Cut(strings.TrimSpace(p), "=")
			name = strings.TrimSpace(name)
			value = strings.Trim(strings.TrimSpace(value), `"`)
			if name == "" {
				return nil, ErrBadExtension
			}
			e.params = append(e.params, param{name, value})
		}
		out = append(out, e)
	}
	return out, nil
}

// deflateParams is one parsed permessage-deflate offer or acceptance.
type deflateParams struct {
	serverNoContextTakeover bool
	clientNoContextTakeover bool
	serverMaxWindowBits     int // 0 when absent.
	clientMaxWindowBits     int // 0 when absent, 15 when present without a value.
}

// parseDeflate interprets the parameters of a permessage-deflate extension.
// ok is false for a syntactically valid offer this endpoint cannot honor.
func parseDeflate(e extension) (p deflateParams, ok bool) {
	seen := map[string]bool{}
	for _, kv := range e.params {
		name := strings.ToLower(kv.name)
		if seen[name] {
			return p, false
		}
		seen[name] = true
		switch name {
		case "server_no_context_takeover":
			if kv.value != "" {
				return p, false
			}
			p.serverNoContextTakeover = true
		case "client_no_context_takeover":
			if kv.value != "" {
				return p, false
			}
			p.clientNoContextTakeover = true
		case "server_max_window_bits":
			bits, ok := windowBits(kv.value)
			if !ok || bits == 0 {
				return p, false
			}
			p.serverMaxWindowBits = bits
		case "client_max_window_bits":
			bits, ok := windowBits(kv.value)
			if !ok {
				return p, false
			}
			if bits == 0 {
				bits = 15
			}
			p.clientMaxWindowBits = bits
		default:
			return p, false
		}
	}
	return p, true
}

// windowBits parses an optional window size parameter; 0 means no value.
func windowBits(value string) (int, bool) {
	if value == "" {
		return 0, true
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 8 || n > 15 {
		return 0, false
	}
	return n, true
}

// formatDeflate builds the server's header value for a negotiated configuration.
func formatDeflate(c *Compression) string {
	s := deflateName
	if !c.SendContextTakeover {
		s += "; server_no_context_takeover"
	}
	if !c.ReceiveContextTakeover {
		s += "; client_no_context_takeover"
	}
	if c.SendWindowBits != 0 && c.SendWindowBits != 15 {
		s += "; server_max_window_bits=" + strconv.Itoa(c.SendWindowBits)
	}
	return s
}
