// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package consensus provides a complete DAG-based consensus node that
// orchestrates workers, primary, and Bullshark components.
package consensus

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/primary"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// Default configuration values.
const (
	DefaultNumWorkers = 1
	// DefaultDAGGCDepth is how many rounds of DAG history are retained past
	// the last commit. This is also the round catch-up window (#4057): a
	// node that falls further behind than this cannot recover round-by-round
	// because its peers have pruned the certificates it needs. At ~10
	// rounds/second the old value of 50 retained FIVE SECONDS of history —
	// any outage longer than that wedged the node permanently. 10,000
	// rounds is ~16 minutes at that rate and costs roughly 25 MB.
	DefaultDAGGCDepth       = 10_000
	DefaultCommitBufferSize = 5000 // Increased from 1000 for high throughput
)

// ErrNodeClosed is returned when operations are attempted on a closed node.
var ErrNodeClosed = errors.New("node is closed")

// ErrNodeNotStarted is returned when operations require the node to be started.
var ErrNodeNotStarted = errors.New("node not started")

// NodeConfig holds the configuration for a consensus Node.
type NodeConfig struct {
	// Partition is the network partition this node operates on.
	Partition string

	// KeyPair is the ed25519 private key for this validator.
	KeyPair ed25519.PrivateKey

	// NumWorkers is the number of workers to run. Defaults to DefaultNumWorkers.
	NumWorkers int

	// WorkerConfig is the configuration for each worker.
	WorkerConfig worker.Config

	// DAGGCDepth is the garbage collection depth for the DAG.
	// Defaults to DefaultDAGGCDepth.
	DAGGCDepth types.Round

	// CommitBufferSize is the size of the committed certificates channel.
	// Defaults to DefaultCommitBufferSize.
	CommitBufferSize int

	// MinRoundInterval paces round advancement, and therefore block cadence:
	// Bullshark commits a leader every other round, so blocks arrive at
	// roughly twice this interval. Zero falls back to
	// primary.DefaultMinRoundInterval (100ms) — which, unwired, is what ran
	// the Directory at ~21 blocks/sec under load and flooded anchor delivery
	// at block rate (#4098). Callers honouring config.Timing should set this
	// to BlockInterval/2.
	MinRoundInterval time.Duration

	// BatchCollectTimeout bounds how long CollectBatches waits for a committed
	// certificate's batches before declaring them unrecoverable (#4159).
	// CollectBatches never returns a partial set — executing without a batch
	// diverges state — so historically it waited forever, on the premise that
	// the certificate proves the data exists. LRU eviction broke that premise
	// (every holder can evict a not-yet-committed batch); worker.performEviction
	// no longer evicts a worker's OWN uncommitted batches, which restores it for
	// the common case, but if the sole holder is gone the wait is still endless.
	// After this timeout WITH no batch ever fetched from any peer, CollectBatches
	// returns ErrBatchesUnrecoverable so the node can halt cleanly (state-sync to
	// recover) instead of spinning silently. Generous by default so transient
	// absences and peer catch-up self-heal. Zero uses the default; negative
	// restores the old wait-forever behaviour.
	BatchCollectTimeout time.Duration
}

// DefaultBatchCollectTimeout is how long CollectBatches waits for a committed
// certificate's batches, with zero peer hits, before declaring them
// unrecoverable (#4159). Long enough that no healthy fetch or catch-up is cut
// short; short enough that a genuinely stranded partition stops in minutes
// instead of never.
const DefaultBatchCollectTimeout = 10 * time.Minute

