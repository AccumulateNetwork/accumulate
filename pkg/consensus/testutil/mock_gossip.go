// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"context"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
)

// MockPubSub implements pubsub functionality for testing.
type MockPubSub struct {
	mu       sync.RWMutex
	topics   map[string]*MockTopic
	peers    []peer.ID
	localID  peer.ID
	registry *MockPubSubRegistry
}

// MockPubSubRegistry connects multiple MockPubSub instances for cross-node messaging.
type MockPubSubRegistry struct {
	mu        sync.RWMutex
	instances map[peer.ID]*MockPubSub
}

// NewMockPubSubRegistry creates a new registry for connecting MockPubSub instances.
func NewMockPubSubRegistry() *MockPubSubRegistry {
	return &MockPubSubRegistry{
		instances: make(map[peer.ID]*MockPubSub),
	}
}

// Register registers a MockPubSub instance.
func (r *MockPubSubRegistry) Register(ps *MockPubSub) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[ps.localID] = ps
}

// Unregister removes a MockPubSub instance.
func (r *MockPubSubRegistry) Unregister(id peer.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, id)
}

// Broadcast sends a message to all instances subscribed to a topic.
func (r *MockPubSubRegistry) Broadcast(from peer.ID, topic string, data []byte) {
	r.mu.RLock()
	instances := make([]*MockPubSub, 0, len(r.instances))
	for _, ps := range r.instances {
		instances = append(instances, ps)
	}
	r.mu.RUnlock()

	for _, ps := range instances {
		ps.deliverMessage(from, topic, data)
	}
}

// NewMockPubSub creates a new mock pubsub.
func NewMockPubSub(localID peer.ID) *MockPubSub {
	return &MockPubSub{
		topics:  make(map[string]*MockTopic),
		localID: localID,
	}
}

// NewMockPubSubWithRegistry creates a new mock pubsub connected to a registry.
func NewMockPubSubWithRegistry(localID peer.ID, registry *MockPubSubRegistry) *MockPubSub {
	ps := &MockPubSub{
		topics:   make(map[string]*MockTopic),
		localID:  localID,
		registry: registry,
	}
	registry.Register(ps)
	return ps
}

// Join joins a topic and returns a handle.
func (ps *MockPubSub) Join(topic string) (*MockTopic, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if t, ok := ps.topics[topic]; ok {
		return t, nil
	}

	t := &MockTopic{
		name:     topic,
		pubsub:   ps,
		publish:  make(chan []byte, 100),
		handlers: make([]func([]byte), 0),
	}
	ps.topics[topic] = t
	return t, nil
}

// GetTopics returns all joined topics.
func (ps *MockPubSub) GetTopics() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	topics := make([]string, 0, len(ps.topics))
	for name := range ps.topics {
		topics = append(topics, name)
	}
	return topics
}

// ListPeers returns the peers subscribed to a topic.
func (ps *MockPubSub) ListPeers(topic string) []peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.peers
}

// AddPeer adds a peer to the mock pubsub.
func (ps *MockPubSub) AddPeer(p peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers = append(ps.peers, p)
}

// RemovePeer removes a peer from the mock pubsub.
func (ps *MockPubSub) RemovePeer(p peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i, existing := range ps.peers {
		if existing == p {
			ps.peers = append(ps.peers[:i], ps.peers[i+1:]...)
			break
		}
	}
}

// deliverMessage delivers a message to local subscriptions.
func (ps *MockPubSub) deliverMessage(from peer.ID, topicName string, data []byte) {
	ps.mu.RLock()
	topic, ok := ps.topics[topicName]
	ps.mu.RUnlock()

	if !ok {
		return
	}

	topic.deliverMessage(from, data)
}

// Close closes the pubsub.
func (ps *MockPubSub) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.registry != nil {
		ps.registry.Unregister(ps.localID)
	}

	for _, topic := range ps.topics {
		topic.Close()
	}
	return nil
}

// MockTopic represents a pubsub topic.
type MockTopic struct {
	mu       sync.RWMutex
	name     string
	pubsub   *MockPubSub
	subs     []*MockSubscription
	publish  chan []byte
	handlers []func([]byte)
	closed   bool
}

// String returns the topic name.
func (t *MockTopic) String() string {
	return t.name
}

// Publish publishes a message to the topic.
func (t *MockTopic) Publish(ctx context.Context, data []byte) error {
	t.mu.RLock()
	closed := t.closed
	pubsub := t.pubsub
	t.mu.RUnlock()

	if closed {
		return context.Canceled
	}

	// If connected to a registry, broadcast to all instances
	if pubsub.registry != nil {
		pubsub.registry.Broadcast(pubsub.localID, t.name, data)
	} else {
		// Local delivery only
		t.deliverMessage(pubsub.localID, data)
	}

	return nil
}

// Subscribe creates a new subscription to the topic.
func (t *MockTopic) Subscribe(opts ...pubsub.SubOpt) (*MockSubscription, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	sub := &MockSubscription{
		topic:   t,
		msgChan: make(chan *pubsub.Message, 100),
		cancel:  cancel,
		ctx:     ctx,
	}
	t.subs = append(t.subs, sub)
	return sub, nil
}

// deliverMessage delivers a message to all subscriptions.
func (t *MockTopic) deliverMessage(from peer.ID, data []byte) {
	t.mu.RLock()
	subs := make([]*MockSubscription, len(t.subs))
	copy(subs, t.subs)
	handlers := make([]func([]byte), len(t.handlers))
	copy(handlers, t.handlers)
	t.mu.RUnlock()

	topicName := t.name
	msg := &pubsub.Message{
		Message: &pb.Message{
			Data:  data,
			From:  []byte(from),
			Topic: &topicName,
		},
		ReceivedFrom: from,
	}

	// Deliver to subscriptions
	for _, sub := range subs {
		select {
		case sub.msgChan <- msg:
		default:
			// Channel full, drop message
		}
	}

	// Call registered handlers
	for _, handler := range handlers {
		go handler(data)
	}
}

// RegisterHandler registers a handler for messages on this topic.
func (t *MockTopic) RegisterHandler(handler func([]byte)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handlers = append(t.handlers, handler)
}

// Close closes the topic.
func (t *MockTopic) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	close(t.publish)

	for _, sub := range t.subs {
		sub.Cancel()
	}
	return nil
}

// ListPeers returns the peers subscribed to this topic.
func (t *MockTopic) ListPeers() []peer.ID {
	if t.pubsub == nil {
		return nil
	}
	return t.pubsub.ListPeers(t.name)
}

// MockSubscription represents a subscription to a topic.
type MockSubscription struct {
	mu      sync.Mutex
	topic   *MockTopic
	msgChan chan *pubsub.Message
	cancel  context.CancelFunc
	ctx     context.Context
}

// Next returns the next message from the subscription.
func (s *MockSubscription) Next(ctx context.Context) (*pubsub.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case msg, ok := <-s.msgChan:
		if !ok {
			return nil, context.Canceled
		}
		return msg, nil
	}
}

// Cancel cancels the subscription.
func (s *MockSubscription) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel()
}

// Topic returns the subscription's topic.
func (s *MockSubscription) Topic() string {
	if s.topic == nil {
		return ""
	}
	return s.topic.name
}
