// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

// TopicAnchor is the topic pattern for anchor messages.
// The %s placeholder is replaced with the partition ID.
const TopicAnchor = "acc/%s/anchors"

// Anchor represents a block anchor for cross-partition coordination.
type Anchor struct {
	// SourcePartition is the partition that produced this anchor.
	SourcePartition string

	// BlockIndex is the index of the block being anchored.
	BlockIndex uint64

	// BlockHash is the hash of the anchored block.
	BlockHash [32]byte

	// StateRoot is the state root after the block.
	StateRoot [32]byte

	// Timestamp is when the anchor was produced.
	Timestamp time.Time
}

// AnchorAck is an acknowledgment of an anchor.
type AnchorAck struct {
	// AnchorPartition is the partition that produced the anchor.
	AnchorPartition string

	// AckPartition is the partition sending the acknowledgment.
	AckPartition string

	// BlockIndex is the index of the acknowledged block.
	BlockIndex uint64

	// BlockHash is the hash of the acknowledged block.
	BlockHash [32]byte
}

// AnchorHandler processes anchor messages for cross-partition coordination.
// It handles both sending anchors to the directory and receiving anchor
// acknowledgments from other partitions.
type AnchorHandler struct {
	host      host.Host
	pubsub    *pubsub.PubSub
	partition string

	// Topic for anchor messages
	anchorTopic *pubsub.Topic

	// Channels for received anchors and acks
	anchors chan *Anchor
	acks    chan *AnchorAck

	// Pending anchors awaiting acknowledgment
	pendingMu sync.RWMutex
	pending   map[uint64]*pendingAnchor

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed bool
	mu     sync.Mutex
}

type pendingAnchor struct {
	anchor *Anchor
	acks   map[string]bool // partition -> received
}

// AnchorHandlerOptions configures an AnchorHandler.
type AnchorHandlerOptions struct {
	// AnchorChannelSize is the buffer size for the anchor channel.
	AnchorChannelSize int

	// AckChannelSize is the buffer size for the ack channel.
	AckChannelSize int
}

// DefaultAnchorChannelSize is the default buffer size for anchor channels.
const DefaultAnchorChannelSize = 100

// NewAnchorHandler creates a new AnchorHandler for the given partition.
func NewAnchorHandler(h host.Host, ps *pubsub.PubSub, partition string) (*AnchorHandler, error) {
	return NewAnchorHandlerWithOptions(h, ps, partition, AnchorHandlerOptions{})
}

// NewAnchorHandlerWithOptions creates a new AnchorHandler with custom options.
func NewAnchorHandlerWithOptions(h host.Host, ps *pubsub.PubSub, partition string, opts AnchorHandlerOptions) (*AnchorHandler, error) {
	if h == nil {
		return nil, fmt.Errorf("host is nil")
	}
	if ps == nil {
		return nil, fmt.Errorf("pubsub is nil")
	}
	if partition == "" {
		return nil, fmt.Errorf("partition is empty")
	}

	// Apply defaults
	if opts.AnchorChannelSize <= 0 {
		opts.AnchorChannelSize = DefaultAnchorChannelSize
	}
	if opts.AckChannelSize <= 0 {
		opts.AckChannelSize = DefaultAnchorChannelSize
	}

	return &AnchorHandler{
		host:      h,
		pubsub:    ps,
		partition: strings.ToLower(partition),
		anchors:   make(chan *Anchor, opts.AnchorChannelSize),
		acks:      make(chan *AnchorAck, opts.AckChannelSize),
		pending:   make(map[uint64]*pendingAnchor),
	}, nil
}

// Start begins anchor message processing.
func (ah *AnchorHandler) Start(ctx context.Context) error {
	ah.mu.Lock()
	if ah.closed {
		ah.mu.Unlock()
		return fmt.Errorf("anchor handler is closed")
	}
	if ah.ctx != nil {
		ah.mu.Unlock()
		return fmt.Errorf("anchor handler already started")
	}
	ah.ctx, ah.cancel = context.WithCancel(ctx)
	ah.mu.Unlock()

	// Join the anchor topic
	topicName := fmt.Sprintf(TopicAnchor, ah.partition)
	topic, err := ah.pubsub.Join(topicName)
	if err != nil {
		return fmt.Errorf("join anchor topic: %w", err)
	}
	ah.anchorTopic = topic

	// Subscribe to receive messages
	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe to anchor topic: %w", err)
	}

	// Start message handler
	ah.wg.Add(1)
	go ah.handleMessages(sub)

	return nil
}

