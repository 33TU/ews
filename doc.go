// Package ews connects the WebSocket handshake to net/http. Upgrade performs
// the server side of the opening handshake and returns the raw connection
// for ws.NewConn; the caller keeps it for deadlines and closing.
//
// Frame encoding lives in codec, message I/O in ws, handshake rules in
// handshake, and permessage-deflate in deflate.
package ews
