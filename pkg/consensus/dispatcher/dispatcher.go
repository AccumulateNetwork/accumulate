// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package dispatcher provides inter-partition message routing for DAG-BFT consensus.
// It replaces CometBFT-based dispatching with libp2p-based communication.
package dispatcher

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TopicDispatch is the topic pattern for inter-partition message dispatch.
// The %s placeholder is replaced with the partition ID.
const TopicDispatch = "acc/%s/dispatch"

// Default configuration values.
const (
	DefaultEnvelopeChannelSize = 500
	DefaultSendTimeout         = 10 * time.Second
	DefaultAckTimeout          = 5 * time.Second
)

// Dispatcher routes synthetic transactions between partitions using libp2p.
// It implements the execute.Dispatcher interface for integration with the
// block executor.
type Dispatcher struct {
	host      host.Host
	pubsub    *pubsub.PubSub
	router    routing.Router
	partition string

	// Topics for other partitions
	mu     sync.RWMutex
	topics map[string]*pubsub.Topic

	// Queue of envelopes to send per partition
	queueMu sync.Mutex
	queue   map[string][]*messaging.Envelope

	// Inbound message channel
	inbound chan *messaging.Envelope

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed bool

	// Discovery
	discovery *PartitionDiscovery

	// Options
	sendTimeout time.Duration
}

// DispatcherOptions configures a Dispatcher.
type DispatcherOptions struct {
	// EnvelopeChannelSize is the buffer size for the inbound envelope channel.
	// Defaults to DefaultEnvelopeChannelSize.
	EnvelopeChannelSize int

	// SendTimeout is the timeout for sending messages.
	// Defaults to DefaultSendTimeout.
	SendTimeout time.Duration
}

var _ execute.Dispatcher = (*Dispatcher)(nil)

// NewDispatcher creates a new Dispatcher for inter-partition communication.
func NewDispatcher(h host.Host, ps *pubsub.PubSub, router routing.Router, partition string) (*Dispatcher, error) {
	return NewDispatcherWithOptions(h, ps, router, partition, DispatcherOptions{})
}

// NewDispatcherWithOptions creates a new Dispatcher with custom options.
func NewDispatcherWithOptions(h host.Host, ps *pubsub.PubSub, router routing.Router, partition string, opts DispatcherOptions) (*Dispatcher, error) {
	if h == nil {
		return nil, fmt.Errorf("host is nil")
	}
	if ps == nil {
		return nil, fmt.Errorf("pubsub is nil")
	}
	if router == nil {
		return nil, fmt.Errorf("router is nil")
	}
	if partition == "" {
		return nil, fmt.Errorf("partition is empty")
	}

	// Apply defaults
	if opts.EnvelopeChannelSize <= 0 {
		opts.EnvelopeChannelSize = DefaultEnvelopeChannelSize
	}
	if opts.SendTimeout <= 0 {
		opts.SendTimeout = DefaultSendTimeout
	}

	d := &Dispatcher{
		host:        h,
		pubsub:      ps,
		router:      router,
		partition:   strings.ToLower(partition),
		topics:      make(map[string]*pubsub.Topic),
		queue:       make(map[string][]*messaging.Envelope),
		inbound:     make(chan *messaging.Envelope, opts.EnvelopeChannelSize),
		sendTimeout: opts.SendTimeout,
	}

	// Create partition discovery
	d.discovery = NewPartitionDiscovery(h, partition)

	return d, nil
}

// Start begins message processing and joins the dispatch topic for this partition.
func (d *Dispatcher) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("dispatcher is closed")
	}
	if d.ctx != nil {
		d.mu.Unlock()
		return fmt.Errorf("dispatcher already started")
	}
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.mu.Unlock()

	// Start discovery
	if err := d.discovery.Start(ctx); err != nil {
		return fmt.Errorf("start discovery: %w", err)
	}

	// Join our own partition's dispatch topic to receive messages
	topicName := fmt.Sprintf(TopicDispatch, d.partition)
	topic, err := d.pubsub.Join(topicName)
	if err != nil {
		return fmt.Errorf("join topic %s: %w", topicName, err)
	}

	d.mu.Lock()
	d.topics[d.partition] = topic
	d.mu.Unlock()

	// Subscribe to receive messages
	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe to topic %s: %w", topicName, err)
	}

	// Start message handler
	d.wg.Add(1)
	go d.handleInbound(sub)

	return nil
}

