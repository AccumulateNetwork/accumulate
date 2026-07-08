// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default channel buffer sizes for message channels.
// Sized for high throughput (~30k+ TPS) to avoid dropped messages.
const (
	DefaultBatchChannelSize       = 1000
	DefaultHeaderChannelSize      = 500  // Increased from 100 for high throughput
	DefaultVoteChannelSize        = 1000
	DefaultCertificateChannelSize = 1000 // Increased from 100 for high throughput
	DefaultCertSyncChannelSize    = 500  // Increased from 200 to handle sustained load
)

// GossipLayer provides reliable broadcast for consensus messages.
// It wraps libp2p GossipSub to provide typed publish/subscribe for
// batches, headers, votes, and certificates.
type GossipLayer struct {
	host      host.Host
	pubsub    *pubsub.PubSub
	topics    *TopicManager
	partition string

	// Channels for received messages
	batches      chan *types.Batch
	headers      chan *types.Header
	votes        chan *types.Vote
	certs        chan *types.Certificate
	syncRequests chan *CertSyncRequest
	syncResponses chan *CertSyncResponse

	// Rate limiting
	rateLimiter *PeerRateLimiter

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	closed bool
}

// GossipLayerOptions configures a GossipLayer.
type GossipLayerOptions struct {
	// BatchChannelSize is the buffer size for the batch channel.
	// Defaults to DefaultBatchChannelSize.
	BatchChannelSize int

	// HeaderChannelSize is the buffer size for the header channel.
	// Defaults to DefaultHeaderChannelSize.
	HeaderChannelSize int

	// VoteChannelSize is the buffer size for the vote channel.
	// Defaults to DefaultVoteChannelSize.
	VoteChannelSize int

	// CertificateChannelSize is the buffer size for the certificate channel.
	// Defaults to DefaultCertificateChannelSize.
	CertificateChannelSize int

	// CertSyncChannelSize is the buffer size for the cert sync channels.
	// Defaults to DefaultCertSyncChannelSize.
	CertSyncChannelSize int
}

// NewGossipLayer creates a new GossipLayer for the given partition.
func NewGossipLayer(h host.Host, ps *pubsub.PubSub, partition string) (*GossipLayer, error) {
	return NewGossipLayerWithOptions(h, ps, partition, GossipLayerOptions{})
}

// NewGossipLayerWithOptions creates a new GossipLayer with custom options.
func NewGossipLayerWithOptions(h host.Host, ps *pubsub.PubSub, partition string, opts GossipLayerOptions) (*GossipLayer, error) {
	if h == nil {
		return nil, fmt.Errorf("host is nil")
	}
	if ps == nil {
		return nil, fmt.Errorf("pubsub is nil")
	}
	if partition == "" {
		return nil, fmt.Errorf("partition is empty")
	}

	tm, err := NewTopicManager(ps, partition)
	if err != nil {
		return nil, fmt.Errorf("create topic manager: %w", err)
	}

	// Apply defaults
	if opts.BatchChannelSize <= 0 {
		opts.BatchChannelSize = DefaultBatchChannelSize
	}
	if opts.HeaderChannelSize <= 0 {
		opts.HeaderChannelSize = DefaultHeaderChannelSize
	}
	if opts.VoteChannelSize <= 0 {
		opts.VoteChannelSize = DefaultVoteChannelSize
	}
	if opts.CertificateChannelSize <= 0 {
		opts.CertificateChannelSize = DefaultCertificateChannelSize
	}
	if opts.CertSyncChannelSize <= 0 {
		opts.CertSyncChannelSize = DefaultCertSyncChannelSize
	}

	// Initialize rate limiter with default configuration
	rateLimiter := NewPeerRateLimiter(RateLimitConfig{})

	return &GossipLayer{
		host:          h,
		pubsub:        ps,
		topics:        tm,
		partition:     partition,
		batches:       make(chan *types.Batch, opts.BatchChannelSize),
		headers:       make(chan *types.Header, opts.HeaderChannelSize),
		votes:         make(chan *types.Vote, opts.VoteChannelSize),
		certs:         make(chan *types.Certificate, opts.CertificateChannelSize),
		syncRequests:  make(chan *CertSyncRequest, opts.CertSyncChannelSize),
		syncResponses: make(chan *CertSyncResponse, opts.CertSyncChannelSize),
		rateLimiter:   rateLimiter,
	}, nil
}

