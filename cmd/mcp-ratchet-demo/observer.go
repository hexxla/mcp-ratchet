package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// WebSocket upgrader configuration
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == ""
	},
}

// eventBroadcaster manages WebSocket connections and broadcasts events
type eventBroadcaster struct {
	mu          sync.RWMutex
	connections map[ratchetDomain.SessionID]map[*websocket.Conn]struct{}
}

func newEventBroadcaster() *eventBroadcaster {
	return &eventBroadcaster{
		connections: make(map[ratchetDomain.SessionID]map[*websocket.Conn]struct{}),
	}
}

func (b *eventBroadcaster) subscribe(sessionID ratchetDomain.SessionID, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.connections[sessionID] == nil {
		b.connections[sessionID] = make(map[*websocket.Conn]struct{})
	}
	b.connections[sessionID][conn] = struct{}{}
}

func (b *eventBroadcaster) unsubscribe(sessionID ratchetDomain.SessionID, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if conns, ok := b.connections[sessionID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(b.connections, sessionID)
		}
	}
}

// Broadcast implements EventBroadcaster interface.
// Sends the event to all WebSocket clients subscribed to this session.
func (b *eventBroadcaster) Broadcast(sessionID ratchetDomain.SessionID, event *ratchetDomain.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	conns := b.connections[sessionID]
	for conn := range conns {
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = conn.WriteJSON(redactDemoEvent(event))
	}
}

func redactDemoEvent(event *ratchetDomain.Event) *ratchetDomain.Event {
	if event == nil {
		return nil
	}
	safe := *event
	safe.Token = ""
	safe.Metadata = nil
	return &safe
}
