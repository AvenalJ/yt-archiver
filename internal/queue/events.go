package queue

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type EventMessage struct {
	Type string      `json:"type"` // "progress", "status_change", "queue_update", "toast", "circuit_breaker"
	Data interface{} `json:"data"`
}

type SSEBroadcaster struct {
	clients map[chan []byte]bool
	mu      sync.RWMutex
}

var Broadcaster = &SSEBroadcaster{
	clients: make(map[chan []byte]bool),
}

func (b *SSEBroadcaster) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 64)
	b.clients[ch] = true
	return ch
}

func (b *SSEBroadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

func (b *SSEBroadcaster) Broadcast(eventType string, data interface{}) {
	msg := EventMessage{
		Type: eventType,
		Data: data,
	}
	bytesData, err := json.Marshal(msg)
	if err != nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- bytesData:
		default:
			// Client channel full or slow, skip
		}
	}
}

func (b *SSEBroadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Heartbeat ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Initial ping
	fmt.Fprintf(w, "event: ping\ndata: connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var ev EventMessage
			if err := json.Unmarshal(msg, &ev); err == nil && ev.Type != "" {
				dataBytes, _ := json.Marshal(ev.Data)
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(dataBytes))
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