// Close stops the anchor handler.
func (ah *AnchorHandler) Close() error {
	ah.mu.Lock()
	if ah.closed {
		ah.mu.Unlock()
		return nil
	}
	ah.closed = true
	if ah.cancel != nil {
		ah.cancel()
	}
	ah.mu.Unlock()

	// Wait for handlers to stop
	ah.wg.Wait()

	// Close topic
	if ah.anchorTopic != nil {
		ah.anchorTopic.Close()
	}

	// Close channels
	close(ah.anchors)
	close(ah.acks)

	return nil
}

// BroadcastAnchor sends an anchor to the network.
func (ah *AnchorHandler) BroadcastAnchor(ctx context.Context, anchor *Anchor) error {
	if ah.anchorTopic == nil {
		return fmt.Errorf("anchor handler not started")
	}

	data, err := marshalAnchor(anchor)
	if err != nil {
		return fmt.Errorf("marshal anchor: %w", err)
	}

	// Wrap as anchor message (type 0x01)
	msg := make([]byte, 1+len(data))
	msg[0] = 0x01 // anchor type
	copy(msg[1:], data)

	// Track pending anchor
	ah.pendingMu.Lock()
	ah.pending[anchor.BlockIndex] = &pendingAnchor{
		anchor: anchor,
		acks:   make(map[string]bool),
	}
	ah.pendingMu.Unlock()

	return ah.anchorTopic.Publish(ctx, msg)
}

// BroadcastAck sends an anchor acknowledgment to the network.
func (ah *AnchorHandler) BroadcastAck(ctx context.Context, ack *AnchorAck) error {
	if ah.anchorTopic == nil {
		return fmt.Errorf("anchor handler not started")
	}

	data, err := marshalAnchorAck(ack)
	if err != nil {
		return fmt.Errorf("marshal ack: %w", err)
	}

	// Wrap as ack message (type 0x02)
	msg := make([]byte, 1+len(data))
	msg[0] = 0x02 // ack type
	copy(msg[1:], data)

	return ah.anchorTopic.Publish(ctx, msg)
}

// SubscribeAnchors returns a channel that receives anchors from other partitions.
func (ah *AnchorHandler) SubscribeAnchors() <-chan *Anchor {
	return ah.anchors
}

// SubscribeAcks returns a channel that receives anchor acknowledgments.
func (ah *AnchorHandler) SubscribeAcks() <-chan *AnchorAck {
	return ah.acks
}

// GetPendingAnchor returns a pending anchor by block index.
func (ah *AnchorHandler) GetPendingAnchor(blockIndex uint64) (*Anchor, bool) {
	ah.pendingMu.RLock()
	defer ah.pendingMu.RUnlock()

	pa, ok := ah.pending[blockIndex]
	if !ok {
		return nil, false
	}
	return pa.anchor, true
}

// RecordAck records an acknowledgment for a pending anchor.
// Returns true if this completes the anchor (all required acks received).
func (ah *AnchorHandler) RecordAck(blockIndex uint64, partition string) bool {
	ah.pendingMu.Lock()
	defer ah.pendingMu.Unlock()

	pa, ok := ah.pending[blockIndex]
	if !ok {
		return false
	}

	pa.acks[strings.ToLower(partition)] = true
	return false // Caller determines completeness based on quorum
}

// ClearPendingAnchor removes a pending anchor after it's fully acknowledged.
func (ah *AnchorHandler) ClearPendingAnchor(blockIndex uint64) {
	ah.pendingMu.Lock()
	defer ah.pendingMu.Unlock()

	delete(ah.pending, blockIndex)
}

// handleMessages processes incoming anchor messages.
func (ah *AnchorHandler) handleMessages(sub *pubsub.Subscription) {
	defer ah.wg.Done()

	for {
		msg, err := sub.Next(ah.ctx)
		if err != nil {
			if ah.ctx.Err() != nil {
				return
			}
			slog.Error("Error reading from anchor subscription",
				"error", err,
				"partition", ah.partition)
			return
		}

		// Skip messages from ourselves
		if msg.ReceivedFrom == ah.host.ID() {
			continue
		}

		if len(msg.Data) < 1 {
			continue
		}

		msgType := msg.Data[0]
		payload := msg.Data[1:]

		switch msgType {
		case 0x01: // Anchor
			anchor, err := unmarshalAnchor(payload)
			if err != nil {
				slog.Debug("Invalid anchor message",
					"error", err,
					"partition", ah.partition)
				continue
			}

			select {
			case ah.anchors <- anchor:
			default:
				slog.Warn("Anchor channel full, dropping message",
					"partition", ah.partition)
			}

		case 0x02: // Ack
			ack, err := unmarshalAnchorAck(payload)
			if err != nil {
				slog.Debug("Invalid anchor ack message",
					"error", err,
					"partition", ah.partition)
				continue
			}

			select {
			case ah.acks <- ack:
			default:
				slog.Warn("Anchor ack channel full, dropping message",
					"partition", ah.partition)
			}

		default:
			slog.Debug("Unknown anchor message type",
				"type", msgType,
				"partition", ah.partition)
		}
	}
}