// applyDefaults fills in default values for unset configuration fields.
func (c *NodeConfig) applyDefaults() {
	if c.BatchCollectTimeout == 0 {
		c.BatchCollectTimeout = DefaultBatchCollectTimeout
	}
	if c.NumWorkers <= 0 {
		c.NumWorkers = DefaultNumWorkers
	}
	// A power-of-two worker count lets the routing key be masked,
	// which is uniform and cheap; anything else falls back to modulo, which
	// works but gives uneven buckets. This network was
	// configured with 100 (#4133), which is neither a power of two nor a
	// number anyone chose for a reason. Warn rather than refuse: a running
	// deployment should not fail to start over it.
	if !IsPowerOfTwo(c.NumWorkers) {
		slog.Warn("Worker count is not a power of two — routing falls back to modulo and buckets are uneven",
			"numWorkers", c.NumWorkers,
			"suggestion", "use a power of two (e.g. 64 or 128)")
	}
	if c.DAGGCDepth == 0 {
		c.DAGGCDepth = DefaultDAGGCDepth
	}
	if c.CommitBufferSize <= 0 {
		c.CommitBufferSize = DefaultCommitBufferSize
	}
}

// Node represents a complete consensus node with all components.
// It orchestrates workers, primary, and Bullshark for DAG-based consensus.
type Node struct {
	config    NodeConfig
	committee *types.Committee
	host      host.Host
	pubsub    *pubsub.PubSub

	// Components
	dag       *dag.DAG
	gossip    *gossip.GossipLayer
	workers   []*worker.Worker
	primary   *primary.Primary
	bullshark *bullshark.Bullshark
	protocols *gossip.ProtocolHandler

	// Committed certificates channel
	committed chan []*types.Certificate

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	closed atomic.Bool

	// Metrics
	transactionsSubmitted atomic.Uint64
	certificatesCommitted atomic.Uint64
}

// NewNode creates a new consensus Node with the given configuration.
func NewNode(config NodeConfig, committee *types.Committee, h host.Host, ps *pubsub.PubSub) (*Node, error) {
	if committee == nil {
		return nil, errors.New("committee is nil")
	}
	if len(config.KeyPair) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid key pair size")
	}
	if config.Partition == "" {
		return nil, errors.New("partition is required")
	}

	config.applyDefaults()

	// Create DAG
	d := dag.NewDAG(config.DAGGCDepth)

	// Create gossip layer (may be nil for testing)
	var g *gossip.GossipLayer
	var err error
	if h != nil && ps != nil {
		g, err = gossip.NewGossipLayer(h, ps, config.Partition)
		if err != nil {
			return nil, fmt.Errorf("create gossip layer: %w", err)
		}
	}

	// Create workers
	workers := make([]*worker.Worker, config.NumWorkers)
	for i := 0; i < config.NumWorkers; i++ {
		wcfg := config.WorkerConfig
		wcfg.ID = types.WorkerID(i)
		wcfg.Partition = config.Partition
		workers[i] = worker.New(wcfg, g)
	}

	// Create primary
	pcfg := primary.Config{
		Partition:        config.Partition,
		KeyPair:          config.KeyPair,
		MinRoundInterval: config.MinRoundInterval,
	}
	p := primary.New(pcfg, committee, g, d, workers)

	// Create Bullshark
	bs := bullshark.New(committee, d)
	bs.SetPartition(config.Partition)

	// NOTE: We intentionally do NOT set an onCommit callback for batch pruning here.
	// The consumer of the committed channel (e.g., main.go or test code) is responsible
	// for pruning batches AFTER reading them. Pruning in onCommit would cause a race
	// condition where batches are pruned before the consumer can read them, resulting
	// in "Missing batch for certificate" errors.

	n := &Node{
		config:    config,
		committee: committee,
		host:      h,
		pubsub:    ps,
		dag:       d,
		gossip:    g,
		workers:   workers,
		primary:   p,
		bullshark: bs,
		committed: make(chan []*types.Certificate, config.CommitBufferSize),
	}

	// The batch-fetch protocol backs CollectBatches and the vote gate's
	// missing-batch pull: a committed certificate proves 2f+1 validators
	// stored its batches, so a node that lacks one pulls it from a peer
	// instead of executing without it. Partition-scoped: several partitions'
	// Nodes share one host, and an unscoped handler ID meant the last-started
	// partition served every fetch (#4159).
	if h != nil {
		ph, err := gossip.NewProtocolHandler(h, config.Partition, multiWorkerBatchStore{workers}, nil)
		if err != nil {
			return nil, fmt.Errorf("create protocol handler: %w", err)
		}
		n.protocols = ph
	}

	// The vote gate defers voting on a header until this node holds its
	// batches (#4159). Deferring alone would wedge — batch bytes are
	// broadcast once and the author's header rebroadcast does not re-send
	// them — so a deferring voter actively pulls the batch from peers (the
	// author certainly holds it: own batches are un-evictable). Async and
	// deduplicated by the primary; the next header rebroadcast finds the
	// batch present and the vote goes out.
	p.SetMissingBatchHandler(func(d types.BatchDigest) {
		go func() {
			if n.closed.Load() || n.protocols == nil || n.host == nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, peerID := range n.host.Network().Peers() {
				fctx, fcancel := context.WithTimeout(ctx, 2*time.Second)
				b, err := n.protocols.FetchBatch(fctx, peerID, d)
				fcancel()
				if err == nil && b != nil && b.Digest() == d {
					_ = n.workers[workerFor(d[:], len(n.workers))].StoreBatch(b)
					return
				}
				if ctx.Err() != nil {
					return
				}
			}
		}()
	})

	return n, nil
}

