// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Config holds the load generator configuration.
type Config struct {
	Nodes        []string
	TPS          int
	Duration     time.Duration
	NumAccounts  int
	Setup        bool
	FunderURL    string
	FunderKey    ed25519.PrivateKey
	UseLite      bool
	LitePercent  int // percentage of accounts that are lite (0-100)
}

// Account represents a test account for load generation.
type Account struct {
	URL        *url.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	IsLite     bool
	Timestamp  atomic.Uint64
}

// Metrics tracks load generation statistics.
type Metrics struct {
	Submitted  atomic.Uint64
	Success    atomic.Uint64
	Failure    atomic.Uint64
	Latencies  []time.Duration
	latencyMu  sync.Mutex
	StartTime  time.Time
	PerNode    map[string]*NodeMetrics
	perNodeMu  sync.RWMutex
}

// NodeMetrics tracks per-node statistics.
type NodeMetrics struct {
	Submitted atomic.Uint64
	Success   atomic.Uint64
	Failure   atomic.Uint64
}

// LoadGenerator generates load against Accumulate nodes.
type LoadGenerator struct {
	config   Config
	clients  []*jsonrpc.Client
	accounts []*Account
	metrics  *Metrics
	funder   *Account
}

func main() {
	// Parse command line flags
	var (
		nodesStr    = flag.String("nodes", "http://127.0.0.1:8080/v3", "Comma-separated list of node URLs")
		tps         = flag.Int("tps", 1000, "Target transactions per second")
		duration    = flag.Duration("duration", 30*time.Minute, "Load test duration")
		numAccounts = flag.Int("accounts", 10000, "Number of test accounts to create")
		setup       = flag.Bool("setup", false, "Run account setup phase")
		funderURL   = flag.String("funder", "acc://load-funder.acme/tokens", "Funder account URL")
		funderKey   = flag.String("funder-key", "", "Funder private key (hex encoded)")
		litePercent = flag.Int("lite-percent", 20, "Percentage of accounts to create as lite (0-100)")
	)
	flag.Parse()

	// Parse nodes
	nodes := strings.Split(*nodesStr, ",")
	for i, n := range nodes {
		nodes[i] = strings.TrimSpace(n)
		if !strings.HasSuffix(nodes[i], "/v3") {
			nodes[i] = strings.TrimSuffix(nodes[i], "/") + "/v3"
		}
	}

	// Parse funder key
	var funderPrivKey ed25519.PrivateKey
	if *funderKey != "" {
		keyBytes, err := parseHexKey(*funderKey)
		if err != nil {
			log.Fatalf("Invalid funder key: %v", err)
		}
		funderPrivKey = ed25519.PrivateKey(keyBytes)
	} else if *setup {
		log.Fatal("Funder key is required for setup phase")
	}

	config := Config{
		Nodes:       nodes,
		TPS:         *tps,
		Duration:    *duration,
		NumAccounts: *numAccounts,
		Setup:       *setup,
		FunderURL:   *funderURL,
		FunderKey:   funderPrivKey,
		LitePercent: *litePercent,
	}

	lg, err := NewLoadGenerator(config)
	if err != nil {
		log.Fatalf("Failed to create load generator: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	if config.Setup {
		log.Println("Starting account setup phase...")
		if err := lg.SetupAccounts(ctx); err != nil {
			log.Fatalf("Account setup failed: %v", err)
		}
		log.Println("Account setup complete")
	} else {
		log.Println("Starting load generation phase...")
		if err := lg.GenerateLoad(ctx); err != nil && ctx.Err() == nil {
			log.Fatalf("Load generation failed: %v", err)
		}
		lg.PrintMetrics()
	}
}

// NewLoadGenerator creates a new load generator.
func NewLoadGenerator(config Config) (*LoadGenerator, error) {
	lg := &LoadGenerator{
		config: config,
		metrics: &Metrics{
			PerNode: make(map[string]*NodeMetrics),
		},
	}

	// Create clients for each node
	for _, node := range config.Nodes {
		client := jsonrpc.NewClient(node)
		lg.clients = append(lg.clients, client)
		lg.metrics.PerNode[node] = &NodeMetrics{}
	}

	// Setup funder account if key provided
	if config.FunderKey != nil {
		funderURL, err := url.Parse(config.FunderURL)
		if err != nil {
			return nil, fmt.Errorf("invalid funder URL: %w", err)
		}
		lg.funder = &Account{
			URL:        funderURL,
			PrivateKey: config.FunderKey,
			PublicKey:  config.FunderKey.Public().(ed25519.PublicKey),
			IsLite:     false,
		}
		lg.funder.Timestamp.Store(uint64(time.Now().UTC().UnixMilli()))
	}

	// Generate test accounts
	lg.generateAccounts()

	return lg, nil
}

// generateAccounts creates the test account list.
func (lg *LoadGenerator) generateAccounts() {
	lg.accounts = make([]*Account, lg.config.NumAccounts)

	for i := 0; i < lg.config.NumAccounts; i++ {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		isLite := (i*100/lg.config.NumAccounts) < lg.config.LitePercent

		var acctURL *url.URL
		if isLite {
			var err error
			acctURL, err = protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeED25519)
			if err != nil {
				log.Fatalf("Failed to create lite address: %v", err)
			}
		} else {
			// ADI token account: acc://a00000001.acme/tokens
			acctURL = protocol.AccountUrl(fmt.Sprintf("a%08d", i+1)).JoinPath("tokens")
		}

		lg.accounts[i] = &Account{
			URL:        acctURL,
			PrivateKey: priv,
			PublicKey:  pub,
			IsLite:     isLite,
		}
		lg.accounts[i].Timestamp.Store(uint64(time.Now().UTC().UnixMilli()))
	}
}

