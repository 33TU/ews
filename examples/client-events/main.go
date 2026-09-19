// Command client-events is the chat client as an events.Handler: Dial runs
// the handler until the connection ends and returns the error OnClose saw.
// State known at dial time, here the name to sign messages with, reaches
// OnOpen through Config.UserData and is updated per connection after that.
//
//	go run ./examples/echo-events
//	go run ./examples/client-events -url ws://localhost:9005 -name alice
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/33TU/ews/codec"
	"github.com/33TU/ews/events"
	"github.com/33TU/ews/transport"
	"github.com/33TU/ews/ws"
)

// session is the per-connection state: set before the dial, counted after.
type session struct {
	name     string
	received int
}

// chat sends stdin lines signed with the session's name and prints replies.
type chat struct{ events.Base }

func (chat) OnOpen(c *events.Conn) {
	s := c.UserData.(*session)
	log.Printf("connected to %s as %s", c.Request.Host, s.name)

	// Writes may come from any goroutine, so stdin gets one of its own. When
	// stdin ends, Close starts the close handshake; the peer's reply ends the
	// connection and Dial returns.
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if err := c.Write(codec.Text, fmt.Appendf(nil, "%s: %s", s.name, sc.Bytes())); err != nil {
				return
			}
		}
		if err := sc.Err(); err != nil {
			log.Printf("stdin: %v", err)
		}
		c.Close(1000, "")
	}()
}

func (chat) OnMessage(c *events.Conn, _ codec.Opcode, payload []byte) error {
	c.UserData.(*session).received++
	fmt.Printf("< %s\n", payload)
	return nil
}

func (chat) OnClose(c *events.Conn, err error) {
	s := c.UserData.(*session)
	if _, ok := errors.AsType[*ws.CloseError](err); ok {
		log.Printf("closed after %d replies", s.received)
		return
	}
	log.Printf("lost after %d replies: %v", s.received, err)
}

func main() {
	url := flag.String("url", "ws://localhost:9005", "server URL")
	name := flag.String("name", "anon", "name to sign messages with")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := events.Dial(ctx, *url, transport.DialOptions{}, chat{}, ws.Config{UserData: &session{name: *name}})
	if _, ok := errors.AsType[*ws.CloseError](err); !ok {
		log.Fatal(err)
	}
}
