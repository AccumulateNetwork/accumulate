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
	"fmt"
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

	// VotesPerHeaderMultiplier is the multiplier for maximum votes per header.
	// Maximum votes = quorum_threshold × multiplier.
	// This provides spam protection while allowing a safety margin above the consensus threshold.
	// With 2x multiplier: for n=4 validators, quorum=3, max_votes=6
	VotesPerHeaderMultiplier = 2
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
	config  Config
	gossip  *gossip.GossipLayer
	dag     *dag.DAG
	workers []*worker.Worker

	// Committee (protected by committeeMu - read-heavy, writes rare during epoch transitions)
	committeeMu sync.RWMutex
	committee   *types.Committee

	// Current round state (protected by roundMu)
	roundMu          sync.Mutex
	currentRound     types.Round
	currentEpoch     uint64
	lastRoundAdvance time.Time

	// Vote collection and certificate tracking (protected by pendingMu)
	pendingMu sync.Mutex
	// Vote collection for our headers
	pendingVotes map[types.HeaderDigest][]*types.Vote
	// Our created headers (needed to build certificates)
	ourHeaders map[types.HeaderDigest]*types.Header
	// Certificates we've created
	ourCerts map[types.Round]*types.Certificate
	// Set of headers we've already voted on (to avoid duplicate votes)
	// Maps header digest to the round it was for (enables round-based cleanup)
	votedHeaders map[types.HeaderDigest]types.Round
	sentVotes    map[types.HeaderDigest]*types.Vote

	// Round-sync pacing (#4057)
	roundSyncMu       sync.Mutex
	lastRoundPull     time.Time
	lastRoundPush     time.Time
	lastStallRecovery time.Time
	lastStrandedWarn  time.Time

	// pendingCerts buffers certificates that cannot be inserted due to missing parents
	pendingCerts *PendingCertificates

	// certSyncer handles requesting missing certificates from peers
	certSyncer *CertSyncer

	// Channel to signal new certificates (for Bullshark)
	newCerts   chan *types.Certificate
	newCertsMu sync.Mutex

	// Missing-batch pull for the vote-time availability gate (#4159): the
	// Node wires onMissingBatch to a peer fetch; missingBatchAsked
	// deduplicates asks per digest so the 1s header rebroadcast does not
	// re-fetch on every re-receipt.
	missingBatchMu    sync.Mutex
	missingBatchAsked map[types.BatchDigest]time.Time
	onMissingBatch    func(types.BatchDigest)

	// Lifecycle management
	// lifecycleMu guards ctx/cancel: Start runs in a goroutine spawned by
	// Node.Start, so an early Stop raced the write (caught by -race once the
	// root package finally ran under it, #4116).
	lifecycleMu sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	closed      atomic.Bool

	// Metrics
	headersCreated      atomic.Uint64
	certificatesCreated atomic.Uint64
	votesReceived       atomic.Uint64
	votesSent           atomic.Uint64
}

// DefaultPendingCertsGCDepth is the gc_depth for sizing the pending certificates buffer.
const DefaultPendingCertsGCDepth = 10

// New creates a new Primary with the given configuration.
func New(config Config, committee *types.Committee, g *gossip.GossipLayer, d *dag.DAG, workers []*worker.Worker) *Primary {
	config.applyDefaults()

	// Calculate pending certs buffer size: gc_depth × committee_size × 2
	pendingCertsMaxSize := DefaultPendingCertsGCDepth * committee.Len() * 2
	pendingCerts := NewPendingCertificates(pendingCertsMaxSize)

	p := &Primary{
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
		sentVotes:    make(map[types.HeaderDigest]*types.Vote),
		pendingCerts: pendingCerts,
		newCerts:     make(chan *types.Certificate, config.NewCertsChannelSize),
	}

	// Initialize CertSyncer if gossip is available
	if g != nil {
		syncerConfig := CertSyncerConfig{
			PublicKey: config.KeyPair.Public().(ed25519.PublicKey),
		}
		p.certSyncer = NewCertSyncer(syncerConfig, d, g, pendingCerts)
		// Set callback to process received certificates
		p.certSyncer.SetCertReceivedCallback(p.OnCertificateReceived)
	}

	return p
}