// SetupAccounts creates accounts on the network.
func (lg *LoadGenerator) SetupAccounts(ctx context.Context) error {
	if lg.funder == nil {
		return fmt.Errorf("funder account not configured")
	}

	log.Printf("Setting up %d accounts (%d%% lite, %d%% ADI)...",
		lg.config.NumAccounts, lg.config.LitePercent, 100-lg.config.LitePercent)

	// Setup in batches
	batchSize := 100
	for i := 0; i < len(lg.accounts); i += batchSize {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		end := i + batchSize
		if end > len(lg.accounts) {
			end = len(lg.accounts)
		}

		batch := lg.accounts[i:end]
		if err := lg.setupAccountBatch(ctx, batch); err != nil {
			return fmt.Errorf("batch %d-%d: %w", i, end, err)
		}

		log.Printf("Setup accounts %d-%d of %d", i+1, end, lg.config.NumAccounts)
	}

	return nil
}

// setupAccountBatch creates a batch of accounts.
func (lg *LoadGenerator) setupAccountBatch(ctx context.Context, accounts []*Account) error {
	for _, acct := range accounts {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if acct.IsLite {
			// Lite accounts: just fund them via SendTokens
			if err := lg.fundAccount(ctx, acct.URL, 1000); err != nil {
				return fmt.Errorf("fund lite %s: %w", acct.URL, err)
			}
		} else {
			// ADI accounts: create identity, key book, key page, and token account
			if err := lg.createADIAccount(ctx, acct); err != nil {
				return fmt.Errorf("create ADI %s: %w", acct.URL, err)
			}
		}
	}
	return nil
}

// createADIAccount creates an ADI with token account.
func (lg *LoadGenerator) createADIAccount(ctx context.Context, acct *Account) error {
	adi := acct.URL.RootIdentity()
	book := adi.JoinPath("book")
	page := book.JoinPath("1")

	// Create Identity
	ts := lg.funder.Timestamp.Add(1)
	env, err := build.Transaction().For(lg.funder.URL).
		CreateIdentity(adi).
		WithKey(acct.PublicKey, protocol.SignatureTypeED25519).
		WithKeyBook(book).
		SignWith(lg.funder.URL).Version(1).Timestamp(&ts).PrivateKey(lg.funder.PrivateKey).
		Done()
	if err != nil {
		return fmt.Errorf("build CreateIdentity: %w", err)
	}

	if err := lg.submitAndWait(ctx, env); err != nil {
		return fmt.Errorf("submit CreateIdentity: %w", err)
	}

	// Add credits to the key page
	ts = lg.funder.Timestamp.Add(1)
	env, err = build.Transaction().For(lg.funder.URL).
		AddCredits().WithOracle(0.05).Purchase(10000).To(page).
		SignWith(lg.funder.URL).Version(1).Timestamp(&ts).PrivateKey(lg.funder.PrivateKey).
		Done()
	if err != nil {
		return fmt.Errorf("build AddCredits: %w", err)
	}

	if err := lg.submitAndWait(ctx, env); err != nil {
		return fmt.Errorf("submit AddCredits: %w", err)
	}

	// Create Token Account
	ts2 := acct.Timestamp.Add(1)
	env, err = build.Transaction().For(adi).
		CreateTokenAccount(acct.URL).ForToken(protocol.AcmeUrl()).WithAuthority(book).
		SignWith(page).Version(1).Timestamp(&ts2).PrivateKey(acct.PrivateKey).
		Done()
	if err != nil {
		return fmt.Errorf("build CreateTokenAccount: %w", err)
	}

	if err := lg.submitAndWait(ctx, env); err != nil {
		return fmt.Errorf("submit CreateTokenAccount: %w", err)
	}

	// Fund the token account
	if err := lg.fundAccount(ctx, acct.URL, 1000); err != nil {
		return fmt.Errorf("fund account: %w", err)
	}

	return nil
}