// Start joins all topics and begins processing incoming messages.
// Must be called before using broadcast or subscribe methods.
func (g *GossipLayer) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return fmt.Errorf("gossip layer is closed")
	}
	if g.ctx != nil {
		g.mu.Unlock()
		return fmt.Errorf("gossip layer already started")
	}
	g.ctx, g.cancel = context.WithCancel(ctx)
	g.mu.Unlock()

	// Join all topics
	if err := g.topics.JoinAll(g.ctx); err != nil {
		return fmt.Errorf("join topics: %w", err)
	}

	// Start message handlers BEFORE waiting for mesh formation.
	// This ensures we can receive messages while waiting for peers.
	g.wg.Add(7)
	go g.handleSubscription(TopicBatches, g.handleBatchMessage)
	go g.handleSubscription(TopicHeaders, g.handleHeaderMessage)
	go g.handleVoteSubscription()
	go g.handleSubscription(TopicCerts, g.handleCertMessage)
	go g.handleSubscription(TopicCertSync, g.handleCertSyncMessage)
	go g.rateLimiterCleanup()
	go g.meshMonitor()

	// Wait for GossipSub mesh to form before signaling ready.
	// This ensures peers are connected before the Primary starts broadcasting.
	// The mesh formation takes time as peers need to discover each other,
	// join topics, and establish connections.
	time.Sleep(3 * time.Second)
	slog.Info("Gossip layer started, topics joined", "partition", g.partition)

	return nil
}

// Close stops all message handlers and closes topics.
func (g *GossipLayer) Close() error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil
	}
	g.closed = true
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()

	// Wait for handlers to stop
	g.wg.Wait()

	// Close channels
	close(g.batches)
	close(g.headers)
	close(g.votes)
	close(g.certs)
	close(g.syncRequests)
	close(g.syncResponses)

	// Close topics
	return g.topics.Close()
}

// Partition returns the partition this GossipLayer is for.
func (g *GossipLayer) Partition() string {
	return g.partition
}

// Host returns the underlying libp2p host.
func (g *GossipLayer) Host() host.Host {
	return g.host
}

// BroadcastBatch publishes a batch to the network.
func (g *GossipLayer) BroadcastBatch(ctx context.Context, batch *types.Batch) error {
	data, err := batch.Marshal()
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}
	return g.publish(ctx, TopicBatches, data)
}

// BroadcastHeader publishes a header to the network.
func (g *GossipLayer) BroadcastHeader(ctx context.Context, header *types.Header) error {
	data, err := header.Marshal()
	if err != nil {
		return fmt.Errorf("marshal header: %w", err)
	}
	return g.publish(ctx, TopicHeaders, data)
}

// BroadcastVote publishes a vote to the network.
func (g *GossipLayer) BroadcastVote(ctx context.Context, vote *types.Vote) error {
	data, err := vote.Marshal()
	if err != nil {
		return fmt.Errorf("marshal vote: %w", err)
	}
	return g.publish(ctx, TopicVotes, data)
}

// BroadcastCertificate publishes a certificate to the network.
func (g *GossipLayer) BroadcastCertificate(ctx context.Context, cert *types.Certificate) error {
	data, err := cert.Marshal()
	if err != nil {
		return fmt.Errorf("marshal certificate: %w", err)
	}
	return g.publish(ctx, TopicCerts, data)
}

// SubscribeBatches returns a channel that receives batches from other nodes.
func (g *GossipLayer) SubscribeBatches() <-chan *types.Batch {
	return g.batches
}

// SubscribeHeaders returns a channel that receives headers from other nodes.
func (g *GossipLayer) SubscribeHeaders() <-chan *types.Header {
	return g.headers
}

// SubscribeVotes returns a channel that receives votes from other nodes.
func (g *GossipLayer) SubscribeVotes() <-chan *types.Vote {
	return g.votes
}

// SubscribeCertificates returns a channel that receives certificates from other nodes.
func (g *GossipLayer) SubscribeCertificates() <-chan *types.Certificate {
	return g.certs
}

// publish sends data to a topic.
func (g *GossipLayer) publish(ctx context.Context, pattern string, data []byte) error {
	topic := g.topics.GetTopic(pattern)
	if topic == nil {
		return fmt.Errorf("topic not joined: %s", g.topics.TopicName(pattern))
	}
	if peers := topic.ListPeers(); len(peers) == 0 {
		slog.Info("Publishing to topic with no peers",
			"topic", g.topics.TopicName(pattern),
			"partition", g.partition)
	}
	return topic.Publish(ctx, data)
}

