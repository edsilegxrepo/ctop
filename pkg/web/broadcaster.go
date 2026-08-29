package web

import (
	"sync"
	"time"
)

// Broadcaster manages real-time Server-Sent Events (SSE) subscriber channels and telemetry history.
type Broadcaster struct {
	mu             sync.RWMutex
	subscribers    map[chan TelemetryEvent]struct{}
	history        []TelemetryEvent
	maxHistory     int
	maxSubscribers int
}

// NewBroadcaster constructs a new event broadcaster with a 5,000 event capacity and 256 max subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers:    make(map[chan TelemetryEvent]struct{}),
		history:        make([]TelemetryEvent, 0, 100),
		maxHistory:     5000,
		maxSubscribers: 256,
	}
}

// SetMaxSubscribers configures the maximum concurrent subscriber channels allowed.
func (b *Broadcaster) SetMaxSubscribers(limit int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 {
		limit = 256
	}
	b.maxSubscribers = limit
}

// SetMaxHistory sets the maximum in-memory event retention limit.
func (b *Broadcaster) SetMaxHistory(limit int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 {
		limit = 1000
	}
	b.maxHistory = limit
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
}

// Subscribe registers a new subscriber channel or returns nil if max limit is reached.
func (b *Broadcaster) Subscribe() chan TelemetryEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.maxSubscribers > 0 && len(b.subscribers) >= b.maxSubscribers {
		return nil
	}

	ch := make(chan TelemetryEvent, 64)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes and closes a subscriber channel.
func (b *Broadcaster) Unsubscribe(ch chan TelemetryEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[ch]; exists {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// Broadcast distributes a telemetry event to all connected subscribers non-blockingly.
func (b *Broadcaster) Broadcast(event TelemetryEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Append to circular history
	b.history = append(b.history, event)
	if len(b.history) > b.maxHistory {
		b.history = b.history[1:]
	}

	// Non-blocking broadcast to active subscribers
	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Buffer full, skip to keep pipeline non-blocking
		}
	}
}

// GetLatestEvent returns the most recent telemetry event or an empty event if none exists.
func (b *Broadcaster) GetLatestEvent() TelemetryEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.history) == 0 {
		return TelemetryEvent{}
	}
	return b.history[len(b.history)-1]
}

// SubscriberCount returns the current count of connected subscribers.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
