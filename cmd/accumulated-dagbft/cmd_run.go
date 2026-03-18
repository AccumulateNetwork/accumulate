// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated-dagbft/devnet"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// cmdRun returns the run command for running a single node.
func cmdRun() *cobra.Command {
	var (
		dataDir  string
		logLevel string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a DAG-BFT node",
		Long:  `Run a single DAG-BFT node using the configuration in the specified directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				return fmt.Errorf("--dir is required")
			}

			// Set up logging
			level := parseLogLevel(logLevel)
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

			slog.Info("Starting DAG-BFT node", "dir", dataDir)

			// TODO: Implement single node run
			// This would load config from dataDir and start a node

			return fmt.Errorf("single node run not yet implemented - use 'devnet run' for multi-node testing")
		},
	}

	cmd.Flags().StringVar(&dataDir, "dir", "", "Node data directory")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")

	return cmd
}

// cmdDevnetRun returns the devnet run command.
func cmdDevnetRun() *cobra.Command {
	var (
		dataDir       string
		logLevel      string
		blockInterval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run all nodes in a DAG-BFT devnet",
		Long: `Start all nodes in a DAG-BFT devnet simultaneously.

Example:
  accumulated-dagbft devnet run --dir /tmp/dagbft-devnet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				return fmt.Errorf("--dir is required")
			}

			// Resolve path
			dataDir, err := filepath.Abs(dataDir)
			if err != nil {
				return fmt.Errorf("resolve data dir: %w", err)
			}

			// Set up logging
			level := parseLogLevel(logLevel)
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

			// Load configuration
			configPath := devnet.ConfigPath(dataDir)
			config, err := devnet.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			slog.Info("Starting devnet",
				"network", config.Network,
				"nodes", config.NumNodes,
				"basePort", config.BasePort)

			// Create context with cancellation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start all nodes
			runner := newDevnetRunner(config, blockInterval)
			if err := runner.Start(ctx); err != nil {
				return fmt.Errorf("start devnet: %w", err)
			}

			// Handle shutdown signals
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			// Start status reporter
			go runner.reportStatus(ctx)

			slog.Info("Devnet started, press Ctrl+C to stop")

			// Wait for signal
			sig := <-sigCh
			slog.Info("Received signal, shutting down...", "signal", sig)

			// Stop all nodes
			runner.Stop()

			slog.Info("Devnet stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "dir", "", "Devnet data directory (required)")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	cmd.Flags().DurationVar(&blockInterval, "block-interval", 3*time.Second, "Block production interval")

	return cmd
}

// devnetRunner manages running all nodes in a devnet.
type devnetRunner struct {
	config        *devnet.DevnetConfig
	blockInterval time.Duration
	nodes         []*runningNode
	wg            sync.WaitGroup
}

// runningNode represents a running consensus node.
type runningNode struct {
	config    *devnet.NodeConfig
	node      *consensus.Node
	ctx       context.Context
	cancel    context.CancelFunc
	lastBlock uint64
}

// newDevnetRunner creates a new devnet runner.
func newDevnetRunner(config *devnet.DevnetConfig, blockInterval time.Duration) *devnetRunner {
	return &devnetRunner{
		config:        config,
		blockInterval: blockInterval,
	}
}

// Start starts all nodes in the devnet.
func (r *devnetRunner) Start(ctx context.Context) error {
	// Build committee from all validators
	validatorInfos := make([]types.ValidatorInfo, len(r.config.Nodes))
	for i, node := range r.config.Nodes {
		validatorInfos[i] = types.ValidatorInfo{
			PublicKey: node.ValidatorPublicKey(),
			Stake:     1,
		}
	}
	committee := types.NewCommittee(validatorInfos, 0)

	// Collect peer addresses
	peerAddrs := make([]multiaddr.Multiaddr, 0, len(r.config.Nodes))
	for _, node := range r.config.Nodes {
		peerID, err := node.PeerID()
		if err != nil {
			return fmt.Errorf("get peer ID for %s: %w", node.ID, err)
		}
		addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", node.P2PPort, peerID))
		if err != nil {
			return fmt.Errorf("create multiaddr for %s: %w", node.ID, err)
		}
		peerAddrs = append(peerAddrs, addr)
	}

	// Start each node
	r.nodes = make([]*runningNode, len(r.config.Nodes))
	for i := range r.config.Nodes {
		nodeConfig := &r.config.Nodes[i]
		running, err := r.startNode(ctx, nodeConfig, committee, peerAddrs)
		if err != nil {
			// Stop already started nodes
			for j := 0; j < i; j++ {
				r.nodes[j].cancel()
				r.nodes[j].node.Stop()
			}
			return fmt.Errorf("start %s: %w", nodeConfig.ID, err)
		}
		r.nodes[i] = running
	}

	// Wait for mesh to form
	slog.Info("Waiting for P2P mesh to form...")
	time.Sleep(5 * time.Second)

	// Initialize genesis for all nodes
	slog.Info("Initializing genesis certificates...")
	privKeys := make([]ed25519.PrivateKey, len(r.config.Nodes))
	for i, node := range r.config.Nodes {
		privKeys[i] = node.ValidatorKey
	}
	for _, running := range r.nodes {
		if err := running.node.InsertGenesisForAll(privKeys); err != nil {
			return fmt.Errorf("insert genesis for %s: %w", running.config.ID, err)
		}
	}

	// Start block production for each node
	for _, running := range r.nodes {
		r.wg.Add(1)
		go r.runBlockProduction(running)
	}

	return nil
}

// startNode starts a single node.
func (r *devnetRunner) startNode(ctx context.Context, nodeConfig *devnet.NodeConfig, committee *types.Committee, peers []multiaddr.Multiaddr) (*runningNode, error) {
	slog.Info("Starting node",
		"id", nodeConfig.ID,
		"p2p", nodeConfig.P2PPort,
		"api", nodeConfig.APIPort)

	// Create libp2p host
	libp2pKey, err := crypto.UnmarshalEd25519PrivateKey(nodeConfig.P2PKey)
	if err != nil {
		return nil, fmt.Errorf("unmarshal P2P key: %w", err)
	}

	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", nodeConfig.P2PPort))
	if err != nil {
		return nil, fmt.Errorf("create listen addr: %w", err)
	}

	host, err := libp2p.New(
		libp2p.Identity(libp2pKey),
		libp2p.ListenAddrs(listenAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	// Connect to peers
	for _, peerAddr := range peers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			continue
		}
		if peerInfo.ID == host.ID() {
			continue // Skip self
		}
		go func(pi peer.AddrInfo) {
			if err := host.Connect(ctx, pi); err != nil {
				slog.Debug("Failed to connect to peer", "peer", pi.ID, "error", err)
			}
		}(*peerInfo)
	}

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		host.Close()
		return nil, fmt.Errorf("create pubsub: %w", err)
	}

	// Create consensus node
	config := consensus.NodeConfig{
		Partition:        r.config.Network,
		KeyPair:          nodeConfig.ValidatorKey,
		NumWorkers:       nodeConfig.NumWorkers,
		DAGGCDepth:       types.Round(nodeConfig.DAGGCDepth),
		CommitBufferSize: nodeConfig.CommitBufferSize,
	}

	node, err := consensus.NewNode(config, committee, host, ps)
	if err != nil {
		host.Close()
		return nil, fmt.Errorf("create consensus node: %w", err)
	}

	// Start node
	nodeCtx, nodeCancel := context.WithCancel(ctx)
	if err := node.Start(nodeCtx); err != nil {
		nodeCancel()
		host.Close()
		return nil, fmt.Errorf("start consensus node: %w", err)
	}

	return &runningNode{
		config: nodeConfig,
		node:   node,
		ctx:    nodeCtx,
		cancel: nodeCancel,
	}, nil
}

