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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Config holds load generator configuration
type Config struct {
	ServerURL      string         `json:"serverUrl"`
	TargetTPS      int            `json:"targetTps"`
	Duration       int            `json:"duration"`
	OperationMix   map[string]int `json:"operationMix"`
	RampUpSeconds  int            `json:"rampUpSeconds"`
	ConfigFile     string         `json:"-"`
	ReloadInterval int            `json:"reloadInterval"`
}

// Metrics tracks load generator metrics
type Metrics struct {
	TotalSubmitted   atomic.Uint64
	TotalSucceeded   atomic.Uint64
	TotalFailed      atomic.Uint64
	OperationCounts  map[string]*atomic.Uint64
	OperationLatency map[string]*LatencyTracker
}

// LatencyTracker tracks latency statistics
type LatencyTracker struct {
	count   atomic.Uint64
	sum     atomic.Uint64 // in milliseconds
	min     atomic.Uint64
	max     atomic.Uint64
	mu      sync.Mutex
	samples []time.Duration
}

// Account represents a test account with keys
type Account struct {
	URL        *url.URL
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Address    address.Address
	IsADI      bool
	Nonce      uint64
}

// LoadGenerator manages the load testing process
type LoadGenerator struct {
	config     *Config
	client     *jsonrpc.Client
	metrics    *Metrics
	accounts   []*Account
	adiCounter atomic.Uint64
	ctx        context.Context
	cancel     context.CancelFunc
	logFile    *os.File
	wg         sync.WaitGroup
	mu         sync.RWMutex
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to configuration file (optional)")
	flag.Parse()

	// Default configuration
	config := &Config{
		ServerURL:      "http://127.0.1.1:26660",
		TargetTPS:      100,
		Duration:       60,
		RampUpSeconds:  10,
		ReloadInterval: 30,
		OperationMix: map[string]int{
			"token-transfer-lite": 30,
			"token-transfer-adi":  20,
			"write-data":          20,
			"create-account":      10,
			"key-rotation":        10,
			"create-keypage":      5,
			"create-keybook":      5,
		},
	}

	// Load configuration from file if provided
	if configFile != "" {
		config.ConfigFile = configFile
		if err := loadConfig(config); err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Setup logging to /tmp/load-generator.log
	logFile, err := os.OpenFile("/tmp/load-generator.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// Create load generator
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lg := &LoadGenerator{
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
		logFile:  logFile,
		metrics:  newMetrics(),
		accounts: make([]*Account, 0),
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Initialize client
	if err := lg.initClient(); err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	// Initialize accounts
	log.Println("Initializing accounts...")
	if err := lg.initializeAccounts(); err != nil {
		log.Fatalf("Failed to initialize accounts: %v", err)
	}

	// Start metrics reporter
	lg.wg.Add(1)
	go lg.reportMetrics()

	// Start config reloader if config file specified
	if config.ConfigFile != "" {
		lg.wg.Add(1)
		go lg.reloadConfig()
	}

	// Run load test
	log.Printf("Starting load test: TPS=%d, Duration=%ds, RampUp=%ds\n",
		config.TargetTPS, config.Duration, config.RampUpSeconds)
	lg.runLoadTest()

	// Wait for goroutines to finish
	lg.wg.Wait()

	// Print final metrics
	lg.printFinalMetrics()
}

func newMetrics() *Metrics {
	m := &Metrics{
		OperationCounts:  make(map[string]*atomic.Uint64),
		OperationLatency: make(map[string]*LatencyTracker),
	}

	operations := []string{
		"token-transfer-lite", "token-transfer-adi", "write-data",
		"create-account", "key-rotation", "create-keypage", "create-keybook",
	}

	for _, op := range operations {
		m.OperationCounts[op] = &atomic.Uint64{}
		m.OperationLatency[op] = &LatencyTracker{samples: make([]time.Duration, 0, 1000)}
	}

	return m
}

func (lt *LatencyTracker) Record(duration time.Duration) {
	ms := uint64(duration.Milliseconds())
	lt.count.Add(1)
	lt.sum.Add(ms)

	// Update min
	for {
		current := lt.min.Load()
		if current != 0 && current <= ms {
			break
		}
		if lt.min.CompareAndSwap(current, ms) {
			break
		}
	}

	// Update max
	for {
		current := lt.max.Load()
		if current >= ms {
			break
		}
		if lt.max.CompareAndSwap(current, ms) {
			break
		}
	}

	// Store sample
	lt.mu.Lock()
	if len(lt.samples) < 1000 {
		lt.samples = append(lt.samples, duration)
	}
	lt.mu.Unlock()
}

func (lt *LatencyTracker) Stats() (count, avg, min, max uint64) {
	count = lt.count.Load()
	if count == 0 {
		return 0, 0, 0, 0
	}
	avg = lt.sum.Load() / count
	min = lt.min.Load()
	max = lt.max.Load()
	return
}

func loadConfig(config *Config) error {
	data, err := os.ReadFile(config.ConfigFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}

func (lg *LoadGenerator) initClient() error {
	lg.client = jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(lg.config.ServerURL, "v3"))
	lg.client.Client.Timeout = 5 * time.Minute
	return nil
}

func (lg *LoadGenerator) initializeAccounts() error {
	// Create 10 lite token accounts for testing
	for i := 0; i < 10; i++ {
		acc, err := lg.createLiteAccount()
		if err != nil {
			return fmt.Errorf("failed to create lite account %d: %v", i, err)
		}
		lg.accounts = append(lg.accounts, acc)
	}

	// Create 5 ADI accounts for testing
	for i := 0; i < 5; i++ {
		acc, err := lg.createADIAccount()
		if err != nil {
			return fmt.Errorf("failed to create ADI account %d: %v", i, err)
		}
		lg.accounts = append(lg.accounts, acc)
	}

	return nil
}

func (lg *LoadGenerator) createLiteAccount() (*Account, error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	addr := &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  pk,
		},
		Key: sk,
	}

	kh, _ := addr.GetPublicKeyHash()
	liteURL := protocol.LiteAuthorityForHash(kh).JoinPath("ACME")

	acc := &Account{
		URL:        liteURL,
		PrivateKey: sk,
		PublicKey:  pk,
		Address:    addr,
		IsADI:      false,
		Nonce:      uint64(time.Now().UnixMilli()),
	}

	// Fund the account with faucet
	Q := api.Querier2{Querier: lg.client}
	_, err = Q.QueryAccountAs(lg.ctx, liteURL, nil, new(*protocol.LiteTokenAccount))
	if errors.Is(err, errors.NotFound) {
		_, err = lg.client.Faucet(lg.ctx, liteURL, api.FaucetOptions{Token: protocol.AcmeUrl()})
		if err != nil {
			return nil, fmt.Errorf("faucet failed: %v", err)
		}
		time.Sleep(2 * time.Second) // Wait for transaction to complete
	}

	// Add credits
	liteIdentity := liteURL.RootIdentity()
	var lid *protocol.LiteIdentity
	_, err = Q.QueryAccountAs(lg.ctx, liteIdentity, nil, &lid)
	if err != nil {
		return nil, fmt.Errorf("query lite identity failed: %v", err)
	}

	if lid.CreditBalance < 100 {
		ns, err := lg.client.NetworkStatus(lg.ctx, api.NetworkStatusOptions{})
		if err != nil {
			return nil, err
		}

		nonce := acc.Nonce
		env, err := build.Transaction().
			For(liteURL).
			AddCredits().To(liteIdentity).Spend(100).
			WithOracle(float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision).
			SignWith(liteIdentity).Version(1).Timestamp(&nonce).PrivateKey(sk).
			Done()
		if err != nil {
			return nil, err
		}

		_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
		if err != nil {
			return nil, fmt.Errorf("add credits failed: %v", err)
		}
		acc.Nonce = nonce
		time.Sleep(2 * time.Second)
	}

	return acc, nil
}

func (lg *LoadGenerator) createADIAccount() (*Account, error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	addr := &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  pk,
		},
		Key: sk,
	}

	// Create lite account first for funding
	liteAcc, err := lg.createLiteAccount()
	if err != nil {
		return nil, err
	}

	// Create ADI
	adiNum := lg.adiCounter.Add(1)
	adiName := fmt.Sprintf("loadtest-%d-%d.acme", time.Now().Unix(), adiNum)
	adiURL := protocol.AccountUrl(adiName)

	nonce := liteAcc.Nonce
	env, err := build.Transaction().
		For(liteAcc.URL).
		CreateIdentity(adiURL).
		WithKey(pk, protocol.SignatureTypeED25519).
		WithKeyBook(adiURL.JoinPath("book")).
		SignWith(liteAcc.URL.RootIdentity()).Version(1).Timestamp(&nonce).PrivateKey(liteAcc.PrivateKey).
		Done()
	if err != nil {
		return nil, err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("create ADI failed: %v", err)
	}
	liteAcc.Nonce = nonce
	time.Sleep(2 * time.Second)

	// Create token account
	tokenURL := adiURL.JoinPath("tokens")
	nonce = liteAcc.Nonce
	env, err = build.Transaction().
		For(adiURL).
		CreateTokenAccount(tokenURL).
		ForToken(protocol.AcmeUrl()).
		SignWith(adiURL.JoinPath("book", "1")).Version(1).Timestamp(&nonce).PrivateKey(sk).
		Done()
	if err != nil {
		return nil, err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("create token account failed: %v", err)
	}
	liteAcc.Nonce = nonce
	time.Sleep(2 * time.Second)

	// Fund token account
	nonce = liteAcc.Nonce
	env, err = build.Transaction().
		For(liteAcc.URL).
		SendTokens(1000, protocol.AcmePrecision).To(tokenURL).
		SignWith(liteAcc.URL.RootIdentity()).Version(1).Timestamp(&nonce).PrivateKey(liteAcc.PrivateKey).
		Done()
	if err != nil {
		return nil, err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return nil, fmt.Errorf("fund token account failed: %v", err)
	}
	time.Sleep(2 * time.Second)

	return &Account{
		URL:        tokenURL,
		PrivateKey: sk,
		PublicKey:  pk,
		Address:    addr,
		IsADI:      true,
		Nonce:      uint64(time.Now().UnixMilli()),
	}, nil
}

