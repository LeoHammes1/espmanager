package httpapi

import (
	"net/http"
	"sync"
)

type SSEHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func NewSSEHub() *SSEHub {
	return &SSEHub{clients: make(map[chan struct{}]struct{})}
}

// Changed signals every connected client to refresh. One generic tick is
// enough: the SPA invalidates its query cache and refetches on any change.
func (h *SSEHub) Changed() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *SSEHub) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		ch := make(chan struct{}, 1)
		h.add(ch)
		defer h.remove(ch)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				if _, err := w.Write([]byte("data: changed\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func (h *SSEHub) add(ch chan struct{}) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *SSEHub) remove(ch chan struct{}) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}
