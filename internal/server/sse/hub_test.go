package sse_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xb/ari/internal/server/sse"
)

func TestHub_SubscribeAndPublish(t *testing.T) {
	hub := sse.NewHub()
	squadID := uuid.New()

	sub := hub.Subscribe(squadID)
	defer hub.Unsubscribe(sub)

	hub.Publish(squadID, "test.event", map[string]string{"key": "value"})

	select {
	case evt := <-sub.Ch:
		if evt.Type != "test.event" {
			t.Fatalf("expected event type %q, got %q", "test.event", evt.Type)
		}
		if evt.ID <= 0 {
			t.Fatalf("expected positive event ID, got %d", evt.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHub_SquadIsolation(t *testing.T) {
	hub := sse.NewHub()
	squad1 := uuid.New()
	squad2 := uuid.New()

	sub1 := hub.Subscribe(squad1)
	defer hub.Unsubscribe(sub1)
	sub2 := hub.Subscribe(squad2)
	defer hub.Unsubscribe(sub2)

	hub.Publish(squad1, "squad1.event", nil)

	// sub1 should receive it
	select {
	case evt := <-sub1.Ch:
		if evt.Type != "squad1.event" {
			t.Fatalf("expected squad1.event, got %q", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("sub1 timed out")
	}

	// sub2 should NOT receive it
	select {
	case evt := <-sub2.Ch:
		t.Fatalf("sub2 should not receive squad1 events, got %v", evt)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event for squad2
	}
}

func TestHub_Unsubscribe(t *testing.T) {
	hub := sse.NewHub()
	squadID := uuid.New()

	sub := hub.Subscribe(squadID)
	if hub.SubscriberCount(squadID) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", hub.SubscriberCount(squadID))
	}

	hub.Unsubscribe(sub)
	if hub.SubscriberCount(squadID) != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe, got %d", hub.SubscriberCount(squadID))
	}

	// Channel should be closed
	_, ok := <-sub.Ch
	if ok {
		t.Fatal("expected channel to be closed")
	}
}

func TestHub_SlowSubscriberDoesNotBlock(t *testing.T) {
	hub := sse.NewHub()
	squadID := uuid.New()

	sub := hub.Subscribe(squadID)
	defer hub.Unsubscribe(sub)

	// Fill the channel buffer
	for i := 0; i < sse.SubscriberBufferSize+10; i++ {
		hub.Publish(squadID, "flood", i)
	}

	// The Publish calls should not have blocked
	count := 0
	for {
		select {
		case <-sub.Ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != sse.SubscriberBufferSize {
		t.Fatalf("expected %d events (buffer size), got %d", sse.SubscriberBufferSize, count)
	}
}

func TestHub_ConcurrentPublishSubscribe(t *testing.T) {
	hub := sse.NewHub()
	squadID := uuid.New()

	var wg sync.WaitGroup

	// Launch 10 subscribers
	subs := make([]*sse.Subscriber, 10)
	for i := range subs {
		subs[i] = hub.Subscribe(squadID)
	}

	// Publish 100 events concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			hub.Publish(squadID, "concurrent", n)
		}(i)
	}

	wg.Wait()

	// Clean up
	for _, sub := range subs {
		hub.Unsubscribe(sub)
	}
}

func TestHub_MonotonicEventIDs(t *testing.T) {
	hub := sse.NewHub()
	squadID := uuid.New()

	sub := hub.Subscribe(squadID)
	defer hub.Unsubscribe(sub)

	for i := 0; i < 10; i++ {
		hub.Publish(squadID, "seq", i)
	}

	var lastID int64
	for i := 0; i < 10; i++ {
		evt := <-sub.Ch
		if evt.ID <= lastID {
			t.Fatalf("event IDs not monotonic: %d <= %d", evt.ID, lastID)
		}
		lastID = evt.ID
	}
}