// handleSubscription reads messages from a subscription and dispatches them.
func (g *GossipLayer) handleSubscription(pattern string, handler func([]byte)) {
	defer g.wg.Done()

	sub := g.topics.GetSubscription(pattern)
	if sub == nil {
		slog.Error("Subscription not found", "pattern", pattern, "partition", g.partition)
		return
	}

	for {
		msg, err := sub.Next(g.ctx)
		if err != nil {
			// Context canceled or subscription closed
			if g.ctx.Err() != nil {
				return
			}
			slog.Error("Error reading from subscription",
				"error", err,
				"pattern", pattern,
				"partition", g.partition)
			return
		}

		// Skip messages from ourselves
		if msg.ReceivedFrom == g.host.ID() {
			continue
		}

		handler(msg.Data)
	}
}

// handleBatchMessage deserializes and queues a batch message.
func (g *GossipLayer) handleBatchMessage(data []byte) {
	batch, err := types.UnmarshalBatch(data)
	if err != nil {
		slog.Debug("Invalid batch message",
			"error", err,
			"partition", g.partition)
		return
	}

	select {
	case g.batches <- batch:
	default:
		slog.Warn("Batch channel full, dropping message",
			"partition", g.partition)
	}
}

// handleHeaderMessage deserializes and queues a header message.
func (g *GossipLayer) handleHeaderMessage(data []byte) {
	header, err := types.UnmarshalHeader(data)
	if err != nil {
		slog.Debug("Invalid header message",
			"error", err,
			"partition", g.partition)
		return
	}
	slog.Info("Received header via gossip",
		"partition", g.partition,
		"round", header.Round,
		"author", fmt.Sprintf("%x", header.Author[:8]))

	select {
	case g.headers <- header:
	default:
		slog.Warn("Header channel full, dropping message",
			"partition", g.partition)
	}
}

// handleVoteMessage deserializes and queues a vote message.
func (g *GossipLayer) handleVoteMessage(data []byte) {
	vote, err := types.UnmarshalVote(data)
	if err != nil {
		slog.Debug("Invalid vote message",
			"error", err,
			"partition", g.partition)
		return
	}
	slog.Info("Received vote via gossip",
		"partition", g.partition,
		"round", vote.Round,
		"author", fmt.Sprintf("%x", vote.Author[:8]))

	select {
	case g.votes <- vote:
	default:
		slog.Warn("Vote channel full, dropping message",
			"partition", g.partition)
	}
}

// handleCertMessage deserializes and queues a certificate message.
func (g *GossipLayer) handleCertMessage(data []byte) {
	cert, err := types.UnmarshalCertificate(data)
	if err != nil {
		slog.Debug("Invalid certificate message",
			"error", err,
			"partition", g.partition)
		return
	}

	select {
	case g.certs <- cert:
	default:
		slog.Warn("Certificate channel full, dropping message",
			"partition", g.partition)
	}
}

// handleCertSyncMessage deserializes and routes cert sync messages.
// Cert sync messages can be either requests or responses, distinguished by
// the first byte: 0x01 = request, 0x02 = response.
func (g *GossipLayer) handleCertSyncMessage(data []byte) {
	if len(data) < 1 {
		slog.Debug("Empty cert sync message",
			"partition", g.partition)
		return
	}

	msgType := data[0]
	payload := data[1:]

	switch msgType {
	case 0x01: // Request
		req, err := UnmarshalCertSyncRequest(payload)
		if err != nil {
			slog.Debug("Invalid cert sync request",
				"error", err,
				"partition", g.partition)
			return
		}

		select {
		case g.syncRequests <- req:
		default:
			slog.Warn("Cert sync request channel full, dropping message",
				"partition", g.partition)
		}

	case 0x02: // Response
		resp, err := UnmarshalCertSyncResponse(payload)
		if err != nil {
			slog.Debug("Invalid cert sync response",
				"error", err,
				"partition", g.partition)
			return
		}

		select {
		case g.syncResponses <- resp:
		default:
			slog.Warn("Cert sync response channel full, dropping message",
				"partition", g.partition)
		}

	default:
		slog.Debug("Unknown cert sync message type",
			"type", msgType,
			"partition", g.partition)
	}
}

// BroadcastSyncRequest publishes a certificate sync request to the network.
func (g *GossipLayer) BroadcastSyncRequest(ctx context.Context, req *CertSyncRequest) error {
	payload, err := req.Marshal()
	if err != nil {
		return fmt.Errorf("marshal sync request: %w", err)
	}

	// Prepend message type byte
	data := make([]byte, 1+len(payload))
	data[0] = 0x01 // Request type
	copy(data[1:], payload)

	return g.publish(ctx, TopicCertSync, data)
}