// Start begins the primary's main loop, processing headers, votes, and certificates.
// This method blocks until the context is canceled or Close is called.
func (p *Primary) Start(ctx context.Context) error {
	if p.closed.Load() {
		return ErrPrimaryClosed
	}

	p.lifecycleMu.Lock()
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.lifecycleMu.Unlock()

	// Start CertSyncer if available
	if p.certSyncer != nil {
		if err := p.certSyncer.Start(ctx); err != nil {
			return fmt.Errorf("start cert syncer: %w", err)
		}
	}

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

	// Pacing for header rebroadcast — see the ticker case below.
	var lastRebroadcast time.Time

	// Get current round/epoch for logging (thread-safe)
	p.roundMu.Lock()
	startRound := p.currentRound
	startEpoch := p.currentEpoch
	p.roundMu.Unlock()

	slog.Info("Primary started",
		"partition", p.config.Partition,
		"epoch", startEpoch,
		"round", startRound)

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
			// Periodically prune old pending certificates
			p.prunePendingCerts()
			// Re-broadcast pending headers that haven't achieved quorum.
			// This is a recovery mechanism for lost deliveries, so pace it
			// at 1s rather than the ticker's 50ms: every rebroadcast makes
			// every validator that already voted resend its vote, and at
			// ticker frequency that vote storm trips the per-peer rate
			// limiter and permanently stalls large committees (#4054).
			if time.Since(lastRebroadcast) >= time.Second {
				lastRebroadcast = time.Now()
				p.rebroadcastPendingHeaders()
			}
			// Recover from a silent stall (#4057): after an outage all
			// validators can end up with their OWN header certified but
			// missing each other's certificates — nothing is pending (so
			// nothing rebroadcasts), the round cannot advance (no quorum
			// of certificates), and with no traffic nothing triggers
			// round sync. Re-share the current round's certificates and
			// pull for what we are missing.
			p.recoverFromStall()
		}
	}
}

// Stop stops the primary gracefully.
func (p *Primary) Stop() {
	if p.closed.Swap(true) {
		return // Already closed
	}

	// Stop CertSyncer first
	if p.certSyncer != nil {
		p.certSyncer.Stop()
	}

	p.lifecycleMu.Lock()
	cancel := p.cancel
	p.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
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
	p.roundMu.Lock()
	defer p.roundMu.Unlock()
	return p.currentRound
}