// runBlockProduction runs the block production loop for a node.
func (r *devnetRunner) runBlockProduction(running *runningNode) {
	defer r.wg.Done()

	committed := running.node.Committed()
	stateHash := [32]byte{}

	for {
		select {
		case <-running.ctx.Done():
			return
		case cert, ok := <-committed:
			if !ok {
				return
			}
			if cert == nil {
				continue
			}

			// Simulate block production
			running.lastBlock++

			// Update state hash (simple simulation)
			for i := 0; i < 32; i++ {
				stateHash[i] ^= byte(running.lastBlock >> (i % 8))
			}

			slog.Debug("Block produced",
				"node", running.config.ID,
				"block", running.lastBlock,
				"round", cert.Header.Round,
				"batches", len(cert.Header.Payload),
				"stateHash", hex.EncodeToString(stateHash[:8]))

			// Prune batches
			digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
			for d := range cert.Header.Payload {
				digests = append(digests, d)
			}
			for _, w := range running.node.Workers() {
				w.PruneBatches(digests)
			}
		}
	}
}

// Stop stops all nodes.
func (r *devnetRunner) Stop() {
	for _, running := range r.nodes {
		running.cancel()
	}
	for _, running := range r.nodes {
		running.node.Stop()
	}
	r.wg.Wait()
}

// reportStatus periodically reports devnet status.
func (r *devnetRunner) reportStatus(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var blocks []string
			var rounds []string
			for _, running := range r.nodes {
				blocks = append(blocks, fmt.Sprintf("%s:%d", running.config.ID, running.lastBlock))
				rounds = append(rounds, fmt.Sprintf("%s:%d", running.config.ID, running.node.CurrentRound()))
			}
			slog.Info("Devnet status",
				"blocks", strings.Join(blocks, " "),
				"rounds", strings.Join(rounds, " "))
		}
	}
}

// parseLogLevel parses a log level string.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