func (lg *LoadGenerator) runLoadTest() {
	startTime := time.Now()
	endTime := startTime.Add(time.Duration(lg.config.Duration) * time.Second)

	// Calculate total weight for operation mix
	totalWeight := 0
	for _, weight := range lg.config.OperationMix {
		totalWeight += weight
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	currentSecond := 0
	for {
		select {
		case <-lg.ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(endTime) {
				log.Println("Load test duration completed")
				return
			}

			currentSecond++

			// Calculate current TPS based on ramp-up
			currentTPS := lg.config.TargetTPS
			if currentSecond <= lg.config.RampUpSeconds {
				currentTPS = (lg.config.TargetTPS * currentSecond) / lg.config.RampUpSeconds
				if currentTPS < 1 {
					currentTPS = 1
				}
			}

			// Submit transactions for this second
			lg.wg.Add(1)
			go func(tps int) {
				defer lg.wg.Done()
				lg.submitTransactionsForSecond(tps, totalWeight)
			}(currentTPS)
		}
	}
}

func (lg *LoadGenerator) submitTransactionsForSecond(tps, totalWeight int) {
	var wg sync.WaitGroup

	for i := 0; i < tps; i++ {
		// Select operation based on mix
		operation := lg.selectOperation(totalWeight)

		wg.Add(1)
		go func(op string) {
			defer wg.Done()
			lg.executeOperation(op)
		}(operation)

		// Small delay to spread transactions across the second
		time.Sleep(time.Second / time.Duration(tps))
	}

	wg.Wait()
}

func (lg *LoadGenerator) selectOperation(totalWeight int) string {
	lg.mu.RLock()
	defer lg.mu.RUnlock()

	// Use weighted random selection
	r := time.Now().UnixNano() % int64(totalWeight)
	currentWeight := int64(0)

	for op, weight := range lg.config.OperationMix {
		currentWeight += int64(weight)
		if r < currentWeight {
			return op
		}
	}

	return "token-transfer-lite" // fallback
}

func (lg *LoadGenerator) executeOperation(operation string) {
	start := time.Now()
	lg.metrics.TotalSubmitted.Add(1)

	var err error
	switch operation {
	case "token-transfer-lite":
		err = lg.opTokenTransferLite()
	case "token-transfer-adi":
		err = lg.opTokenTransferADI()
	case "write-data":
		err = lg.opWriteData()
	case "create-account":
		err = lg.opCreateAccount()
	case "key-rotation":
		err = lg.opKeyRotation()
	case "create-keypage":
		err = lg.opCreateKeyPage()
	case "create-keybook":
		err = lg.opCreateKeyBook()
	default:
		err = fmt.Errorf("unknown operation: %s", operation)
	}

	duration := time.Since(start)

	if err != nil {
		lg.metrics.TotalFailed.Add(1)
		log.Printf("Operation %s failed: %v\n", operation, err)
	} else {
		lg.metrics.TotalSucceeded.Add(1)
		lg.metrics.OperationCounts[operation].Add(1)
		lg.metrics.OperationLatency[operation].Record(duration)
	}
}

func (lg *LoadGenerator) opTokenTransferLite() error {
	// Select two random lite accounts
	if len(lg.accounts) < 2 {
		return fmt.Errorf("not enough accounts")
	}

	from := lg.accounts[0]
	to := lg.accounts[1]

	if from.IsADI || to.IsADI {
		return nil // skip if ADI selected
	}

	from.Nonce++
	env, err := build.Transaction().
		For(from.URL).
		SendTokens(1, protocol.AcmePrecision).To(to.URL).
		SignWith(from.URL.RootIdentity()).Version(1).Timestamp(&from.Nonce).PrivateKey(from.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) opTokenTransferADI() error {
	// Find ADI accounts
	var adiAccounts []*Account
	for _, acc := range lg.accounts {
		if acc.IsADI {
			adiAccounts = append(adiAccounts, acc)
		}
	}

	if len(adiAccounts) < 2 {
		return nil // skip if not enough ADI accounts
	}

	from := adiAccounts[0]
	to := adiAccounts[1]

	from.Nonce++
	env, err := build.Transaction().
		For(from.URL).
		SendTokens(1, protocol.AcmePrecision).To(to.URL).
		SignWith(from.URL.RootIdentity().JoinPath("book", "1")).Version(1).Timestamp(&from.Nonce).PrivateKey(from.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) opWriteData() error {
	acc := lg.accounts[0]

	entry := &protocol.DoubleHashDataEntry{
		Data: [][]byte{
			[]byte(fmt.Sprintf("load-test-%d", time.Now().UnixNano())),
		},
	}

	chainId := protocol.ComputeLiteDataAccountId(entry)
	lda, err := protocol.LiteDataAddress(chainId)
	if err != nil {
		return err
	}

	acc.Nonce++
	env, err := build.Transaction().
		For(lda).
		WriteData(entry).
		SignWith(acc.URL.RootIdentity()).Version(1).Timestamp(&acc.Nonce).PrivateKey(acc.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) opCreateAccount() error {
	acc := lg.accounts[0]
	if !acc.IsADI {
		return nil // skip for lite accounts
	}

	// Create a data account
	dataAccountURL := acc.URL.RootIdentity().JoinPath(fmt.Sprintf("data-%d", time.Now().UnixNano()))

	acc.Nonce++
	env, err := build.Transaction().
		For(acc.URL.RootIdentity()).
		CreateDataAccount(dataAccountURL).
		SignWith(acc.URL.RootIdentity().JoinPath("book", "1")).Version(1).Timestamp(&acc.Nonce).PrivateKey(acc.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) opKeyRotation() error {
	acc := lg.accounts[0]
	if !acc.IsADI {
		return nil // skip for lite accounts
	}

	// Generate new key
	newPk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	keyPageURL := acc.URL.RootIdentity().JoinPath("book", "1")

	acc.Nonce++
	env, err := build.Transaction().
		For(keyPageURL).
		UpdateKeyPage().
		Add().Entry().Key(newPk, protocol.SignatureTypeED25519).FinishEntry().
		FinishOperation().
		SignWith(keyPageURL).Version(1).Timestamp(&acc.Nonce).PrivateKey(acc.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) opCreateKeyPage() error {
	acc := lg.accounts[0]
	if !acc.IsADI {
		return nil // skip for lite accounts
	}

	pk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	keyBookURL := acc.URL.RootIdentity().JoinPath("book")

	acc.Nonce++
	env, err := build.Transaction().
		For(keyBookURL).
		CreateKeyPage().
		WithEntry().Key(pk, protocol.SignatureTypeED25519).FinishEntry().
		SignWith(keyBookURL.JoinPath("1")).Version(1).Timestamp(&acc.Nonce).PrivateKey(acc.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) opCreateKeyBook() error {
	acc := lg.accounts[0]
	if !acc.IsADI {
		return nil // skip for lite accounts
	}

	pk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	keyBookURL := acc.URL.RootIdentity().JoinPath(fmt.Sprintf("book-%d", time.Now().UnixNano()))

	acc.Nonce++
	env, err := build.Transaction().
		For(acc.URL.RootIdentity()).
		CreateKeyBook(keyBookURL).
		WithKey(pk, protocol.SignatureTypeED25519).
		SignWith(acc.URL.RootIdentity().JoinPath("book", "1")).Version(1).Timestamp(&acc.Nonce).PrivateKey(acc.PrivateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	return err
}

func (lg *LoadGenerator) reportMetrics() {
	defer lg.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-lg.ctx.Done():
			return
		case <-ticker.C:
			lg.printMetrics()
		}
	}
}

func (lg *LoadGenerator) printMetrics() {
	submitted := lg.metrics.TotalSubmitted.Load()
	succeeded := lg.metrics.TotalSucceeded.Load()
	failed := lg.metrics.TotalFailed.Load()

	successRate := float64(0)
	if submitted > 0 {
		successRate = float64(succeeded) / float64(submitted) * 100
	}

	log.Printf("\n=== Metrics Report ===\n")
	log.Printf("Total Submitted: %d\n", submitted)
	log.Printf("Total Succeeded: %d\n", succeeded)
	log.Printf("Total Failed: %d\n", failed)
	log.Printf("Success Rate: %.2f%%\n", successRate)
	log.Printf("\nOperation Breakdown:\n")

	for op, count := range lg.metrics.OperationCounts {
		opCount := count.Load()
		if opCount == 0 {
			continue
		}

		latency := lg.metrics.OperationLatency[op]
		cnt, avg, min, max := latency.Stats()

		log.Printf("  %s: %d transactions (avg: %dms, min: %dms, max: %dms)\n",
			op, cnt, avg, min, max)
	}
}

func (lg *LoadGenerator) printFinalMetrics() {
	log.Println("\n=== Final Metrics ===")
	lg.printMetrics()
}

func (lg *LoadGenerator) reloadConfig() {
	defer lg.wg.Done()

	ticker := time.NewTicker(time.Duration(lg.config.ReloadInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-lg.ctx.Done():
			return
		case <-ticker.C:
			lg.mu.Lock()
			if err := loadConfig(lg.config); err != nil {
				log.Printf("Failed to reload config: %v\n", err)
			} else {
				log.Println("Configuration reloaded successfully")
			}
			lg.mu.Unlock()
		}
	}
}
