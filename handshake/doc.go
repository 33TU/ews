// Package handshake implements the WebSocket opening handshake rules without
// performing I/O: key and accept computation, request and response
// validation, permessage-deflate negotiation, and subprotocol selection.
// Everything takes and returns header values, so it serves net/http servers,
// clients, and event-driven transports alike.
package handshake
