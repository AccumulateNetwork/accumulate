// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package primary implements the Primary component for DAG-based consensus.
// The Primary is responsible for creating headers that reference available batches,
// collecting votes from other validators, aggregating votes into certificates,
// and broadcasting certificates to build the DAG.
package primary

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// Default configuration values.
const (
	// DefaultRoundAdvanceInterval is the default interval for checking round advancement.
	DefaultRoundAdvanceInterval = 50 * time.Millisecond

	// DefaultNewCertsChannelSize is the default buffer size for the new certificates channel.
	DefaultNewCertsChannelSize = 1000

	// DefaultMaxPendingHeaders is the maximum number of headers we track for vote collection.
	DefaultMaxPendingHeaders = 100

	// DefaultMinRoundInterval is the minimum time between round advancements.
	// This prevents consensus from advancing faster than the execution layer can handle.
	DefaultMinRoundInterval = 100 * time.Millisecond
)

// ErrPrimaryClosed is returned when operations are attempted on a closed primary.
var ErrPrimaryClosed = errors.New("primary is closed")

// ErrNotEnoughParents is returned when there aren't enough parent certificates.
var ErrNotEnoughParents = errors.New("not enough parent certificates")

// ErrInvalidVote is returned when a vote fails validation.
var ErrInvalidVote = errors.New("invalid vote")

// ErrInvalidHeader is returned when a header fails validation.
var ErrInvalidHeader = errors.New("invalid header")

// ErrInvalidCertificate is returned when a certificate fails validation.
var ErrInvalidCertificate = errors.New("invalid certificate")

// Config holds the configuration for a Primary.
type Config struct {
	// Partition is the network partition this primary operates on.
	Partition string

	// KeyPair is the ed25519 private key for this validator.
	KeyPair ed25519.PrivateKey

	// RoundAdvanceInterval is the interval for checking round advancement.
	// Defaults to DefaultRoundAdvanceInterval.
	RoundAdvanceInterval time.Duration

	// NewCertsChannelSize is the buffer size for the new certificates channel.
	// Defaults to DefaultNewCertsChannelSize.
	NewCertsChannelSize int

	// MinRoundInterval is the minimum time between round advancements.
	// This prevents consensus from racing ahead of the execution layer.
	// Defaults to DefaultMinRoundInterval.
	MinRoundInterval time.Duration
}

// applyDefaults fills in default values for unset configuration fields.
func (c *Config) applyDefaults() {
	if c.RoundAdvanceInterval <= 0 {
		c.RoundAdvanceInterval = DefaultRoundAdvanceInterval
	}
	if c.NewCertsChannelSize <= 0 {
		c.NewCertsChannelSize = DefaultNewCertsChannelSize
	}
	if c.MinRoundInterval <= 0 {
		c.MinRoundInterval = DefaultMinRoundInterval
	}
}

// Primary creates headers, collects votes, and produces certificates for DAG consensus.
// It implements the "header proposal" layer in the Narwhal/Bullshark architecture.
type Primary struct {
	config    Config
	committee *types.Committee
	gossip    *gossip.GossipLayer
	dag       *dag.DAG
	workers   []*worker.Worker

	// Current round state (protected by mu)
	mu               sync.Mutex
	currentRound     types.Round
	currentEpoch     uint64
	lastRoundAdvance time.Time

	// Vote collection for our headers (protected by mu)
	pendingVotes map[types.HeaderDigest][]*types.Vote

	// Our created headers (needed to build certificates, protected by mu)
	ourHeaders map[types.HeaderDigest]*types.Header

	// Certificates we've created (protected by mu)
	ourCerts map[types.Round]*types.Certificate

	// Set of headers we've already voted on (to avoid duplicate votes, protected by mu)
	// Maps header digest to the round it was for (enables round-based cleanup)
	votedHeaders map[types.HeaderDigest]types.Round

	// Channel to signal new certificates (for Bullshark)
	newCerts   chan *types.Certificate
	newCertsMu sync.Mutex

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool

	// Metrics
	headersCreated      atomic.Uint64
	certificatesCreated atomic.Uint64
	votesReceived       atomic.Uint64
	votesSent           atomic.Uint64
}

// New creates a new Primary with the given configuration.
func New(config Config, committee *types.Committee, g *gossip.GossipLayer, d *dag.DAG, workers []*worker.Worker) *Primary {
	config.applyDefaults()

	return &Primary{
		config:       config,
		committee:    committee,
		gossip:       g,
		dag:          d,
		workers:      workers,
		currentRound: 0,
		currentEpoch: committee.Epoch,
		pendingVotes: make(map[types.HeaderDigest][]*types.Vote),
		ourHeaders:   make(map[types.HeaderDigest]*types.Header),
		ourCerts:     make(map[types.Round]*types.Certificate),
		votedHeaders: make(map[types.HeaderDigest]types.Round),
		newCerts:     make(chan *types.Certificate, config.NewCertsChannelSize),
	}
}

