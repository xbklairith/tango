// Package sse provides a per-squad Server-Sent Events fan-out hub.
package sse

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// SubscriberBufferSize is the channel buffer size for each subscriber.
const SubscriberBufferSize = 64

// Event represents a single SSE event to be delivered to subscribers.
type Event struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Subscriber represents a single SSE client connection.
type Subscriber struct {
	SquadID uuid.UUID
	Ch      chan Event
}

// Hub manages per-squad SSE subscriber channels.
// All methods are safe for concurrent use.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[*Subscriber]struct{} // squadID → set
	counter     atomic.Int64                           // monotonic event ID
}

// NewHub creates a new SSE hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[uuid.UUID]map[*Subscriber]struct{}),
	}
}

// Subscribe registers a subscriber for the given squad and returns it.
// The caller must call Unsubscribe when the client disconnects.
func (h *Hub) Subscribe(squadID uuid.UUID) *Subscriber {
	sub := &Subscriber{
		SquadID: squadID,
		Ch:      make(chan Event, SubscriberBufferSize),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscribers[squadID] == nil {
		h.subscribers[squadID] = make(map[*Subscriber]struct{})
	}
	h.subscribers[squadID][sub] = struct{}{}

	return sub
}

// Unsubscribe removes the subscriber and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.subscribers[s.SquadID]; ok {
		delete(subs, s)
		if len(subs) == 0 {
			delete(h.subscribers, s.SquadID)
		}
	}
	close(s.Ch)
}

// Publish emits an event to all subscribers on the given squad.
// Non-blocking: if a subscriber's channel is full, the event is dropped
// for that subscriber (REQ-059 — slow subscriber must not block others).
func (h *Hub) Publish(squadID uuid.UUID, eventType string, data any) {
	evt := Event{
		ID:   h.counter.Add(1),
		Type: eventType,
		Data: data,
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	subs := h.subscribers[squadID]
	for sub := range subs {
		select {
		case sub.Ch <- evt:
		default:
			// Drop event for slow subscriber (REQ-059)
		}
	}
}

// SubscriberCount returns the number of active subscribers for a given squad.
func (h *Hub) SubscriberCount(squadID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers[squadID])
}
