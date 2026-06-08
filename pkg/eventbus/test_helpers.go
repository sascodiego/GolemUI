package eventbus

// SubscriberCount returns the number of active subscribers for a channel.
// Exported for testing and diagnostics.
func (b *InMemEventBus) SubscriberCount(channel string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[channel])
}
