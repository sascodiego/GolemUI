package eventbus

import (
	"fmt"
	"sync"
)

// SubmitChannelPrefix is the prefix for scoped submit channels.
// Actual channels are: "screen:submit:<vistaID>"
const SubmitChannelPrefix = "screen:submit"

type Event struct {
	Channel string
	Payload interface{}
}

type Handler func(Event)

type EventBus interface {
	Publish(channel string, payload interface{})
	Subscribe(channel string, h Handler) string // Returns unique sub ID
	Unsubscribe(channel string, subID string)
}

type InMemEventBus struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]Handler
	nextSubID   uint64
}

func NewEventBus() EventBus {
	return &InMemEventBus{
		subscribers: make(map[string]map[string]Handler),
	}
}

func (b *InMemEventBus) Subscribe(channel string, h Handler) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[channel]; !exists {
		b.subscribers[channel] = make(map[string]Handler)
	}

	b.nextSubID++
	subID := fmt.Sprintf("%s:%d", channel, b.nextSubID)
	b.subscribers[channel][subID] = h
	return subID
}

func (b *InMemEventBus) Unsubscribe(channel string, subID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, exists := b.subscribers[channel]; exists {
		delete(subs, subID)
		if len(subs) == 0 {
			delete(b.subscribers, channel)
		}
	}
}

func (b *InMemEventBus) Publish(channel string, payload interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.subscribers[channel]
	if !exists {
		return
	}

	event := Event{
		Channel: channel,
		Payload: payload,
	}

	for _, handler := range subs {
		h := handler
		go h(event)
	}
}