// fundAccount sends tokens to an account.
func (lg *LoadGenerator) fundAccount(ctx context.Context, recipient *url.URL, amount uint64) error {
	ts := lg.funder.Timestamp.Add(1)
	env, err := build.Transaction().For(lg.funder.URL).
		SendTokens(amount, protocol.AcmePrecisionPower).To(recipient).
		SignWith(lg.funder.URL).Version(1).Timestamp(&ts).PrivateKey(lg.funder.PrivateKey).
		Done()
	if err != nil {
		return fmt.Errorf("build SendTokens: %w", err)
	}

	return lg.submitAndWait(ctx, env)
}

// submitAndWait submits a transaction and waits for confirmation.
func (lg *LoadGenerator) submitAndWait(ctx context.Context, env *messaging.Envelope) error {
	client := lg.clients[0]

	subs, err := client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	for _, sub := range subs {
		if !sub.Success {
			return fmt.Errorf("submission failed: %s", sub.Message)
		}
	}

	// Wait a bit for the transaction to be processed
	// In production, we'd poll for status
	time.Sleep(2 * time.Second)
	return nil
}

// GenerateLoad generates transaction load against the network.
func (lg *LoadGenerator) GenerateLoad(ctx context.Context) error {
	log.Printf("Generating load at %d TPS for %v across %d nodes",
		lg.config.TPS, lg.config.Duration, len(lg.clients))

	lg.metrics.StartTime = time.Now()

	// Calculate rate per worker
	numWorkers := lg.config.TPS
	if numWorkers > len(lg.accounts) {
		numWorkers = len(lg.accounts)
	}

	// Rate limiter: tokens per second
	interval := time.Second / time.Duration(lg.config.TPS)

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, lg.config.Duration)
	defer cancel()

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			lg.worker(ctx, workerID, interval)
		}(i)
	}

	// Progress reporter
	go lg.reportProgress(ctx)

	wg.Wait()
	return nil
}

// worker sends transactions at the specified rate.
func (lg *LoadGenerator) worker(ctx context.Context, workerID int, interval time.Duration) {
	// Each worker handles a subset of accounts
	accountsPerWorker := len(lg.accounts) / lg.config.TPS
	if accountsPerWorker < 1 {
		accountsPerWorker = 1
	}

	startIdx := workerID * accountsPerWorker % len(lg.accounts)

	ticker := time.NewTicker(interval * time.Duration(lg.config.TPS/100+1))
	defer ticker.Stop()

	txCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Select source and destination accounts
			srcIdx := (startIdx + txCount) % len(lg.accounts)
			dstIdx := (srcIdx + 1) % len(lg.accounts)

			src := lg.accounts[srcIdx]
			dst := lg.accounts[dstIdx]

			// Select node (round-robin)
			nodeIdx := int(lg.metrics.Submitted.Load()) % len(lg.clients)
			client := lg.clients[nodeIdx]
			nodeURL := lg.config.Nodes[nodeIdx]

			// Send transaction
			lg.sendTransaction(ctx, client, nodeURL, src, dst)
			txCount++
		}
	}
}