// multiWorkerBatchStore serves batch fetches from any of the node's workers.
type multiWorkerBatchStore struct{ workers []*worker.Worker }

func (s multiWorkerBatchStore) GetBatch(digest types.BatchDigest) (*types.Batch, error) {
	for _, w := range s.workers {
		if b, err := w.GetBatch(digest); err == nil && b != nil {
			return b, nil
		}
	}
	return nil, nil
}

func (s multiWorkerBatchStore) StoreBatch(batch *types.Batch) error {
	if len(s.workers) == 0 {
		return errors.New("no workers")
	}
	// Spread by digest rather than always worker 0.
	//
	// Both intake paths — gossip and peer fetch — used to store every batch
	// this node did not create into worker 0, while MaxStoredBatches is
	// enforced PER worker. With many workers that made worker 0 fill and evict
	// far sooner than any other, and what it evicts is exactly the batches
	// peers come asking for, which is the failure #4128 is about (#4133).
	d := batch.Digest()
	return s.workers[workerFor(d[:], len(s.workers))].StoreBatch(batch)
}

// ErrAlreadyExecuted reports that a committed certificate has already been
// executed by this node, so there is nothing left to collect.
//
// It is returned when a batch the certificate names is missing AND this node's
// tombstone says that same certificate is what committed it. That combination
// can only mean one thing: this node executed the certificate, retired its
// batches, and is being handed it a second time. Waiting is then not a
// liveness cost but a permanent halt — the Directory died exactly this way in
// run 20260822T015342Z, spinning at 5,500 peer requests a minute for a batch
// its own commit had retired (#4125).
//
// Skipping is safe ONLY in this exact case, because the work was already done.
// A batch missing for any other reason is still waited on: skipping there
// would execute a certificate without its transactions and diverge this node
// from every peer that had them.
var ErrAlreadyExecuted = errors.New("certificate already executed by this node")

// ErrBatchesUnrecoverable reports that a committed certificate's batches could
// not be collected within BatchCollectTimeout and no peer ever served one, so
// they are gone from the network (#4159). CollectBatches never returns a
// partial set — executing without a batch diverges state — so the caller must
// treat this as a clean HALT (state-sync to recover), never as a skip.
var ErrBatchesUnrecoverable = errors.New("committed certificate's batches are unrecoverable")

// executedBefore reports whether a missing batch proves this certificate has
// already been executed here.
func (n *Node) executedBefore(digest types.BatchDigest, cert *types.Certificate) bool {
	certDigest := cert.Digest().String()
	for _, w := range n.workers {
		g, ok := w.BatchGone(digest)
		if !ok {
			continue
		}
		if g.Reason == worker.GonePruned && g.Cert != "" && g.Cert == certDigest {
			return true
		}
	}
	return false
}

