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
}

// applyDefaults fills in default values for unset configuration fields.
func (c *NodeConfig) applyDefaults() {
	if c.NumWorkers <= 0 {
		c.NumWorkers = DefaultNumWorkers
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
	committed chan *types.Certificate

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
		committed: make(chan *types.Certificate, config.CommitBufferSize),
	}

	// The batch-fetch protocol backs CollectBatches: a committed certificate
	// proves 2f+1 validators stored its batches, so a node that lacks one
	// pulls it from a peer instead of executing without it.
	if h != nil {
		ph, err := gossip.NewProtocolHandler(h, multiWorkerBatchStore{workers}, nil)
		if err != nil {
			return nil, fmt.Errorf("create protocol handler: %w", err)
		}
		n.protocols = ph
	}

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
	return s.workers[0].StoreBatch(batch)
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

	for {
		missing := 0
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
					b, err := n.protocols.FetchBatch(fctx, peerID, entry.Digest)
					cancel()
					if err == nil && b != nil && b.Digest() == entry.Digest {
						// Store it so pruning-on-commit finds it and so this
						// node can serve it onward.
						_ = n.workers[0].StoreBatch(b)
						batches[i] = b
						break
					}
				}
			}
			if batches[i] == nil {
				missing++
			}
		}
		if missing == 0 {
			return batches, nil
		}

		slog.Warn("Waiting for batches of committed certificate",
			"partition", n.config.Partition,
			"round", cert.Header.Round,
			"missing", missing)
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

	// Round-robin to workers
	idx := int(n.transactionsSubmitted.Add(1)-1) % len(n.workers)
	return n.workers[idx].Submit(tx)
}

// Committed returns a channel that receives committed certificates.
// The certificates are ordered according to Bullshark consensus.
func (n *Node) Committed() <-chan *types.Certificate {
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

			// Send committed certificates to the executor
			// NOTE: Batch pruning is handled by the executor (main.go) after reading
			// batches from workers. We must NOT prune here because the committed
			// channel is buffered - pruning before the executor reads would cause
			// "Missing batch for certificate" errors.
			for _, output := range outputs {
				n.certificatesCommitted.Add(1)
				select {
				case n.committed <- output.Certificate:
				default:
					slog.Warn("Committed channel full, dropping certificate",
						"digest", output.Certificate.Digest().String())
				}
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