// Start begins the primary's main loop, processing headers, votes, and certificates.
// This method blocks until the context is canceled or Close is called.
func (p *Primary) Start(ctx context.Context) error {
	if p.closed.Load() {
		return ErrPrimaryClosed
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	// Subscribe to gossip channels (if gossip layer is available)
	var headers <-chan *types.Header
	var votes <-chan *types.Vote
	var certs <-chan *types.Certificate

	if p.gossip != nil {
		headers = p.gossip.SubscribeHeaders()
		votes = p.gossip.SubscribeVotes()
		certs = p.gossip.SubscribeCertificates()
	}

	// Round advancement ticker
	ticker := time.NewTicker(p.config.RoundAdvanceInterval)
	defer ticker.Stop()

	slog.Info("Primary started",
		"partition", p.config.Partition,
		"epoch", p.currentEpoch,
		"round", p.currentRound)

	// Try to create initial header if we can
	p.tryCreateAndBroadcastHeader()

	for {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()

		case header := <-headers:
			if header != nil {
				p.OnHeaderReceived(header)
			}

		case vote := <-votes:
			if vote != nil {
				p.OnVoteReceived(vote)
			}

		case cert := <-certs:
			if cert != nil {
				p.OnCertificateReceived(cert)
			}

		case <-ticker.C:
			p.tryAdvanceRound()
		}
	}
}

// Stop stops the primary gracefully.
func (p *Primary) Stop() {
	if p.closed.Swap(true) {
		return // Already closed
	}

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for goroutines to finish
	p.wg.Wait()

	// Close the new certs channel with lock
	p.newCertsMu.Lock()
	close(p.newCerts)
	p.newCertsMu.Unlock()

	slog.Info("Primary stopped",
		"partition", p.config.Partition,
		"headersCreated", p.headersCreated.Load(),
		"certificatesCreated", p.certificatesCreated.Load())
}

// CurrentRound returns the current consensus round.
func (p *Primary) CurrentRound() types.Round {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentRound
}

// CurrentEpoch returns the current consensus epoch.
func (p *Primary) CurrentEpoch() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentEpoch
}

// NewCertificates returns a channel that receives new certificates.
// This is used by Bullshark to process committed certificates.
func (p *Primary) NewCertificates() <-chan *types.Certificate {
	return p.newCerts
}

// Metrics returns the primary's metrics.
func (p *Primary) Metrics() (headersCreated, certificatesCreated, votesReceived, votesSent uint64) {
	return p.headersCreated.Load(), p.certificatesCreated.Load(),
		p.votesReceived.Load(), p.votesSent.Load()
}

// PublicKey returns the validator's public key.
func (p *Primary) PublicKey() ed25519.PublicKey {
	return p.config.KeyPair.Public().(ed25519.PublicKey)
}

// tryCreateAndBroadcastHeader attempts to create a header for the current round
// and broadcast it for vote collection.
func (p *Primary) tryCreateAndBroadcastHeader() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if we already have a header for this round
	for _, h := range p.ourHeaders {
		if h.Round == p.currentRound {
			return // Already have a header for this round
		}
	}

	// Create header
	header, err := p.createHeaderLocked()
	if err != nil {
		slog.Debug("Cannot create header",
			"error", err,
			"round", p.currentRound)
		return
	}

	// Store for vote collection
	digest := header.Digest()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil

	p.headersCreated.Add(1)

	slog.Debug("Created header",
		"digest", digest.String(),
		"round", header.Round,
		"payload", len(header.Payload),
		"parents", len(header.Parents))

	// Add our own vote (self-vote)
	pubKey := p.config.KeyPair.Public().(ed25519.PublicKey)
	selfVote := types.NewVote(digest, header.Round, header.Epoch, pubKey)
	if err := selfVote.Sign(p.config.KeyPair); err != nil {
		slog.Warn("Failed to sign self-vote", "error", err)
	} else {
		p.pendingVotes[digest] = append(p.pendingVotes[digest], selfVote)
		p.votesSent.Add(1)

		// Check if we already have quorum (possible in single-node case)
		p.tryCreateCertificateLocked(digest)
	}

	// Broadcast header (outside lock)
	go func() {
		if p.gossip == nil {
			return
		}
		if err := p.gossip.BroadcastHeader(p.ctx, header); err != nil {
			slog.Warn("Failed to broadcast header",
				"error", err,
				"digest", digest.String())
		}
	}()
}

// cleanupOldHeaders removes headers, votes, and certificates for old rounds.
func (p *Primary) cleanupOldHeaders() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Headers/votes only needed for active vote collection (2 rounds)
	headerCutoff := p.currentRound
	if headerCutoff > 2 {
		headerCutoff = p.currentRound - 2
	} else {
		headerCutoff = 0
	}

	// Certs and voted headers can be kept longer (10 rounds)
	certCutoff := p.currentRound
	if certCutoff > 10 {
		certCutoff = p.currentRound - 10
	} else {
		certCutoff = 0
	}

	// Clean pending headers and votes (short retention)
	for digest, header := range p.ourHeaders {
		if header.Round < headerCutoff {
			delete(p.ourHeaders, digest)
			delete(p.pendingVotes, digest)
		}
	}

	// Clean old certificates we created (longer retention)
	for round := range p.ourCerts {
		if round < certCutoff {
			delete(p.ourCerts, round)
		}
	}

	// Clean old voted headers by round (longer retention)
	for digest, round := range p.votedHeaders {
		if round < certCutoff {
			delete(p.votedHeaders, digest)
		}
	}
}

// signalNewCertificate sends a certificate to the newCerts channel (non-blocking).
func (p *Primary) signalNewCertificate(cert *types.Certificate) {
	p.newCertsMu.Lock()
	defer p.newCertsMu.Unlock()

	if p.closed.Load() {
		return // Don't send on closed channel
	}

	select {
	case p.newCerts <- cert:
	default:
		slog.Warn("New certificates channel full, dropping notification",
			"digest", cert.Digest().String())
	}
}