// batchAbsence explains, as far as this node can tell, why it does not hold a
// batch. Every removal from a worker's store leaves a tombstone, so the answer
// is usually "pruned after block N" or "evicted": the difference matters, since
// a batch pruned by an EARLIER commit means the same digest reached two
// certificates, while an eviction means the store was simply too small. When no
// worker has a tombstone the batch was never stored here at all, and the
// question is why the author never delivered it.
// absenceReason is batchAbsence reduced to a metric label.
func (n *Node) absenceReason(digest types.BatchDigest) string {
	for _, w := range n.workers {
		if g, ok := w.BatchGone(digest); ok {
			switch g.Reason {
			case worker.GonePruned:
				return "pruned"
			case worker.GoneEvicted:
				return "evicted"
			case worker.GoneRetentionExpired:
				return "retention_expired"
			}
			return "other"
		}
	}
	return "no_record"
}

func (n *Node) batchAbsence(digest types.BatchDigest) string {
	for _, w := range n.workers {
		if g, ok := w.BatchGone(digest); ok {
			return g.String()
		}
	}
	return worker.GoneUnknown
}

// CollectBatches returns the batches named by the certificate's payload, in
// canonical payload order. Local workers are consulted first; a batch this
// node does not hold is fetched from connected peers, retrying until the
// context expires. It never returns a partial set: executing a certificate
// without some of its batches makes this node's state diverge from every
// node that had them — six nodes at the same block index produced six
// different state hashes in TestStress_MultiNodeNetworkUnderLoad before this
// existed. Skipping is a safety violation; waiting is only a liveness cost,
// and the certificate itself is proof the data exists.
func (n *Node) CollectBatches(ctx context.Context, cert *types.Certificate) ([]*types.Batch, error) {
	batches := make([]*types.Batch, len(cert.Header.Payload))

	retry := time.NewTicker(50 * time.Millisecond)
	defer retry.Stop()

	// Diagnostics for a wait that does not end. The 2026-08-21 Directory halt
	// (#4125) produced 190,500 identical "missing=1" lines in twelve minutes,
	// naming neither the batch nor a reason — the log drowned the very fact it
	// was meant to flag, the same defect #4123 fixed for the stall report. Log
	// the first pass immediately and then at most one line per stallLogEvery,
	// carrying the digest, why this node thinks the batch is gone, and how the
	// peers answered.
	const stallLogEvery = 10 * time.Second
	var (
		waited     int
		lastLogged time.Time
		peerAsks   int
		peerHits   int
		started    = time.Now()
	)

	for {
		missing := 0
		var firstMissing types.BatchDigest
		for i, entry := range cert.Header.Payload {
			if batches[i] != nil {
				continue
			}
			// Local first — the common case.
			for _, w := range n.workers {
				if b, err := w.GetBatch(entry.Digest); err == nil && b != nil {
					batches[i] = b
					break
				}
			}
			if batches[i] != nil {
				continue
			}
			// Fetch from peers.
			if n.protocols != nil && n.host != nil {
				for _, peerID := range n.host.Network().Peers() {
					fctx, cancel := context.WithTimeout(ctx, 2*time.Second)
					peerAsks++
					b, err := n.protocols.FetchBatch(fctx, peerID, entry.Digest)
					cancel()
					if err == nil && b != nil && b.Digest() == entry.Digest {
						peerHits++
						// Store it so pruning-on-commit finds it and so this
						// node can serve it onward. Same worker the gossip
						// path would have chosen, so a fetched batch and a
						// gossiped one land in the same place (#4133).
						_ = n.workers[workerFor(entry.Digest[:], len(n.workers))].StoreBatch(b)
						batches[i] = b
						break
					}
				}
			}
			if batches[i] == nil {
				// A batch this certificate itself committed means the
				// certificate is being delivered twice. Say so instead of
				// waiting for something this node deliberately retired.
				if n.executedBefore(entry.Digest, cert) {
					metrics.CertificatesRedeliveredTotal.Inc()
					slog.Info("Skipping re-delivered certificate: already executed here",
						"partition", n.config.Partition,
						"round", cert.Header.Round,
						"cert", cert.Digest().String()[:16],
						"digest", entry.Digest.String()[:16])
					return nil, ErrAlreadyExecuted
				}
				if missing == 0 {
					firstMissing = entry.Digest
				}
				missing++
			}
		}
		if missing == 0 {
			return batches, nil
		}

		waited++
		if waited == 1 {
			metrics.BatchWaitsTotal.WithLabelValues(n.absenceReason(firstMissing)).Inc()
		}
		if now := time.Now(); lastLogged.IsZero() || now.Sub(lastLogged) >= stallLogEvery {
			lastLogged = now
			slog.Warn("Waiting for batches of committed certificate",
				"partition", n.config.Partition,
				"round", cert.Header.Round,
				"cert", cert.Digest().String()[:16],
				"author", fmt.Sprintf("%x", cert.Header.Author[:4]),
				"missing", missing,
				"payload", len(cert.Header.Payload),
				"digest", firstMissing.String(),
				"absence", n.batchAbsence(firstMissing),
				"peerAsks", peerAsks,
				"peerHits", peerHits,
				"attempts", waited)
		}

		// Bound the wait (#4159). The old contract — wait forever, the
		// certificate proves the data exists — held only while some node
		// retained the batch; LRU eviction could delete it everywhere. Fix 1
		// (workers keep their own uncommitted batches) restores that for the
		// common case, so a healthy fetch always makes progress. But if the
		// sole holder is gone the wait never ends. Give up ONLY when both are
		// true: the timeout elapsed, AND not a single missing batch was ever
		// fetched from a peer (peerHits==0) — i.e. no holder answered at all.
		// A run with any peer hits is making progress and keeps waiting. This
		// never returns a partial set; the caller halts cleanly so the node
		// can state-sync instead of spinning silently.
		if to := n.config.BatchCollectTimeout; to > 0 && peerHits == 0 && time.Since(started) > to {
			return nil, fmt.Errorf("%w: round %d cert %s: %d batch(es) still missing after %s (firstMissing=%s absence=%s peerAsks=%d peerHits=%d)",
				ErrBatchesUnrecoverable, cert.Header.Round, cert.Digest().String()[:16],
				missing, to, firstMissing.String(), n.batchAbsence(firstMissing), peerAsks, peerHits)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("collect batches for round %d: %d still missing: %w",
				cert.Header.Round, missing, ctx.Err())
		case <-retry.C:
		}
	}
}

