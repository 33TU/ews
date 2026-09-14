// Package ews connects the WebSocket handshake to the network. Upgrade
// performs the server side of the opening handshake inside a net/http
// handler, Server does it on a bare listener without net/http, and Dial is
// the client side; all hand over the raw connection for ws.NewConn, which
// the caller keeps for deadlines and closing.
//
// Frame encoding lives in codec, message I/O in ws, handshake rules in
// handshake, and permessage-deflate in deflate.
package ews
