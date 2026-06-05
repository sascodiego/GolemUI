# Specification: Client Reactivity Broker (client-reactivity-broker)

## Introduction
The `client-reactivity-broker` capability defines a local, thread-safe in-memory event bus/broker for the GolemUI client. It facilitates low-latency pub/sub communication between UI widgets. Widgets express dependency or publish actions to event channels specified in their metadata (using the `bind_to` key), enabling reactivity without server-side database round-trips.

## Requirements
1. The reactivity broker MUST maintain a thread-safe, in-memory registry of event channels and their subscribers.
2. The broker MUST allow widgets to subscribe to specific named event channels dynamically.
3. The broker MUST support publishing event payloads (containing channel name, source identifier, and optional data) to any active channel.
4. When an event is published to a channel, the broker MUST broadcast the payload to all active subscribers on that channel.
5. The reactivity broker MUST execute message delivery asynchronously or concurrently to prevent slow consumers from blocking the UI thread or publisher.
6. The broker MUST provide a mechanism to unsubscribe widgets or clean up channels when components or layouts are destroyed to prevent memory leaks.

## Scenarios

### Scenario 1: Successful Pub/Sub Event Broadcast
*   **GIVEN** an active client reactivity broker with two text widgets subscribed to the channel `"cart:items_count"`
*   **WHEN** a numeric pad widget publishes an event to the `"cart:items_count"` channel with payload `5`
*   **THEN** the broker MUST deliver the event payload to both subscribed text widgets
*   **AND** the target widgets SHALL update their internal states and refresh their display accordingly.

### Scenario 2: Prevention of Message Blocking with Slow Subscribers
*   **GIVEN** a fast publisher widget, a slow-processing subscriber widget, and a fast-processing subscriber widget on the same channel
*   **WHEN** the publisher widget dispatches an event
*   **THEN** the broker MUST deliver the event to the fast-processing subscriber without waiting for the slow-processing subscriber to complete its event execution.

### Scenario 3: Event Channel Cleanup and Memory Leak Prevention
*   **GIVEN** a temporary screen layout containing three widgets subscribed to `"temp:channel"`
*   **WHEN** the screen layout is dismissed and the unsubscribe command is triggered for all three widgets
*   **THEN** the broker MUST remove all subscription records corresponding to those widgets
*   **AND** any subsequent publish calls to `"temp:channel"` SHALL NOT broadcast messages to the destroyed widget handlers
*   **AND** the memory structures allocated for the subscriptions MUST be freed.