// CurrentEpoch returns the current consensus epoch.
func (p *Primary) CurrentEpoch() uint64 {
	p.roundMu.Lock()
	defer p.roundMu.Unlock()
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
	// Wait for additional parent certificates before creating header.
	// This helps ensure late-arriving certificates are included as parents,
	// which is critical for commit rate: if a leader's certificate isn't
	// referenced by any header, it can never be committed.
	p.waitForAllParents()

	// Get current round (needs roundMu)
	p.roundMu.Lock()
	currentRound := p.currentRound
	currentEpoch := p.currentEpoch
	p.roundMu.Unlock()

	// Check pending state and create header (needs pendingMu)
	p.pendingMu.Lock()

	// Check if we already have a header for this round
	for _, h := range p.ourHeaders {
		if h.Round == currentRound {
			p.pendingMu.Unlock()
			return // Already have a header for this round
		}
	}

	// Create header (needs roundMu for createHeaderLocked, but we already have the round)
	header, err := p.createHeaderLockedWithRound(currentRound, currentEpoch)
	if err != nil {
		p.pendingMu.Unlock()
		slog.Info("Cannot create header",
			"partition", p.config.Partition,
			"error", err,
			"round", currentRound)
		return
	}

	// Store for vote collection
	digest := header.Digest()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil

	p.headersCreated.Add(1)

	slog.Info("Created header",
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

	p.pendingMu.Unlock()

	// Broadcast header (outside lock)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Check if context is cancelled
		if p.ctx != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
		}
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
	// Get current round
	p.roundMu.Lock()
	currentRound := p.currentRound
	p.roundMu.Unlock()

	// Headers/votes only needed for active vote collection (2 rounds)
	headerCutoff := currentRound
	if headerCutoff > 2 {
		headerCutoff = currentRound - 2
	} else {
		headerCutoff = 0
	}

	// Certs and voted headers can be kept longer (10 rounds)
	certCutoff := currentRound
	if certCutoff > 10 {
		certCutoff = currentRound - 10
	} else {
		certCutoff = 0
	}

	// Clean pending state
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	// Clean pending headers and votes (short retention). A header that never
	// became a certificate carries batches that were consumed from the workers
	// when it was built; discarding it without requeuing them silently loses
	// every transaction inside. Run 20260820T063739Z: roughly half of all
	// anchor-signature submissions vanished this way — the DN's pending-anchor
	// backlog grew without bound and healing's bounded re-drive became the
	// only working delivery path. ourCerts is retained five times longer than
	// ourHeaders, so it is a reliable certified-or-not signal at this cutoff.
	for digest, header := range p.ourHeaders {
		if header.Round < headerCutoff {
			if _, certified := p.ourCerts[header.Round]; !certified {
				p.requeueHeaderBatches(header)
			}
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

// requeueHeaderBatches returns a discarded header's batches to their workers
// so the next header this node authors carries them again.
func (p *Primary) requeueHeaderBatches(header *types.Header) {
	for _, entry := range header.Payload {
		for _, w := range p.workers {
			if w.ID() == entry.Worker {
				w.RequeueBatches([]types.BatchDigest{entry.Digest})
				break
			}
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

// rebroadcastPendingHeaders re-broadcasts headers that haven't achieved quorum.
// This helps with mesh formation issues where initial broadcasts may be lost.
func (p *Primary) rebroadcastPendingHeaders() {
	if p.gossip == nil {
		return
	}

	p.roundMu.Lock()
	currentRound := p.currentRound
	p.roundMu.Unlock()

	p.pendingMu.Lock()
	// Collect headers that need rebroadcast (those without certificates yet)
	var toRebroadcast []*types.Header
	for digest, header := range p.ourHeaders {
		// Voters accept rounds no older than their current round minus one, so
		// a header two or more rounds behind can never gather votes — drop it
		// here rather than spam the network with it (cleanupOldHeaders only
		// runs when the round advances, which it may not during a stall).
		// Same rule as cleanupOldHeaders: a header that never certified still
		// holds consumed batches — requeue them or their transactions are lost.
		if header.Round+1 < currentRound {
			if _, certified := p.ourCerts[header.Round]; !certified {
				p.requeueHeaderBatches(header)
			}
			delete(p.ourHeaders, digest)
			delete(p.pendingVotes, digest)
			continue
		}
		// Only rebroadcast if we don't have a certificate for this round yet
		if _, hasCert := p.ourCerts[header.Round]; !hasCert {
			toRebroadcast = append(toRebroadcast, header)
		}
	}
	p.pendingMu.Unlock()

	// Rebroadcast outside the lock
	for _, header := range toRebroadcast {
		p.wg.Add(1)
		go func(h *types.Header) {
			defer p.wg.Done()

			// Check if context is cancelled
			if p.ctx != nil {
				select {
				case <-p.ctx.Done():
					return
				default:
				}
			}
			if err := p.gossip.BroadcastHeader(p.ctx, h); err != nil {
				slog.Debug("Failed to rebroadcast header",
					"error", err,
					"digest", h.Digest().String())
			}
		}(header)
	}
}

// recoverFromStall re-shares the current round's certificates and requests
// the ones we are missing when the round has not advanced for a while. Paced
// internally; a no-op while consensus is healthy.
func (p *Primary) recoverFromStall() {
	p.roundMu.Lock()
	current := p.currentRound
	stalled := !p.lastRoundAdvance.IsZero() && time.Since(p.lastRoundAdvance) > 5*time.Second
	p.roundMu.Unlock()
	if !stalled {
		return
	}

	p.roundSyncMu.Lock()
	if time.Since(p.lastStallRecovery) < 2*time.Second {
		p.roundSyncMu.Unlock()
		return
	}
	p.lastStallRecovery = time.Now()
	p.roundSyncMu.Unlock()

	// Re-share every certificate we hold for the current and previous round
	// so peers that missed the one-shot broadcast can assemble quorum
	var shared int
	if p.gossip != nil {
		start := current
		if start > 0 {
			start--
		}
		for r := start; r <= current; r++ {
			for _, cert := range p.dag.GetRound(r) {
				cert := cert
				p.wg.Add(1)
				go func() {
					defer p.wg.Done()
					if err := p.gossip.BroadcastCertificate(p.ctx, cert); err != nil {
						slog.Debug("Failed to re-share certificate", "error", err)
					}
				}()
				shared++
			}
		}
	}

	// And pull whatever we are missing around the current round
	if p.certSyncer != nil {
		rounds := []types.Round{current, current + 1}
		if current > 0 {
			rounds = append([]types.Round{current - 1}, rounds...)
		}
		p.certSyncer.RequestRounds(rounds)
	}

	slog.Info("Consensus stalled - re-sharing certificates",
		"partition", p.config.Partition,
		"round", current,
		"shared", shared)
}
