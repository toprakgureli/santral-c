package teams

import (
	"encoding/json"
	"sync"
)

// Hub fans room events out to every open tab of every member, keyed by
// user id. A slow subscriber never blocks the sender: a full channel drops
// the frame and the client's periodic refresh catches up. The set of
// subscribed users doubles as chat presence: a person with at least one
// open stream is online.
type Hub struct {
	mu    sync.Mutex
	subs  map[uint]map[chan []byte]struct{}
	rooms map[uint]uint // the room open in front of the person, if any
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[uint]map[chan []byte]struct{}), rooms: make(map[uint]uint)}
}

// SetRoom records which room the person is looking at (0: none) and
// returns whether it changed and what it was before.
func (h *Hub) SetRoom(userID uint, room uint) (bool, uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	old := h.rooms[userID]
	if len(h.subs[userID]) == 0 {
		return false, old
	}
	if old == room {
		return false, old
	}
	if room == 0 {
		delete(h.rooms, userID)
	} else {
		h.rooms[userID] = room
	}
	return true, old
}

// Rooms returns the room each of the given users is looking at.
func (h *Hub) Rooms(ids []uint) map[uint]uint {
	out := make(map[uint]uint, len(ids))
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range ids {
		if r, ok := h.rooms[id]; ok {
			out[id] = r
		}
	}
	return out
}

// Subscribe opens a channel for one user's tab. The flag is true when this
// is the user's first open tab, that is, when they just came online.
func (h *Hub) Subscribe(userID uint) (chan []byte, bool) {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	first := h.subs[userID] == nil
	if first {
		h.subs[userID] = make(map[chan []byte]struct{})
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, first
}

// Unsubscribe closes the tab's channel. The flag is true when it was the
// user's last open tab, that is, when they just went offline.
func (h *Hub) Unsubscribe(userID uint, ch chan []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.subs[userID]
	if !ok {
		return false
	}
	if _, ok := set[ch]; ok {
		delete(set, ch)
		close(ch)
	}
	if len(set) == 0 {
		delete(h.subs, userID)
		delete(h.rooms, userID)
		return true
	}
	return false
}

// Online reports which of the given users have an open stream.
func (h *Hub) Online(ids []uint) map[uint]bool {
	out := make(map[uint]bool, len(ids))
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range ids {
		if len(h.subs[id]) > 0 {
			out[id] = true
		}
	}
	return out
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
		h.push(id, data)
	}
}

// Broadcast delivers one event to everyone with an open stream.
func (h *Hub) Broadcast(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range h.subs {
		h.push(id, data)
	}
}

func (h *Hub) push(id uint, data []byte) {
	for ch := range h.subs[id] {
		select {
		case ch <- data:
		default:
		}
	}
}
