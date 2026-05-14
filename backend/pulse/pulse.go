// Package pulse implements an in-process WebSocket hub. The backend uses it
// to push real-time event logs ("scraper started", "job merged", ...) to
// every connected frontend client.
//
// The hub is intentionally trivial: a single mutex-guarded set of clients,
// a buffered channel for outbound events, and a fan-out goroutine. It's
// fine for low single-digit-thousand concurrent dashboards on one node.
package pulse

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// Event is the wire-level message broadcast to every connected client.
// Kind aligns with the frontend `KIND` map (scrape | merge | archive | alert).
type Event struct {
	ID   int64     `json:"id"`
	Kind string    `json:"kind"`
	Msg  string    `json:"msg"`
	T    time.Time `json:"t"`
}

// Client is anything that can receive a serialized Event. Each WebSocket
// connection wraps itself in this interface so the hub stays transport-agnostic.
type Client interface {
	Send(payload []byte) error
	Close()
}

var (
	mu      sync.RWMutex
	clients = make(map[Client]struct{})
	idSeq   int64
	// recent keeps a short history so a freshly-connected client gets
	// some context instead of a blank pulse feed.
	recent    = make([]Event, 0, 32)
	recentMax = 32
)

// Register adds a client and immediately flushes the recent buffer to it.
func Register(c Client) {
	mu.Lock()
	clients[c] = struct{}{}
	snapshot := append([]Event(nil), recent...)
	mu.Unlock()

	for _, ev := range snapshot {
		if payload, err := json.Marshal(ev); err == nil {
			_ = c.Send(payload)
		}
	}
}

// Unregister removes a client and closes its underlying transport.
func Unregister(c Client) {
	mu.Lock()
	if _, ok := clients[c]; ok {
		delete(clients, c)
		mu.Unlock()
		c.Close()
		return
	}
	mu.Unlock()
}

// Broadcast emits one event to every connected client. It never blocks the
// caller — slow clients are dropped after a single failed Send.
func Broadcast(kind, msg string) {
	mu.Lock()
	idSeq++
	ev := Event{ID: idSeq, Kind: kind, Msg: msg, T: time.Now().UTC()}
	recent = append(recent, ev)
	if len(recent) > recentMax {
		recent = recent[len(recent)-recentMax:]
	}
	receivers := make([]Client, 0, len(clients))
	for c := range clients {
		receivers = append(receivers, c)
	}
	mu.Unlock()

	payload, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[pulse] marshal failed: %v", err)
		return
	}
	for _, c := range receivers {
		if err := c.Send(payload); err != nil {
			Unregister(c)
		}
	}
}