// Start begins all node components and processes consensus.
// This method blocks until the context is canceled or Stop is called.
func (n *Node) Start(ctx context.Context) error {
	if n.closed.Load() {
		return ErrNodeClosed
	}

	n.mu.Lock()
	if n.ctx != nil {
		n.mu.Unlock()
		return errors.New("node already started")
	}
	n.ctx, n.cancel = context.WithCancel(ctx)
	nodeCtx := n.ctx
	n.mu.Unlock()

	pubKey := n.config.KeyPair.Public().(ed25519.PublicKey)
	slog.Info("Starting consensus node",
		"partition", n.config.Partition,
		"numWorkers", n.config.NumWorkers,
		"validatorKey", types.HeaderDigest(pubKey).String()[:16])

	// Start gossip layer (if available)
	if n.gossip != nil {
		if err := n.gossip.Start(nodeCtx); err != nil {
			return fmt.Errorf("start gossip: %w", err)
		}
		// Give gossip mesh time to form before starting primary
		time.Sleep(100 * time.Millisecond)
	}

	// Start workers
	for _, w := range n.workers {
		w := w // capture
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			_ = w.Start(nodeCtx)
		}()
	}

	// Start primary
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		_ = n.primary.Start(nodeCtx)
	}()

	// Start Bullshark processor
	n.wg.Add(1)
	go n.processBullshark()

	// Serve batch fetches: CollectBatches on other nodes depends on peers
	// answering, and a certificate is only as good as the availability of
	// its batches.
	if n.protocols != nil {
		if err := n.protocols.RegisterHandlers(); err != nil {
			return fmt.Errorf("register protocol handlers: %w", err)
		}
	}

	slog.Info("Consensus node started")

	return nil
}