// BroadcastSyncResponse publishes a certificate sync response to the network.
func (g *GossipLayer) BroadcastSyncResponse(ctx context.Context, resp *CertSyncResponse) error {
	payload, err := resp.Marshal()
	if err != nil {
		return fmt.Errorf("marshal sync response: %w", err)
	}

	// Prepend message type byte
	data := make([]byte, 1+len(payload))
	data[0] = 0x02 // Response type
	copy(data[1:], payload)

	return g.publish(ctx, TopicCertSync, data)
}

// SubscribeSyncRequests returns a channel that receives sync requests from other nodes.
func (g *GossipLayer) SubscribeSyncRequests() <-chan *CertSyncRequest {
	return g.syncRequests
}

// SubscribeSyncResponses returns a channel that receives sync responses from other nodes.
func (g *GossipLayer) SubscribeSyncResponses() <-chan *CertSyncResponse {
	return g.syncResponses
}

// handleVoteSubscription reads votes from the subscription and applies rate limiting.
func (g *GossipLayer) handleVoteSubscription() {
	defer g.wg.Done()

	sub := g.topics.GetSubscription(TopicVotes)
	if sub == nil {
		slog.Error("Subscription not found", "pattern", TopicVotes, "partition", g.partition)
		return
	}

	for {
		msg, err := sub.Next(g.ctx)
		if err != nil {
			// Context canceled or subscription closed
			if g.ctx.Err() != nil {
				return
			}
			slog.Error("Error reading from subscription",
				"error", err,
				"pattern", TopicVotes,
				"partition", g.partition)
			return
		}

		// Skip messages from ourselves
		if msg.ReceivedFrom == g.host.ID() {
			continue
		}

		// Check if peer was previously unbanned (track ban metric)
		wasBanned := g.rateLimiter.IsBanned(msg.ReceivedFrom)

		// Apply rate limiting
		if !g.rateLimiter.Allow(msg.ReceivedFrom) {
			metrics.VotesRateLimitedTotal.Inc()

			// Check if peer is now banned (first time)
			if !wasBanned && g.rateLimiter.IsBanned(msg.ReceivedFrom) {
				metrics.PeersBannedTotal.Inc()
				slog.Warn("Peer banned for rate limiting violations",
					"peer", msg.ReceivedFrom,
					"partition", g.partition)
			}

			slog.Debug("Vote rate limited",
				"peer", msg.ReceivedFrom,
				"partition", g.partition)
			continue
		}

		// Process the vote
		g.handleVoteMessage(msg.Data)
	}
}

// meshMonitor periodically logs gossip mesh health: how many peers the host
// is connected to versus how many peers each consensus topic actually has.
// The distinction diagnoses #4054-class failures — a topic with zero peers
// while the host has connections means subscription announcements are not
// propagating; zero host peers means the node is isolated at the libp2p level.
func (g *GossipLayer) meshMonitor() {
	defer g.wg.Done()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			hostPeers := g.host.Network().Peers()

			headerPeers := 0
			if t := g.topics.GetTopic(TopicHeaders); t != nil {
				headerPeers = len(t.ListPeers())
			}
			votePeers := 0
			if t := g.topics.GetTopic(TopicVotes); t != nil {
				votePeers = len(t.ListPeers())
			}
			certPeers := 0
			if t := g.topics.GetTopic(TopicCerts); t != nil {
				certPeers = len(t.ListPeers())
			}

			// Sample of connected peer IDs so cross-node connectivity can be
			// reconstructed from logs.
			sample := make([]string, 0, 16)
			for i, p := range hostPeers {
				if i == 16 {
					break
				}
				sample = append(sample, p.String()[len(p.String())-8:])
			}

			slog.Info("Gossip mesh status",
				"partition", g.partition,
				"hostPeers", len(hostPeers),
				"headerTopicPeers", headerPeers,
				"voteTopicPeers", votePeers,
				"certTopicPeers", certPeers,
				"connectedTo", strings.Join(sample, ","))
		}
	}
}

// rateLimiterCleanup periodically cleans up old peer state from the rate limiter
// and updates metrics.
func (g *GossipLayer) rateLimiterCleanup() {
	defer g.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			// Update banned peers metric
			stats := g.rateLimiter.GetAllStats()
			bannedCount := 0
			for _, stat := range stats {
				if stat.Banned {
					bannedCount++
				}
			}
			metrics.PeersBannedCurrent.Set(float64(bannedCount))

			// Clean up peers not seen in the last 10 minutes
			g.rateLimiter.Cleanup(10 * time.Minute)
		}
	}
}
