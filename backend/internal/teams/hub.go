package teams

import (
	"encoding/json"
	"sync"
)

// Hub fans room events out to every open tab of every member, keyed by
// user id. A slow subscriber never blocks the sender: a full channel drops
// the frame and the client's periodic refresh catches up.
type Hub struct {
	mu   sync.Mutex
	subs map[uint]map[chan []byte]struct{}
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[uint]map[chan []byte]struct{})}
}

// Subscribe opens a channel for one user's tab.
func (h *Hub) Subscribe(userID uint) chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	if h.subs[userID] == nil {
		h.subs[userID] = make(map[chan []byte]struct{})
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe closes the tab's channel.
func (h *Hub) Unsubscribe(userID uint, ch chan []byte) {
	h.mu.Lock()
	if set, ok := h.subs[userID]; ok {
		if _, ok := set[ch]; ok {
			delete(set, ch)
			close(ch)
		}
		if len(set) == 0 {
			delete(h.subs, userID)
		}
	}
	h.mu.Unlock()
}

// Send delivers one event to every tab of the given users.
func (h *Hub) Send(userIDs []uint, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range userIDs {
		for ch := range h.subs[id] {
			select {
			case ch <- data:
			default:
			}
		}
	}
}