// Stop stops the node gracefully.
func (n *Node) Stop() {
	if n.closed.Swap(true) {
		return // Already closed
	}

	n.mu.Lock()
	cancelFn := n.cancel
	n.mu.Unlock()

	if cancelFn != nil {
		cancelFn()
	}

	// Stop components in reverse order
	n.primary.Stop()

	for _, w := range n.workers {
		_ = w.Close()
	}

	if n.gossip != nil {
		_ = n.gossip.Close()
	}
	if n.protocols != nil {
		n.protocols.UnregisterHandlers()
	}

	// Wait for goroutines to finish
	n.wg.Wait()

	// Close committed channel
	close(n.committed)

	slog.Info("Consensus node stopped",
		"partition", n.config.Partition,
		"transactionsSubmitted", n.transactionsSubmitted.Load(),
		"certificatesCommitted", n.certificatesCommitted.Load())
}

// SubmitTransaction submits a transaction to a worker for batching.
// The transaction will be included in a batch and eventually ordered by consensus.
func (n *Node) SubmitTransaction(tx []byte) error {
	if n.closed.Load() {
		return ErrNodeClosed
	}

	n.mu.RLock()
	if n.ctx == nil {
		n.mu.RUnlock()
		return ErrNodeNotStarted
	}
	n.mu.RUnlock()

	if len(n.workers) == 0 {
		return errors.New("no workers available")
	}

	// No routing key: fall back to round-robin. This spreads a single sender's
	// transactions across workers, which destroys their execution order and
	// gets all but an increasing subsequence rejected by replay protection
	// (#4132). Callers that know the sender must use SubmitTransactionFor.
	idx := int(n.transactionsSubmitted.Add(1)-1) % len(n.workers)
	return n.workers[idx].Submit(tx)
}

// SubmitTransactionFor submits a transaction on behalf of a named sender.
//
// The key decides the worker, so everything from one sender is batched by one
// worker and keeps its order, while distinct senders still spread across
// workers — which is the parallelism worth having. Pass the signer's URL.
//
// An empty key routes to a worker deterministically rather than round-robin:
// unattributable traffic should still be stable, not deliberately scattered.
func (n *Node) SubmitTransactionFor(key string, tx []byte) error {
	if n.closed.Load() {
		return ErrNodeClosed
	}

	n.mu.RLock()
	started := n.ctx != nil
	n.mu.RUnlock()
	if !started {
		return ErrNodeNotStarted
	}

	if len(n.workers) == 0 {
		return errors.New("no workers available")
	}

	n.transactionsSubmitted.Add(1)
	idx := workerFor(routingKeyBytes(key), len(n.workers))
	return n.workers[idx].Submit(tx)
}

// WorkerFor reports which worker a key routes to. Exported for tests and for
// operators diagnosing where a sender's traffic lands.
func (n *Node) WorkerFor(key string) int {
	return workerFor(routingKeyBytes(key), len(n.workers))
}

// Committed returns a channel that receives committed certificate groups.
// Each group is one committed LEADER's sub-DAG in canonical order (leader
// last) — the deterministic unit of commitment, and therefore the unit the
// executor turns into ONE block. Grouping any other way (per certificate, per
// ProcessCertificate trigger) either multiplies end-of-block cost by the
// committee size (#4164) or depends on per-node arrival timing and diverges.
func (n *Node) Committed() <-chan []*types.Certificate {
	return n.committed
}

// Committee returns the current committee.
func (n *Node) Committee() *types.Committee {
	return n.committee
}

// DAG returns the certificate DAG.
func (n *Node) DAG() *dag.DAG {
	return n.dag
}

