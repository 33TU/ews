package codec

// Opcode identifies a WebSocket frame's type.
type Opcode uint8

// The opcodes defined by RFC 6455. Continuation, Text and Binary carry data;
// Close, Ping and Pong are control frames.
const (
	Continuation Opcode = 0x0
	Text         Opcode = 0x1
	Binary       Opcode = 0x2
	Close        Opcode = 0x8
	Ping         Opcode = 0x9
	Pong         Opcode = 0xA
)
