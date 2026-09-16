package ws_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/ws"
)

// examplePair connects a server and a client over an in-memory pipe. Real
// programs get the transport from ews.Upgrade, ews.Server or ews.Dial.
func examplePair() (server, client *ws.Conn) {
	sc, cc := net.Pipe()
	server, _ = ws.NewConn(sc, ws.Config{Role: ws.Server})
	client, _ = ws.NewConn(cc, ws.Config{Role: ws.Client})
	return server, client
}

func ExampleConn_ReadMessage() {
	server, client := examplePair()
	go client.Write(codec.Text, []byte("hello"))

	// The payload is borrowed: it is valid until the next read call.
	op, payload, err := server.ReadMessage()
	if err != nil {
		panic(err)
	}
	fmt.Println(op == codec.Text, string(payload))
	// Output: true hello
}

func ExampleConn_Read() {
	server, client := examplePair()
	go client.Write(codec.Binary, bytes.Repeat([]byte("x"), 10))

	// A message read in chunks: NextMessage, then Read until io.EOF.
	if _, err := server.NextMessage(); err != nil {
		panic(err)
	}
	buf := make([]byte, 4)
	for {
		n, err := server.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fmt.Print(n, " ")
	}
	fmt.Println()
	// Output: 4 4 2
}

func ExampleConn_WriteTo() {
	server, client := examplePair()
	go client.Write(codec.Text, []byte("copied without a buffer"))

	if _, err := server.NextMessage(); err != nil {
		panic(err)
	}
	var sb strings.Builder
	n, err := io.Copy(&sb, server) // Uses WriteTo through io.WriterTo.
	fmt.Println(n, err, sb.String())
	// Output: 23 <nil> copied without a buffer
}

func ExampleConn_WriteFrom() {
	server, client := examplePair()
	go func() {
		// Send content of unknown length as one message, fragmented as read.
		client.WriteFrom(codec.Text, strings.NewReader("streamed from a reader"))
	}()
	_, payload, err := server.ReadMessage()
	fmt.Println(string(payload), err)
	// Output: streamed from a reader <nil>
}

func ExampleQueue() {
	server, client := examplePair()
	done := make(chan string)
	go func() {
		_, p, _ := client.ReadMessage()
		first := string(p)
		_, p, _ = client.ReadMessage()
		done <- first + " " + string(p)
	}()

	// Send returns once the message is queued; a goroutine writes it out.
	q := server.NewQueue(0)
	q.Send(codec.Text, []byte("queued"))
	// Synchronous writes join the queue in order.
	server.Write(codec.Text, []byte("ordered"))
	fmt.Println(<-done, q.Wait())
	// Output: queued ordered <nil>
}

func ExamplePrepare() {
	server, client := examplePair()
	got := make(chan string)
	go func() {
		_, p, _ := client.ReadMessage()
		got <- string(p)
	}()

	// Encode once, send to every recipient by reference.
	p, err := ws.Prepare(codec.Text, []byte("to everyone"))
	if err != nil {
		panic(err)
	}
	q := server.NewQueue(0)
	q.SendPrepared(p)
	fmt.Println(<-got)
	// Output: to everyone
}

func ExampleNetConn() {
	server, client := examplePair()
	sc, cc := ws.NetConn(server, codec.Binary), ws.NetConn(client, codec.Binary)
	go func() {
		// Each Write is one message; the reader sees a byte stream.
		io.WriteString(cc, "tunneled ")
		io.WriteString(cc, "bytes")
		cc.Close()
	}()
	all, err := io.ReadAll(sc)
	fmt.Println(string(all), err)
	// Output: tunneled bytes <nil>
}