// Gossip returns the gossip layer (may be nil).
func (n *Node) Gossip() *gossip.GossipLayer {
	return n.gossip
}

// Workers returns the workers.
// BatchStore returns the store that receives batches from other nodes, so a
// test can verify that intake spreads across workers instead of piling into
// one (#4133).
func (n *Node) BatchStore() worker.BatchStore {
	return multiWorkerBatchStore{n.workers}
}

func (n *Node) Workers() []*worker.Worker {
	return n.workers
}

// Primary returns the primary component.
func (n *Node) Primary() *primary.Primary {
	return n.primary
}

// Bullshark returns the Bullshark consensus component.
func (n *Node) Bullshark() *bullshark.Bullshark {
	return n.bullshark
}

// Host returns the libp2p host (may be nil).
func (n *Node) Host() host.Host {
	return n.host
}

// PublicKey returns the validator's public key.
func (n *Node) PublicKey() ed25519.PublicKey {
	return n.config.KeyPair.Public().(ed25519.PublicKey)
}

// Partition returns the partition name.
func (n *Node) Partition() string {
	return n.config.Partition
}

// CurrentRound returns the current consensus round.
func (n *Node) CurrentRound() types.Round {
	return n.primary.CurrentRound()
}

// LastCommitRound returns the last committed leader round.
// Rejoin seeds the consensus position after a fast sync (#4058). The node's
// executor state was restored to a block committed at the given round, so
// consensus resumes there: the primary participates from the next round and
// Bullshark orders nothing at or below the seed. The seed must be within
// DAGGCDepth of the network's current round — fast sync's epoch is always a
// few blocks behind the tip, so it is — and the normal certificate-sync
// catch-up covers the remainder.
func (n *Node) Rejoin(round types.Round) {
	n.primary.SetRound(round)
	n.bullshark.SetLastCommitRound(round)
	n.dag.SetLastCommitRound(round)
	slog.Info("Rejoined consensus", "partition", n.config.Partition, "round", round)
}

func (n *Node) LastCommitRound() types.Round {
	return n.bullshark.LastCommitRound()
}

// Metrics returns node metrics.
func (n *Node) Metrics() (txSubmitted, certsCommitted uint64) {
	return n.transactionsSubmitted.Load(), n.certificatesCommitted.Load()
}

// processBullshark processes certificates from the primary and orders them.
func (n *Node) processBullshark() {
	defer n.wg.Done()

	certs := n.primary.NewCertificates()
	for {
		select {
		case <-n.ctx.Done():
			return

		case cert, ok := <-certs:
			if !ok {
				return
			}
			if cert == nil {
				continue
			}

			// Process certificate through Bullshark
			outputs := n.bullshark.ProcessCertificate(cert)

			// Send committed certificates to the executor, grouped by the
			// LEADER that committed them: one group = one leader's sub-DAG in
			// canonical order = one executor block. The leader boundary is
			// deterministic across validators whatever order certificates
			// arrived in; the trigger boundary (this loop iteration) is not.
			// NOTE: Batch pruning is handled by the executor (main.go) after reading
			// batches from workers. We must NOT prune here because the committed
			// channel is buffered - pruning before the executor reads would cause
			// "Missing batch for certificate" errors.
			var group []*types.Certificate
			var groupLeader types.Round
			flush := func() bool {
				if len(group) == 0 {
					return true
				}
				// BLOCK, never drop: a dropped committed certificate means
				// this node silently skips transactions its peers execute —
				// permanent state divergence (#4122's shape). If the executor
				// lags, backpressure here is the correct response; consensus
				// certificates keep accumulating in the channel's buffer and
				// the DAG regardless.
				select {
				case n.committed <- group:
					group = nil
					return true
				case <-n.ctx.Done():
					return false
				}
			}
			for _, output := range outputs {
				if len(group) > 0 && output.Leader != groupLeader {
					if !flush() {
						return
					}
				}
				groupLeader = output.Leader
				group = append(group, output.Certificate)
				n.certificatesCommitted.Add(1)
			}
			if !flush() {
				return
			}

			// Garbage collect old rounds
			if lastCommit := n.bullshark.LastCommitRound(); lastCommit > n.config.DAGGCDepth {
				n.dag.GarbageCollect(lastCommit)
			}
		}
	}
}

