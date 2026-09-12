// Package proto is the push-based WebSocket message core shared by the
// blocking ws package and future event-driven transports. It validates frames,
// tracks message and close state, unmasks payloads, and checks UTF-8
// incrementally. It never performs I/O; callers feed bytes and pull events.
package proto
