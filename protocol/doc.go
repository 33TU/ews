// Package protocol handles WebSocket messages after the opening handshake.
// Senders and receivers use byte slices; callers own transport I/O, deadlines, and synchronization.
package protocol
