package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/lucientong/waggle/pkg/observe"
)

// sseClient represents a single connected SSE subscriber.
type sseClient struct {
	ch     chan string
	done   <-chan struct{} // closed when the client disconnects
}

// sseHub manages all connected SSE clients and broadcasts events to them.
type sseHub struct {
	clients  map[*sseClient]struct{}
	register chan *sseClient
	remove   chan *sseClient
	inbound  chan observe.Event
}

// newSSEHub creates an unstarted SSE hub.
func newSSEHub() *sseHub {
	return &sseHub{
		clients:  make(map[*sseClient]struct{}),
		register: make(chan *sseClient, 8),
		remove:   make(chan *sseClient, 8),
		inbound:  make(chan observe.Event, 256),
	}
}

// run is the hub's event loop. Must be called in a separate goroutine.
func (h *sseHub) run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
			slog.Debug("sse client connected", "total", len(h.clients))

		case c := <-h.remove:
			delete(h.clients, c)
			close(c.ch)
			slog.Debug("sse client disconnected", "total", len(h.clients))

		case event := <-h.inbound:
			data, err := json.Marshal(toEventJSON(event))
			if err != nil {
				continue
			}
			msg := fmt.Sprintf("data: %s\n\n", string(data))
			for c := range h.clients {
				select {
				case c.ch <- msg:
				default:
					// Slow client: drop the message to avoid blocking the hub.
				}
			}
		}
	}
}

// publish sends an event to all connected clients.
func (h *sseHub) publish(event observe.Event) {
	select {
	case h.inbound <- event:
	default:
		slog.Warn("sse hub: inbound buffer full, dropping event", "type", event.Type)
	}
}

// handleSSE is the HTTP handler for GET /api/events.
// Each connected client receives a stream of SSE messages.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Verify the client supports streaming.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := &sseClient{
		ch:   make(chan string, 32),
		done: r.Context().Done(),
	}
	s.hub.register <- client

	defer func() {
		s.hub.remove <- client
	}()

	// Send an initial "connected" ping.
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-client.ch:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// toEventJSON converts an observe.Event to the JSON-serializable eventJSON struct.
func toEventJSON(e observe.Event) eventJSON {
	return eventJSON{
		Type:       string(e.Type),
		AgentName:  e.AgentName,
		WorkflowID: e.WorkflowID,
		Timestamp:  e.Timestamp,
		DurationMs: e.Duration.Milliseconds(),
		Error:      e.Error,
		InputSize:  e.InputSize,
		OutputSize: e.OutputSize,
		Metadata:   e.Metadata,
	}
}