// Close stops the dispatcher and releases resources.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Unlock()

	// Wait for handlers to stop
	d.wg.Wait()

	// Close discovery
	if d.discovery != nil {
		d.discovery.Close()
	}

	// Close topics
	d.mu.Lock()
	for _, topic := range d.topics {
		topic.Close()
	}
	d.topics = make(map[string]*pubsub.Topic)
	d.mu.Unlock()

	// Close inbound channel
	close(d.inbound)
}

// Submit queues an envelope for dispatch to the target partition.
// The destination is determined by routing the given URL.
func (d *Dispatcher) Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error {
	// Validate envelope
	_, err := envelope.Normalize()
	if err != nil {
		return fmt.Errorf("normalize envelope: %w", err)
	}

	// Route to get target partition
	partition, err := d.router.RouteAccount(dest)
	if err != nil {
		return fmt.Errorf("route account: %w", err)
	}

	// Normalize partition ID
	partition = strings.ToLower(partition)

	// Queue the envelope
	d.queueMu.Lock()
	d.queue[partition] = append(d.queue[partition], envelope.Copy())
	d.queueMu.Unlock()

	return nil
}

// Send dispatches all queued envelopes asynchronously.
// Returns a channel that receives any errors that occur during sending.
func (d *Dispatcher) Send(ctx context.Context) <-chan error {
	d.queueMu.Lock()
	queue := d.queue
	d.queue = make(map[string][]*messaging.Envelope, len(queue))
	d.queueMu.Unlock()

	errs := make(chan error)
	if len(queue) == 0 {
		close(errs)
		return errs
	}

	// Send asynchronously
	go d.sendAll(ctx, queue, errs)

	return errs
}

// Subscribe returns a channel that receives envelopes from other partitions.
func (d *Dispatcher) Subscribe() <-chan *messaging.Envelope {
	return d.inbound
}

// sendAll sends all queued envelopes to their target partitions.
func (d *Dispatcher) sendAll(ctx context.Context, queue map[string][]*messaging.Envelope, errs chan<- error) {
	defer close(errs)

	ctx, cancel := context.WithTimeout(ctx, d.sendTimeout)
	defer cancel()

	for partition, envelopes := range queue {
		// Get or create topic for this partition
		topic, err := d.getOrCreateTopic(partition)
		if err != nil {
			errs <- fmt.Errorf("get topic for %s: %w", partition, err)
			continue
		}

		// Send each envelope
		for _, env := range envelopes {
			if err := d.publishEnvelope(ctx, topic, env); err != nil {
				slog.Warn("Failed to dispatch envelope",
					"partition", partition,
					"error", err)
				errs <- fmt.Errorf("dispatch to %s: %w", partition, err)
			}
		}
	}
}

