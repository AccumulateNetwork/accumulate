// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package gossip provides the network layer for DAG consensus message exchange.
// It builds on libp2p GossipSub for reliable broadcast and request/response
// protocols for fetching missing data.
package gossip

import (
	"context"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// Topic name patterns for consensus messages.
// The %s placeholder is replaced with the partition ID.
const (
	TopicBatches  = "acc/%s/consensus/batches"
	TopicHeaders  = "acc/%s/consensus/headers"
	TopicVotes    = "acc/%s/consensus/votes"
	TopicCerts    = "acc/%s/consensus/certs"
	TopicCertSync = "acc/%s/consensus/cert-sync"
)

// topicNames returns the list of all topic patterns.
func topicNames() []string {
	return []string{
		TopicBatches,
		TopicHeaders,
		TopicVotes,
		TopicCerts,
		TopicCertSync,
	}
}

// TopicManager manages GossipSub topics for a partition.
// It handles joining topics, creating subscriptions, and cleanup.
type TopicManager struct {
	pubsub    *pubsub.PubSub
	partition string

	mu     sync.RWMutex
	topics map[string]*pubsub.Topic
	subs   map[string]*pubsub.Subscription
}

// NewTopicManager creates a new TopicManager for the given partition.
func NewTopicManager(ps *pubsub.PubSub, partition string) (*TopicManager, error) {
	if ps == nil {
		return nil, fmt.Errorf("pubsub is nil")
	}
	if partition == "" {
		return nil, fmt.Errorf("partition is empty")
	}

	return &TopicManager{
		pubsub:    ps,
		partition: partition,
		topics:    make(map[string]*pubsub.Topic),
		subs:      make(map[string]*pubsub.Subscription),
	}, nil
}

// JoinAll joins all consensus topics for this partition.
// It creates subscriptions for each topic to enable receiving messages.
func (t *TopicManager) JoinAll(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, pattern := range topicNames() {
		topicName := fmt.Sprintf(pattern, t.partition)

		// Join the topic
		topic, err := t.pubsub.Join(topicName)
		if err != nil {
			// Clean up any topics we already joined
			t.closeLockedNoWait()
			return fmt.Errorf("join topic %s: %w", topicName, err)
		}
		t.topics[pattern] = topic

		// Subscribe to receive messages
		sub, err := topic.Subscribe()
		if err != nil {
			// Clean up
			t.closeLockedNoWait()
			return fmt.Errorf("subscribe to topic %s: %w", topicName, err)
		}
		t.subs[pattern] = sub
	}

	return nil
}

// GetTopic returns the topic for the given pattern.
// The pattern should be one of the Topic* constants.
func (t *TopicManager) GetTopic(pattern string) *pubsub.Topic {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.topics[pattern]
}

// GetSubscription returns the subscription for the given pattern.
// The pattern should be one of the Topic* constants.
func (t *TopicManager) GetSubscription(pattern string) *pubsub.Subscription {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.subs[pattern]
}

// TopicName returns the full topic name for a pattern.
func (t *TopicManager) TopicName(pattern string) string {
	return fmt.Sprintf(pattern, t.partition)
}

// Partition returns the partition this TopicManager is for.
func (t *TopicManager) Partition() string {
	return t.partition
}

// Close closes all topics and subscriptions.
func (t *TopicManager) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeLockedNoWait()
}

// closeLockedNoWait closes all topics without waiting.
// Must be called with t.mu held.
func (t *TopicManager) closeLockedNoWait() error {
	var firstErr error

	// Cancel all subscriptions first
	for pattern, sub := range t.subs {
		sub.Cancel()
		delete(t.subs, pattern)
	}

	// Close all topics
	for pattern, topic := range t.topics {
		if err := topic.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(t.topics, pattern)
	}

	return firstErr
}
