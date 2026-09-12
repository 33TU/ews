// Package proto is the push-based WebSocket message core shared by the
// blocking ws package and future event-driven transports. It validates frames,
// tracks message and close state, and unmasks payloads. Text validity is
// checked by whoever assembles a message, since chunks are not strings. It
// never performs I/O; callers feed bytes and pull events.
package proto