// InsertGenesisForAll inserts genesis certificates for all validators.
// This is used for bootstrapping the DAG.
// Deprecated: Use InitGenesis instead for proper genesis initialization.
func (n *Node) InsertGenesisForAll(keys []ed25519.PrivateKey) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i, key := range keys {
		pubKey := key.Public().(ed25519.PublicKey)

		// Create genesis header
		header := types.NewHeader(pubKey, 0, n.committee.Epoch, nil, nil)
		if err := header.Sign(key); err != nil {
			return fmt.Errorf("sign genesis header for validator %d: %w", i, err)
		}

		// Create certificate with signatures from all validators
		sigs := make([][]byte, len(keys))
		authors := make([]uint16, len(keys))
		headerDigest := header.Digest()

		for j, sigKey := range keys {
			sigs[j] = ed25519.Sign(sigKey, headerDigest[:])
			authors[j] = uint16(j)
		}

		cert := types.NewCertificate(header, sigs, authors)

		// Insert into DAG
		if err := n.dag.InsertGenesis(cert); err != nil {
			return fmt.Errorf("insert genesis cert for validator %d: %w", i, err)
		}
	}

	// Set primary to round 1 since genesis is done
	n.primary.SetRound(1)

	return nil
}

// InitGenesis initializes the DAG with genesis certificates from the given
// validators. Each validator provides a private key and stake. All validators
// sign all genesis certificates to establish initial consensus.
// After genesis, the node starts at round 1.
func (n *Node) InitGenesis(validators []genesis.ValidatorInfo) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Initialize genesis
	result, err := genesis.InitGenesis(validators)
	if err != nil {
		return fmt.Errorf("init genesis: %w", err)
	}

	// Bootstrap the DAG with genesis certificates
	if err := genesis.BootstrapDAG(n.dag, result.Committee, result.GenesisCerts); err != nil {
		return fmt.Errorf("bootstrap DAG: %w", err)
	}

	// Update committee to match genesis committee
	n.committee = result.Committee

	// Set primary to round 1 since genesis is done
	n.primary.SetRound(result.InitialRound)

	slog.Info("Genesis initialized",
		"validators", len(validators),
		"epoch", result.Committee.Epoch,
		"initialRound", result.InitialRound)

	return nil
}

// WaitForRound waits until the node reaches the specified round.
// Returns an error if the context is canceled or timeout occurs.
func (n *Node) WaitForRound(ctx context.Context, round types.Round) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if n.CurrentRound() >= round {
				return nil
			}
		}
	}
}

// WaitForCommits waits until the node has committed at least count certificates.
// Returns an error if the context is canceled or timeout occurs.
func (n *Node) WaitForCommits(ctx context.Context, count uint64) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if n.certificatesCommitted.Load() >= count {
				return nil
			}
		}
	}
}

// UpdateCommittee updates the committee across all consensus components.
// This is called when the validator set changes at a block boundary.
// All nodes must call this at the same height to maintain consensus.
func (n *Node) UpdateCommittee(committee *types.Committee) {
	if committee == nil {
		slog.Warn("UpdateCommittee called with nil committee")
		return
	}

	n.mu.Lock()
	oldCommittee := n.committee
	n.committee = committee
	n.mu.Unlock()

	slog.Info("Updating committee",
		"oldEpoch", oldCommittee.Epoch,
		"newEpoch", committee.Epoch,
		"oldValidators", oldCommittee.Len(),
		"newValidators", committee.Len())

	// Update Primary's committee
	n.primary.UpdateCommittee(committee)

	// Update Bullshark's committee
	n.bullshark.UpdateCommittee(committee)
}
