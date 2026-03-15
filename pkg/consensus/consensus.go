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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/primary"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// Default configuration values.
const (
	DefaultNumWorkers       = 1
	DefaultDAGGCDepth       = 50
	DefaultCommitBufferSize = 1000
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
		Partition: config.Partition,
		KeyPair:   config.KeyPair,
	}
	p := primary.New(pcfg, committee, g, d, workers)

	// Create Bullshark
	bs := bullshark.New(committee, d)

	return &Node{
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
	}, nil
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

			// Send committed certificates
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

		cert := types.NewCertificate(*header, sigs, authors)

		// Insert into DAG
		if err := n.dag.InsertGenesis(cert); err != nil {
			return fmt.Errorf("insert genesis cert for validator %d: %w", i, err)
		}
	}

	// Set primary to round 1 since genesis is done
	n.primary.SetRound(1)

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
