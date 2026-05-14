package handlers

import (
	"sync"

	"jobscout/pulse"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// PulseUpgrader is the HTTP middleware that gates /ws/pulse — only
// websocket-upgrade requests are allowed through. Mount it on the route
// group before WSPulse.
func PulseUpgrader(c *fiber.Ctx) error {
	if websocket.IsWebSocketUpgrade(c) {
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}

// wsClient adapts *websocket.Conn to pulse.Client. Sends are serialized
// behind a per-connection mutex because the gorilla/fasthttp ws Conn is
// not safe for concurrent writers.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
	done chan struct{}
}

func (w *wsClient) Send(payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.TextMessage, payload)
}

func (w *wsClient) Close() {
	select {
	case <-w.done:
		return
	default:
		close(w.done)
	}
	_ = w.conn.Close()
}

// WSPulse is the actual websocket handler. Lives at GET /ws/pulse.
//
// Lifecycle: register on connect, run a read-loop solely to detect the
// client closing the tab (any read error == disconnect), unregister.
var WSPulse = websocket.New(func(c *websocket.Conn) {
	client := &wsClient{conn: c, done: make(chan struct{})}
	pulse.Register(client)
	defer pulse.Unregister(client)

	// Drain inbound frames. We don't process them; this loop exists so we
	// notice when the client navigates away or closes the tab.
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
})
