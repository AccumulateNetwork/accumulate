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
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	// Parse flags
	var (
		seed          = flag.String("seed", "", "Hex-encoded 32-byte seed for key generation")
		signingKey    = flag.String("signing-key", "", "Hex-encoded ed25519 private key (64 bytes) or path to key file")
		validatorADI  = flag.String("validator", "", "Validator ADI URL (e.g., acc://validator-1.acme)")
		keyPageFlag   = flag.String("key-page", "", "Key page URL for ADI-based signing (e.g., acc://validator-1.acme/book/1)")
		listenAddr    = flag.String("listen", "/ip4/0.0.0.0/tcp/9000", "Multiaddr to listen on")
		peersFlag     = flag.String("peers", "", "Comma-separated list of peer multiaddrs")
		partition     = flag.String("partition", "testnet", "Partition name")
		blockInterval = flag.Duration("block-interval", 3*time.Second, "Block production interval")
		txRate        = flag.Uint("tx-rate", 100, "Transactions per second to generate")
		_             = flag.Uint("tx-size", 256, "Size of each transaction payload in bytes (deprecated, now uses Accumulate transactions)")
		validators    = flag.String("validators", "", "Comma-separated list of validator public keys (hex)")
		logLevel      = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		warmup        = flag.Duration("warmup", 8*time.Second, "Warmup period before starting consensus")
		pprofPort     = flag.Int("pprof-port", 0, "Port for pprof HTTP server (0 = disabled)")
	)
	flag.Parse()

	// Start pprof server if enabled
	if *pprofPort > 0 {
		go func() {
			addr := fmt.Sprintf(":%d", *pprofPort)
			slog.Info("Starting pprof server", "addr", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				slog.Error("pprof server failed", "error", err)
			}
		}()
	}

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

	// Validate block interval
	if *blockInterval <= 0 {
		slog.Error("Invalid block interval: must be positive", "block_interval", *blockInterval)
		os.Exit(1)
	}

	// Generate or load key and create signer
	var privKey ed25519.PrivateKey
	var signer types.Signer

	// Check for signing key first (ADI mode)
	if *signingKey != "" {
		keyBytes, err := loadSigningKey(*signingKey)
		if err != nil {
			slog.Error("Failed to load signing key", "error", err)
			os.Exit(1)
		}
		privKey = keyBytes
	} else if *seed != "" {
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

	// Create signer based on ADI or raw key mode
	if *keyPageFlag != "" || *validatorADI != "" {
		// ADI-based signing mode
		var keyPageURL *url.URL
		var err error

		if *keyPageFlag != "" {
			keyPageURL, err = url.Parse(*keyPageFlag)
			if err != nil {
				slog.Error("Invalid key page URL", "error", err)
				os.Exit(1)
			}
		} else {
			// Construct default key page from validator ADI
			keyPageURL, err = url.Parse(*validatorADI + "/book/1")
			if err != nil {
				slog.Error("Invalid validator ADI URL", "error", err)
				os.Exit(1)
			}
		}

		signer, err = types.NewADISigner(keyPageURL, privKey)
		if err != nil {
			slog.Error("Failed to create ADI signer", "error", err)
			os.Exit(1)
		}

		slog.Info("Using ADI-based validator identity",
			"validator", signer.ValidatorID(),
			"key_page", keyPageURL.String(),
			"pubkey", hex.EncodeToString(signer.PublicKey()[:8])+"...")
	} else {
		// Raw key signing mode (legacy)
		var err error
		signer, err = types.NewRawKeySigner(privKey)
		if err != nil {
			slog.Error("Failed to create raw key signer", "error", err)
			os.Exit(1)
		}

		slog.Info("Using raw key validator identity",
			"validator", signer.ValidatorID()[:32]+"...")
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
		"validator_id", signer.ValidatorID(),
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

	ps, err := pubsub.NewGossipSub(ctx, host,
		pubsub.WithValidateWorkers(runtime.NumCPU()), // Parallel signature validation
	)
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
		Partition:        *partition,
		KeyPair:          privKey,
		NumWorkers:       1,
		CommitBufferSize: 10000, // Large buffer for high throughput
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
	var submitted atomic.Uint64
	var dropped atomic.Uint64
	txGenDone := make(chan struct{})

	numGenerators := 10 // 10 goroutines each doing txRate/10
	ratePerGenerator := *txRate / uint(numGenerators)
	if ratePerGenerator < 1 {
		ratePerGenerator = 1
	}

	// Create Accumulate transaction generator
	accTxGen, err := NewAccumulateTransactionGenerator(privKey)
	if err != nil {
		slog.Error("Failed to create Accumulate transaction generator", "error", err)
		os.Exit(1)
	}
	slog.Info("Accumulate transaction generator initialized",
		"lite_account", accTxGen.GetLiteTokenAccount().String())

	// Transaction type counters for statistics
	var sendTokensCount, writeDataCount, burnTokensCount atomic.Uint64

	var genWg sync.WaitGroup
	for g := 0; g < numGenerators; g++ {
		genWg.Add(1)
		generatorID := g
		go func() {
			defer genWg.Done()
			ticker := time.NewTicker(time.Second / time.Duration(ratePerGenerator))
			defer ticker.Stop()

			txCounter := uint64(0)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					txCounter++
					var env *messaging.Envelope
					var err error
					var txType string

					// Distribute transaction types: 60% SendTokens, 25% WriteData, 15% BurnTokens
					// Use counter for deterministic distribution across generators
					switch (txCounter + uint64(generatorID)) % 20 {
					case 0, 1, 2: // 15% BurnTokens
						env, err = accTxGen.GenerateBurnTokens(1)
						txType = "BurnTokens"
						if err == nil {
							burnTokensCount.Add(1)
						}
					case 3, 4, 5, 6, 7: // 25% WriteData
						data := make([]byte, 64)
						_, _ = rand.Read(data)
						env, err = accTxGen.GenerateWriteData(data)
						txType = "WriteData"
						if err == nil {
							writeDataCount.Add(1)
						}
					default: // 60% SendTokens
						env, err = accTxGen.GenerateSelfSendTokens(1)
						txType = "SendTokens"
						if err == nil {
							sendTokensCount.Add(1)
						}
					}

					if err != nil {
						slog.Debug("Failed to generate Accumulate transaction", "type", txType, "error", err)
						dropped.Add(1)
						continue
					}

					// Marshal the envelope for submission
					envData, err := env.MarshalBinary()
					if err != nil {
						slog.Debug("Failed to marshal envelope", "error", err)
						dropped.Add(1)
						continue
					}

					if err := node.SubmitTransaction(envData); err != nil {
						dropped.Add(1)
						// Log backpressure specifically - this is NOT a silent drop
						if errors.Is(err, worker.ErrBackpressure) {
							slog.Warn("Backpressure: transaction rejected", "type", txType, "error", err)
						} else {
							slog.Debug("Transaction submission failed", "type", txType, "error", err)
						}
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
					"accumulate_txs", executor.GetAccumulateTxCount(),
					"sub/s", (currSubmitted-lastSubmitted)/10,
					"drop/s", (currDropped-lastDropped)/10,
					"tps", (currProcessed-lastProcessed)/10,
					"round", node.Primary().CurrentRound(),
					"send_tokens", sendTokensCount.Load(),
					"write_data", writeDataCount.Load(),
					"burn_tokens", burnTokensCount.Load())
				lastSubmitted = currSubmitted
				lastDropped = currDropped
				lastProcessed = currProcessed
			}
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	slog.Info("Waiting for shutdown signal...")
	sig := <-sigCh
	slog.Info("Received signal", "signal", sig)

	slog.Info("Shutting down...")
	cancel()
	<-txGenDone
	executor.Stop()
	node.Stop()
	_ = executor.Cleanup()

	// Print final stats
	fmt.Printf("\n=== Final Statistics ===\n")
	fmt.Printf("Blocks produced: %d\n", executor.GetBlockCount())
	fmt.Printf("Transactions processed: %d\n", executor.GetProcessedCount())
	fmt.Printf("  - Accumulate transactions: %d\n", executor.GetAccumulateTxCount())
	fmt.Printf("  - Legacy transactions: %d\n", executor.GetLegacyTxCount())
	fmt.Printf("Transaction types generated:\n")
	fmt.Printf("  - SendTokens: %d\n", sendTokensCount.Load())
	fmt.Printf("  - WriteData: %d\n", writeDataCount.Load())
	fmt.Printf("  - BurnTokens: %d\n", burnTokensCount.Load())
	stateHash := executor.GetStateHash()
	fmt.Printf("Final state hash: %s\n", hex.EncodeToString(stateHash[:]))
	latestBlock := executor.GetLatestBlock()
	latestHash := latestBlock.Hash()
	fmt.Printf("Latest block: height=%d, hash=%s\n", latestBlock.Height, hex.EncodeToString(latestHash[:]))
}

// loadSigningKey loads an ed25519 private key from either a hex string or a file path.
// If the input is 128 hex characters (64 bytes), it's treated as a hex-encoded private key.
// Otherwise, it's treated as a file path containing the hex-encoded key.
func loadSigningKey(keyOrPath string) (ed25519.PrivateKey, error) {
	// Try to decode as hex first
	if len(keyOrPath) == 128 {
		keyBytes, err := hex.DecodeString(keyOrPath)
		if err == nil && len(keyBytes) == ed25519.PrivateKeySize {
			return keyBytes, nil
		}
	}

	// Try to read as file
	data, err := os.ReadFile(keyOrPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	// Trim whitespace and decode
	keyHex := strings.TrimSpace(string(data))
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex in key file: %w", err)
	}

	if len(keyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid key size: expected %d bytes, got %d", ed25519.PrivateKeySize, len(keyBytes))
	}

	return keyBytes, nil
}