// sendTransaction creates and sends a SendTokens transaction.
func (lg *LoadGenerator) sendTransaction(ctx context.Context, client *jsonrpc.Client, nodeURL string, src, dst *Account) {
	start := time.Now()
	lg.metrics.Submitted.Add(1)

	lg.metrics.perNodeMu.RLock()
	nodeMetrics := lg.metrics.PerNode[nodeURL]
	lg.metrics.perNodeMu.RUnlock()
	nodeMetrics.Submitted.Add(1)

	// Build transaction
	ts := src.Timestamp.Add(1)
	amount := big.NewInt(1) // 1 atomic unit

	var signerURL *url.URL
	if src.IsLite {
		signerURL = src.URL.RootIdentity()
	} else {
		signerURL = src.URL.RootIdentity().JoinPath("book", "1")
	}

	env, err := build.Transaction().For(src.URL).
		Body(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: dst.URL, Amount: *amount},
			},
		}).
		SignWith(signerURL).Version(1).Timestamp(&ts).PrivateKey(src.PrivateKey).
		Done()
	if err != nil {
		lg.metrics.Failure.Add(1)
		nodeMetrics.Failure.Add(1)
		return
	}

	// Submit
	subs, err := client.Submit(ctx, env, api.SubmitOptions{})
	latency := time.Since(start)

	if err != nil || len(subs) == 0 || !subs[0].Success {
		lg.metrics.Failure.Add(1)
		nodeMetrics.Failure.Add(1)
		return
	}

	lg.metrics.Success.Add(1)
	nodeMetrics.Success.Add(1)

	lg.metrics.latencyMu.Lock()
	lg.metrics.Latencies = append(lg.metrics.Latencies, latency)
	lg.metrics.latencyMu.Unlock()
}

// reportProgress periodically logs progress.
func (lg *LoadGenerator) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(lg.metrics.StartTime)
			submitted := lg.metrics.Submitted.Load()
			success := lg.metrics.Success.Load()
			failure := lg.metrics.Failure.Load()

			actualTPS := float64(submitted) / elapsed.Seconds()

			log.Printf("Progress: submitted=%d success=%d failure=%d elapsed=%v actual_tps=%.2f",
				submitted, success, failure, elapsed.Round(time.Second), actualTPS)
		}
	}
}

// PrintMetrics outputs final metrics.
func (lg *LoadGenerator) PrintMetrics() {
	elapsed := time.Since(lg.metrics.StartTime)
	submitted := lg.metrics.Submitted.Load()
	success := lg.metrics.Success.Load()
	failure := lg.metrics.Failure.Load()

	fmt.Println("\n=== Load Generation Results ===")
	fmt.Printf("Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("Target TPS: %d\n", lg.config.TPS)
	fmt.Printf("Actual TPS: %.2f\n", float64(submitted)/elapsed.Seconds())
	fmt.Println()
	fmt.Printf("Submitted: %d\n", submitted)
	fmt.Printf("Success:   %d (%.2f%%)\n", success, float64(success)*100/float64(submitted))
	fmt.Printf("Failure:   %d (%.2f%%)\n", failure, float64(failure)*100/float64(submitted))

	// Latency statistics
	lg.metrics.latencyMu.Lock()
	latencies := make([]time.Duration, len(lg.metrics.Latencies))
	copy(latencies, lg.metrics.Latencies)
	lg.metrics.latencyMu.Unlock()

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

		p50 := latencies[len(latencies)*50/100]
		p99 := latencies[len(latencies)*99/100]

		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		avg := sum / time.Duration(len(latencies))

		fmt.Println()
		fmt.Println("Latency:")
		fmt.Printf("  Average: %v\n", avg.Round(time.Millisecond))
		fmt.Printf("  P50:     %v\n", p50.Round(time.Millisecond))
		fmt.Printf("  P99:     %v\n", p99.Round(time.Millisecond))
	}

	// Per-node statistics
	fmt.Println()
	fmt.Println("Per-Node Distribution:")
	lg.metrics.perNodeMu.RLock()
	for node, metrics := range lg.metrics.PerNode {
		fmt.Printf("  %s: submitted=%d success=%d failure=%d\n",
			node, metrics.Submitted.Load(), metrics.Success.Load(), metrics.Failure.Load())
	}
	lg.metrics.perNodeMu.RUnlock()
}

// parseHexKey parses a hex-encoded key.
func parseHexKey(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	if len(s)%2 != 0 {
		s = "0" + s
	}

	key := make([]byte, len(s)/2)
	for i := 0; i < len(key); i++ {
		var b byte
		_, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &b)
		if err != nil {
			return nil, err
		}
		key[i] = b
	}
	return key, nil
}
