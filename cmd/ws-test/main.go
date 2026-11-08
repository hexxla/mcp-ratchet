package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"

	"github.com/gorilla/websocket"
)

func main() {
	endpoint := flag.String("url", "ws://127.0.0.1:8080/observability/stream", "demo WebSocket URL")
	session := flag.String("session", "", "MCP session ID")
	flag.Parse()
	if *session == "" {
		log.Fatal("-session is required")
	}
	u, err := url.Parse(*endpoint)
	if err != nil {
		log.Fatal("URL: ", err)
	}
	query := u.Query()
	query.Set("session_id", *session)
	u.RawQuery = query.Encode()
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer func() { _ = c.Close() }()

	log.Println("connected to WebSocket, listening for events...")

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}
		fmt.Printf("EVENT: %s\n", message)
	}
}