// marshalAnchor serializes an anchor.
func marshalAnchor(a *Anchor) ([]byte, error) {
	// Simple binary format:
	// - 1 byte: source partition length
	// - N bytes: source partition
	// - 8 bytes: block index (big endian)
	// - 32 bytes: block hash
	// - 32 bytes: state root
	// - 8 bytes: timestamp (unix nano)

	src := []byte(a.SourcePartition)
	data := make([]byte, 1+len(src)+8+32+32+8)

	offset := 0
	data[offset] = byte(len(src))
	offset++

	copy(data[offset:], src)
	offset += len(src)

	putUint64BE(data[offset:], a.BlockIndex)
	offset += 8

	copy(data[offset:], a.BlockHash[:])
	offset += 32

	copy(data[offset:], a.StateRoot[:])
	offset += 32

	putUint64BE(data[offset:], uint64(a.Timestamp.UnixNano()))

	return data, nil
}

// unmarshalAnchor deserializes an anchor.
func unmarshalAnchor(data []byte) (*Anchor, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("data too short")
	}

	srcLen := int(data[0])
	minLen := 1 + srcLen + 8 + 32 + 32 + 8
	if len(data) < minLen {
		return nil, fmt.Errorf("data truncated")
	}

	offset := 1
	a := &Anchor{
		SourcePartition: string(data[offset : offset+srcLen]),
	}
	offset += srcLen

	a.BlockIndex = getUint64BE(data[offset:])
	offset += 8

	copy(a.BlockHash[:], data[offset:offset+32])
	offset += 32

	copy(a.StateRoot[:], data[offset:offset+32])
	offset += 32

	a.Timestamp = time.Unix(0, int64(getUint64BE(data[offset:])))

	return a, nil
}

// marshalAnchorAck serializes an anchor ack.
func marshalAnchorAck(ack *AnchorAck) ([]byte, error) {
	// Simple binary format:
	// - 1 byte: anchor partition length
	// - N bytes: anchor partition
	// - 1 byte: ack partition length
	// - M bytes: ack partition
	// - 8 bytes: block index (big endian)
	// - 32 bytes: block hash

	ap := []byte(ack.AnchorPartition)
	akp := []byte(ack.AckPartition)
	data := make([]byte, 1+len(ap)+1+len(akp)+8+32)

	offset := 0
	data[offset] = byte(len(ap))
	offset++

	copy(data[offset:], ap)
	offset += len(ap)

	data[offset] = byte(len(akp))
	offset++

	copy(data[offset:], akp)
	offset += len(akp)

	putUint64BE(data[offset:], ack.BlockIndex)
	offset += 8

	copy(data[offset:], ack.BlockHash[:])

	return data, nil
}

// unmarshalAnchorAck deserializes an anchor ack.
func unmarshalAnchorAck(data []byte) (*AnchorAck, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("data too short")
	}

	apLen := int(data[0])
	if len(data) < 1+apLen+1 {
		return nil, fmt.Errorf("data truncated")
	}

	offset := 1
	anchorPartition := string(data[offset : offset+apLen])
	offset += apLen

	akpLen := int(data[offset])
	offset++

	minLen := offset + akpLen + 8 + 32
	if len(data) < minLen {
		return nil, fmt.Errorf("data truncated")
	}

	ack := &AnchorAck{
		AnchorPartition: anchorPartition,
		AckPartition:    string(data[offset : offset+akpLen]),
	}
	offset += akpLen

	ack.BlockIndex = getUint64BE(data[offset:])
	offset += 8

	copy(ack.BlockHash[:], data[offset:offset+32])

	return ack, nil
}

// Helper functions for big-endian encoding
func putUint64BE(b []byte, v uint64) {
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
}

func getUint64BE(b []byte) uint64 {
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}