// getOrCreateTopic returns the topic for a partition, creating it if needed.
func (d *Dispatcher) getOrCreateTopic(partition string) (*pubsub.Topic, error) {
	d.mu.RLock()
	topic, ok := d.topics[partition]
	d.mu.RUnlock()
	if ok {
		return topic, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check after acquiring write lock
	if topic, ok := d.topics[partition]; ok {
		return topic, nil
	}

	// Create new topic
	topicName := fmt.Sprintf(TopicDispatch, partition)
	topic, err := d.pubsub.Join(topicName)
	if err != nil {
		return nil, fmt.Errorf("join topic %s: %w", topicName, err)
	}

	d.topics[partition] = topic
	return topic, nil
}

// publishEnvelope serializes and publishes an envelope to a topic.
func (d *Dispatcher) publishEnvelope(ctx context.Context, topic *pubsub.Topic, env *messaging.Envelope) error {
	data, err := env.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Wrap with message header containing source partition
	msg := d.wrapMessage(data)

	return topic.Publish(ctx, msg)
}

// wrapMessage adds a header with source partition info.
func (d *Dispatcher) wrapMessage(data []byte) []byte {
	// Message format:
	// - 1 byte: version (0x01)
	// - 1 byte: source partition length
	// - N bytes: source partition
	// - 4 bytes: payload length (big endian)
	// - M bytes: payload

	partitionBytes := []byte(d.partition)
	result := make([]byte, 1+1+len(partitionBytes)+4+len(data))

	result[0] = 0x01 // version
	result[1] = byte(len(partitionBytes))
	copy(result[2:], partitionBytes)
	binary.BigEndian.PutUint32(result[2+len(partitionBytes):], uint32(len(data)))
	copy(result[2+len(partitionBytes)+4:], data)

	return result
}

// unwrapMessage extracts the source partition and payload from a message.
func (d *Dispatcher) unwrapMessage(data []byte) (source string, payload []byte, err error) {
	if len(data) < 6 {
		return "", nil, fmt.Errorf("message too short")
	}

	if data[0] != 0x01 {
		return "", nil, fmt.Errorf("unsupported message version: %d", data[0])
	}

	partitionLen := int(data[1])
	if len(data) < 2+partitionLen+4 {
		return "", nil, fmt.Errorf("message truncated")
	}

	source = string(data[2 : 2+partitionLen])
	payloadLen := binary.BigEndian.Uint32(data[2+partitionLen:])

	if len(data) < 2+partitionLen+4+int(payloadLen) {
		return "", nil, fmt.Errorf("payload truncated")
	}

	payload = data[2+partitionLen+4 : 2+partitionLen+4+int(payloadLen)]
	return source, payload, nil
}

// handleInbound processes incoming messages from the subscription.
func (d *Dispatcher) handleInbound(sub *pubsub.Subscription) {
	defer d.wg.Done()

	for {
		msg, err := sub.Next(d.ctx)
		if err != nil {
			if d.ctx.Err() != nil {
				return
			}
			slog.Error("Error reading from dispatch subscription",
				"error", err,
				"partition", d.partition)
			return
		}

		// Skip messages from ourselves
		if msg.ReceivedFrom == d.host.ID() {
			continue
		}

		// Unwrap and process
		source, payload, err := d.unwrapMessage(msg.Data)
		if err != nil {
			slog.Debug("Invalid dispatch message",
				"error", err,
				"partition", d.partition,
				"from", msg.ReceivedFrom)
			continue
		}

		// Deserialize envelope
		env := new(messaging.Envelope)
		if err := env.UnmarshalBinary(payload); err != nil {
			slog.Debug("Failed to unmarshal envelope",
				"error", err,
				"source", source,
				"partition", d.partition)
			continue
		}

		// Queue for processing
		select {
		case d.inbound <- env:
		default:
			slog.Warn("Inbound envelope channel full, dropping message",
				"source", source,
				"partition", d.partition)
		}
	}
}

// PartitionDiscovery discovers nodes in other partitions.
type PartitionDiscovery struct {
	host      host.Host
	partition string

	mu    sync.RWMutex
	peers map[string][]peer.ID // partition -> peers

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPartitionDiscovery creates a new PartitionDiscovery instance.
func NewPartitionDiscovery(h host.Host, partition string) *PartitionDiscovery {
	return &PartitionDiscovery{
		host:      h,
		partition: strings.ToLower(partition),
		peers:     make(map[string][]peer.ID),
	}
}

// Start begins partition discovery.
func (pd *PartitionDiscovery) Start(ctx context.Context) error {
	pd.mu.Lock()
	if pd.ctx != nil {
		pd.mu.Unlock()
		return fmt.Errorf("discovery already started")
	}
	pd.ctx, pd.cancel = context.WithCancel(ctx)
	pd.mu.Unlock()

	return nil
}

// Close stops partition discovery.
func (pd *PartitionDiscovery) Close() {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	if pd.cancel != nil {
		pd.cancel()
	}
}

// AddPeer registers a peer as belonging to a partition.
func (pd *PartitionDiscovery) AddPeer(partition string, peerID peer.ID) {
	partition = strings.ToLower(partition)

	pd.mu.Lock()
	defer pd.mu.Unlock()

	// Check if already present
	for _, p := range pd.peers[partition] {
		if p == peerID {
			return
		}
	}

	pd.peers[partition] = append(pd.peers[partition], peerID)
}

// RemovePeer removes a peer from a partition.
func (pd *PartitionDiscovery) RemovePeer(partition string, peerID peer.ID) {
	partition = strings.ToLower(partition)

	pd.mu.Lock()
	defer pd.mu.Unlock()

	peers := pd.peers[partition]
	for i, p := range peers {
		if p == peerID {
			pd.peers[partition] = append(peers[:i], peers[i+1:]...)
			return
		}
	}
}

// GetPeers returns the known peers for a partition.
func (pd *PartitionDiscovery) GetPeers(partition string) []peer.ID {
	partition = strings.ToLower(partition)

	pd.mu.RLock()
	defer pd.mu.RUnlock()

	peers := pd.peers[partition]
	if len(peers) == 0 {
		return nil
	}

	result := make([]peer.ID, len(peers))
	copy(result, peers)
	return result
}

// GetAllPartitions returns all known partitions.
func (pd *PartitionDiscovery) GetAllPartitions() []string {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	result := make([]string, 0, len(pd.peers))
	for partition := range pd.peers {
		result = append(result, partition)
	}
	return result
}
