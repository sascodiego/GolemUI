package eventbus

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// SubscriberCount returns the number of active subscribers for a channel.
// For testing only — requires the bus to be *InMemEventBus.
func subscriberCount(t testing.TB, bus EventBus, channel string) int {
	t.Helper()
	b, ok := bus.(*InMemEventBus)
	if !ok {
		t.Fatalf("expected *InMemEventBus, got %T", bus)
	}
	return b.SubscriberCount(channel)
}

func TestEventBus_HappyPath(t *testing.T) {
	bus := NewEventBus()

	var wg sync.WaitGroup
	wg.Add(1)

	var receivedPayload interface{}
	var receivedChannel string

	handler := func(ev Event) {
		receivedChannel = ev.Channel
		receivedPayload = ev.Payload
		wg.Done()
	}

	subID := bus.Subscribe("test:channel", handler)
	if subID == "" {
		t.Fatal("expected a non-empty subscription ID")
	}

	bus.Publish("test:channel", "hello world")

	// Wait with timeout to prevent test hang
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event publication")
	}

	if receivedChannel != "test:channel" {
		t.Errorf("expected channel 'test:channel', got '%s'", receivedChannel)
	}
	if receivedPayload != "hello world" {
		t.Errorf("expected payload 'hello world', got '%v'", receivedPayload)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()

	var received bool
	handler := func(ev Event) {
		received = true
	}

	subID := bus.Subscribe("test:channel", handler)
	bus.Unsubscribe("test:channel", subID)

	bus.Publish("test:channel", "should not receive")

	// Wait a brief moment to ensure no publication happens asynchronously
	time.Sleep(10 * time.Millisecond)

	if received {
		t.Error("expected handler to not be called after unsubscribe")
	}
}

func TestEventBus_SlowSubscriber(t *testing.T) {
	bus := NewEventBus()

	slowDone := make(chan struct{})
	fastDone := make(chan struct{})

	// Slow subscriber
	bus.Subscribe("test:channel", func(ev Event) {
		time.Sleep(100 * time.Millisecond)
		close(slowDone)
	})

	// Fast subscriber
	bus.Subscribe("test:channel", func(ev Event) {
		close(fastDone)
	})

	startTime := time.Now()
	bus.Publish("test:channel", "data")

	// Fast subscriber should finish immediately
	select {
	case <-fastDone:
		// Fast completed. Let's ensure it happened before the slow one.
		select {
		case <-slowDone:
			t.Fatal("slow subscriber completed before or at the same time as fast subscriber, but fast should have finished immediately without waiting for slow")
		default:
			// Success, fast completed first
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("fast subscriber was blocked or too slow to run")
	}

	// Then, slow should eventually finish
	select {
	case <-slowDone:
		// Success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("slow subscriber timed out")
	}

	duration := time.Since(startTime)
	if duration >= 300*time.Millisecond {
		t.Errorf("expected test to take less than 300ms, took %v", duration)
	}
}

func TestEventBus_Concurrency(t *testing.T) {
	bus := NewEventBus()
	numGoroutines := 100
	var wg sync.WaitGroup

	// Spin up subscribers
	subIDs := make([]string, numGoroutines)
	var subMu sync.Mutex

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			channel := fmt.Sprintf("chan-%d", id%5)
			subID := bus.Subscribe(channel, func(ev Event) {
				// Do some dummy work
			})
			subMu.Lock()
			subIDs[id] = subID
			subMu.Unlock()
		}(i)
	}
	wg.Wait()

	// Spin up parallel publishers and unsubscribers
	wg.Add(numGoroutines * 2)
	for i := 0; i < numGoroutines; i++ {
		// Publishers
		go func(id int) {
			defer wg.Done()
			channel := fmt.Sprintf("chan-%d", id%5)
			bus.Publish(channel, fmt.Sprintf("payload-%d", id))
		}(i)

		// Unsubscribers
		go func(id int) {
			defer wg.Done()
			channel := fmt.Sprintf("chan-%d", id%5)

			subMu.Lock()
			subID := subIDs[id]
			subMu.Unlock()

			bus.Unsubscribe(channel, subID)
		}(i)
	}
	wg.Wait()
}

func TestSubmitChannelPrefix_Constant(t *testing.T) {
	if SubmitChannelPrefix != "screen:submit" {
		t.Errorf("expected SubmitChannelPrefix = %q, got %q", "screen:submit", SubmitChannelPrefix)
	}
}
