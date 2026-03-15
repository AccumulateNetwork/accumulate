// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// consensus-testnet is a standalone binary for testing the DAG consensus implementation.
// It runs a complete consensus node that generates transactions, orders them via
// Bullshark consensus, and produces blocks.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func main() {
	// Parse flags
	var (
		seed          = flag.String("seed", "", "Hex-encoded 32-byte seed for key generation")
		listenAddr    = flag.String("listen", "/ip4/0.0.0.0/tcp/9000", "Multiaddr to listen on")
		peersFlag     = flag.String("peers", "", "Comma-separated list of peer multiaddrs")
		partition     = flag.String("partition", "testnet", "Partition name")
		blockInterval = flag.Duration("block-interval", 3*time.Second, "Block production interval")
		txRate        = flag.Uint("tx-rate", 100, "Transactions per second to generate")
		txSize        = flag.Uint("tx-size", 256, "Size of each transaction payload in bytes")
		validators    = flag.String("validators", "", "Comma-separated list of validator public keys (hex)")
		logLevel      = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		warmup        = flag.Duration("warmup", 8*time.Second, "Warmup period before starting consensus")
	)
	flag.Parse()

	// Set up logging
	var level slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	// Generate or load key
	var privKey ed25519.PrivateKey
	if *seed != "" {
		seedBytes, err := hex.DecodeString(*seed)
		if err != nil || len(seedBytes) != 32 {
			slog.Error("Invalid seed: must be 64 hex characters (32 bytes)")
			os.Exit(1)
		}
		privKey = ed25519.NewKeyFromSeed(seedBytes)
	} else {
		_, privKey, _ = ed25519.GenerateKey(rand.Reader)
		slog.Warn("No seed provided, generated random key",
			"pubkey", hex.EncodeToString(privKey.Public().(ed25519.PublicKey)))
	}
	pubKey := privKey.Public().(ed25519.PublicKey)

	// Parse validator list
	var validatorKeys []ed25519.PublicKey
	if *validators != "" {
		for _, v := range strings.Split(*validators, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			key, err := hex.DecodeString(v)
			if err != nil || len(key) != 32 {
				slog.Error("Invalid validator key", "key", v)
				os.Exit(1)
			}
			validatorKeys = append(validatorKeys, key)
		}
	}
	// Add ourselves if not in list
	found := false
	for _, v := range validatorKeys {
		if string(v) == string(pubKey) {
			found = true
			break
		}
	}
	if !found {
		validatorKeys = append(validatorKeys, pubKey)
	}

	slog.Info("Starting consensus testnet node",
		"pubkey", hex.EncodeToString(pubKey),
		"listen", *listenAddr,
		"validators", len(validatorKeys),
		"block_interval", *blockInterval,
		"tx_rate", *txRate)

	// Create libp2p host
	libp2pKey, _ := crypto.UnmarshalEd25519PrivateKey(privKey)
	listenMA, _ := multiaddr.NewMultiaddr(*listenAddr)

	host, err := libp2p.New(
		libp2p.Identity(libp2pKey),
		libp2p.ListenAddrs(listenMA),
	)
	if err != nil {
		slog.Error("Failed to create libp2p host", "error", err)
		os.Exit(1)
	}
	defer host.Close()

	slog.Info("Listening", "id", host.ID(), "addrs", host.Addrs())

	// Create pubsub
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		slog.Error("Failed to create pubsub", "error", err)
		os.Exit(1)
	}

	// Connect to peers
	if *peersFlag != "" {
		for _, peerAddr := range strings.Split(*peersFlag, ",") {
			peerAddr = strings.TrimSpace(peerAddr)
			if peerAddr == "" {
				continue
			}
			ma, err := multiaddr.NewMultiaddr(peerAddr)
			if err != nil {
				slog.Warn("Invalid peer address", "addr", peerAddr, "error", err)
				continue
			}
			peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
			if err != nil {
				slog.Warn("Invalid peer info", "addr", peerAddr, "error", err)
				continue
			}
			if err := host.Connect(ctx, *peerInfo); err != nil {
				slog.Warn("Failed to connect to peer", "peer", peerInfo.ID, "error", err)
			} else {
				slog.Info("Connected to peer", "peer", peerInfo.ID)
			}
		}
	}

	// Create committee
	var validatorInfos []types.ValidatorInfo
	for _, v := range validatorKeys {
		validatorInfos = append(validatorInfos, types.ValidatorInfo{
			PublicKey: v,
			Stake:     1, // Equal stake
		})
	}
	committee := types.NewCommittee(validatorInfos, 1)

	// Create consensus node
	nodeConfig := consensus.NodeConfig{
		Partition:    *partition,
		KeyPair:      privKey,
		NumWorkers:   1,
	}
	node, err := consensus.NewNode(nodeConfig, committee, host, ps)
	if err != nil {
		slog.Error("Failed to create consensus node", "error", err)
		os.Exit(1)
	}

	// Create executor
	executor, err := NewExecutor(ExecutorConfig{
		Validators:    validatorKeys,
		BlockInterval: *blockInterval,
		TxRate:        uint32(*txRate),
		DataDir:       "/tmp/consensus-testnet",
	})
	if err != nil {
		slog.Error("Failed to create executor", "error", err)
		os.Exit(1)
	}

	executor.SetOnBlockProduced(func(block *Block) {
		hash := block.Hash()
		slog.Info("Block finalized",
			"height", block.Height,
			"txns", block.TxnCount,
			"state", hex.EncodeToString(block.StateHash[:8]),
			"hash", hex.EncodeToString(hash[:8]))
	})

	executor.SetOnParamChanged(func(param string, value any) {
		slog.Info("Parameter changed", "param", param, "value", value)
	})

	// Wait for mesh to form before starting consensus
	if *warmup > 0 {
		slog.Info("Waiting for GossipSub mesh to form", "warmup", *warmup)
		time.Sleep(*warmup)
	}

	// Start everything
	if err := node.Start(ctx); err != nil {
		slog.Error("Failed to start consensus node", "error", err)
		os.Exit(1)
	}

	executor.Start(ctx)

	// Process committed certificates
	go func() {
		committed := node.Committed()
		workers := node.Workers()
		for {
			select {
			case <-ctx.Done():
				return
			case cert := <-committed:
				if cert != nil {
					// Get batches for this certificate from workers
					batches := make(map[types.BatchDigest]*types.Batch)
					digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
					for digest := range cert.Header.Payload {
						digests = append(digests, digest)
						for _, w := range workers {
							if batch, err := w.GetBatch(digest); err == nil && batch != nil {
								batches[digest] = batch
								break
							}
						}
					}
					executor.ProcessCertificate(cert, batches)

					// Prune committed batches from workers to free memory
					for _, w := range workers {
						w.PruneBatches(digests)
					}
				}
			}
		}
	}()

	// Transaction generators - use multiple goroutines to hit target rate
	var nonce atomic.Uint64
	var submitted atomic.Uint64
	var dropped atomic.Uint64
	txGenDone := make(chan struct{})

	numGenerators := 10 // 10 goroutines each doing txRate/10
	ratePerGenerator := *txRate / uint(numGenerators)
	if ratePerGenerator < 1 {
		ratePerGenerator = 1
	}

	var genWg sync.WaitGroup
	for g := 0; g < numGenerators; g++ {
		genWg.Add(1)
		go func() {
			defer genWg.Done()
			ticker := time.NewTicker(time.Second / time.Duration(ratePerGenerator))
			defer ticker.Stop()

			payload := make([]byte, *txSize)
			rand.Read(payload)

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					tx := NewDataTx(pubKey, payload, nonce.Add(1))
					tx.Sign(privKey)

					if err := node.SubmitTransaction(tx.Marshal()); err != nil {
						dropped.Add(1)
					} else {
						submitted.Add(1)
					}
				}
			}
		}()
	}
	go func() {
		genWg.Wait()
		close(txGenDone)
	}()

	// Status reporter
	go func() {
		slog.Info("Status reporter started")
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		var lastSubmitted, lastDropped, lastProcessed uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				latest := executor.GetLatestBlock()
				currSubmitted := submitted.Load()
				currDropped := dropped.Load()
				currProcessed := executor.GetProcessedCount()
				slog.Info("Status",
					"blocks", executor.GetBlockCount(),
					"height", latest.Height,
					"txns/blk", latest.TxnCount,
					"submitted", currSubmitted,
					"dropped", currDropped,
					"processed", currProcessed,
					"sub/s", (currSubmitted-lastSubmitted)/10,
					"drop/s", (currDropped-lastDropped)/10,
					"tps", (currProcessed-lastProcessed)/10,
					"round", node.Primary().CurrentRound())
				lastSubmitted = currSubmitted
				lastDropped = currDropped
				lastProcessed = currProcessed
			}
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("Shutting down...")
	cancel()
	<-txGenDone
	executor.Stop()
	node.Stop()
	executor.Cleanup()

	// Print final stats
	fmt.Printf("\n=== Final Statistics ===\n")
	fmt.Printf("Blocks produced: %d\n", executor.GetBlockCount())
	fmt.Printf("Transactions processed: %d\n", executor.GetProcessedCount())
	stateHash := executor.GetStateHash()
	fmt.Printf("Final state hash: %s\n", hex.EncodeToString(stateHash[:]))
	latestBlock := executor.GetLatestBlock()
	latestHash := latestBlock.Hash()
	fmt.Printf("Latest block: height=%d, hash=%s\n", latestBlock.Height, hex.EncodeToString(latestHash[:]))
}
