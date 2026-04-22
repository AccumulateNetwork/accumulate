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
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdLoadGen = &cobra.Command{
	Use:   "loadgen [config-file]",
	Short: "Configurable load generator for comprehensive protocol testing",
	Args:  cobra.ExactArgs(1),
	Run:   loadGen,
}

func init() {
	cmd.AddCommand(cmdLoadGen)
}

// LoadGenConfig defines the configuration for the load generator
type LoadGenConfig struct {
	Server         string             `json:"server"`
	TargetTPS      uint64             `json:"targetTPS"`
	RuntimeSeconds int                `json:"runtimeSeconds"`
	RampUpSeconds  int                `json:"rampUpSeconds"`
	Operations     OperationMixConfig `json:"operations"`
}

// OperationMixConfig defines the percentage distribution of operations
type OperationMixConfig struct {
	LiteToLiteTransfer   int `json:"liteToLiteTransfer"`
	LiteToADITransfer    int `json:"liteToADITransfer"`
	ADIToADITransfer     int `json:"adiToAdiTransfer"`
	KeyRotation          int `json:"keyRotation"`
	AddKeyBook           int `json:"addKeyBook"`
	AddKeyPage           int `json:"addKeyPage"`
	WriteData            int `json:"writeData"`
	CreateAccount        int `json:"createAccount"`
	UpdateKeyWeight      int `json:"updateKeyWeight"`
}

// Metrics tracks load generator performance
type Metrics struct {
	TotalSubmitted   atomic.Uint64
	TotalSuccess     atomic.Uint64
	TotalFailed      atomic.Uint64
	LatencySum       atomic.Uint64
	LatencyCount     atomic.Uint64
	OperationCounts  map[string]*atomic.Uint64
}

func newMetrics() *Metrics {
	m := &Metrics{
		OperationCounts: make(map[string]*atomic.Uint64),
	}
	operations := []string{
		"liteToLiteTransfer", "liteToADITransfer", "adiToAdiTransfer",
		"keyRotation", "addKeyBook", "addKeyPage", "writeData",
		"createAccount", "updateKeyWeight",
	}
	for _, op := range operations {
		m.OperationCounts[op] = &atomic.Uint64{}
	}
	return m
}

func (m *Metrics) recordLatency(latencyMs uint64) {
	m.LatencySum.Add(latencyMs)
	m.LatencyCount.Add(1)
}

func (m *Metrics) averageLatency() float64 {
	count := m.LatencyCount.Load()
	if count == 0 {
		return 0
	}
	return float64(m.LatencySum.Load()) / float64(count)
}

// LoadGenerator manages the load generation process
type LoadGenerator struct {
	config      *LoadGenConfig
	configFile  string
	client      *jsonrpc.Client
	metrics     *Metrics
	ctx         context.Context
	cancel      context.CancelFunc
	accounts    []*testAccount
	adiAccounts []*adiAccount
	logger      *log.Logger
}

type testAccount struct {
	privateKey *address.PrivateKey
	liteAcct   *protocol.LiteTokenAccount
	liteID     *protocol.LiteIdentity
	nonce      atomic.Uint64
}

type adiAccount struct {
	url        *url.URL
	keyBook    *url.URL
	keyPage    *url.URL
	privateKey ed25519.PrivateKey
	nonce      atomic.Uint64
}

func loadGen(_ *cobra.Command, args []string) {
	configFile := args[0]

	// Load configuration
	config, err := loadConfig(configFile)
	check(err)

	// Validate configuration
	if err := validateConfig(config); err != nil {
		fatalf("Invalid configuration: %v", err)
	}

	// Setup logger
	logFile, err := os.Create("/tmp/load-generator.log")
	check(err)
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("Starting load generator with config: %+v", config)

	// Create load generator
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lg := &LoadGenerator{
		config:     config,
		configFile: configFile,
		metrics:    newMetrics(),
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
	}

	// Initialize client
	lg.client = jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(config.Server, "v3"))
	lg.client.Client.Timeout = time.Minute

	// Setup accounts
	lg.logger.Println("Setting up test accounts...")
	if err := lg.setupAccounts(); err != nil {
		fatalf("Failed to setup accounts: %v", err)
	}

	// Start config watcher
	go lg.watchConfig()

	// Start metrics reporter
	go lg.reportMetrics()

	// Run load test
	lg.logger.Println("Starting load generation...")
	lg.run()
}

func loadConfig(path string) (*LoadGenConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config LoadGenConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func validateConfig(config *LoadGenConfig) error {
	if config.TargetTPS == 0 {
		return fmt.Errorf("targetTPS must be greater than 0")
	}

	total := config.Operations.LiteToLiteTransfer +
		config.Operations.LiteToADITransfer +
		config.Operations.ADIToADITransfer +
		config.Operations.KeyRotation +
		config.Operations.AddKeyBook +
		config.Operations.AddKeyPage +
		config.Operations.WriteData +
		config.Operations.CreateAccount +
		config.Operations.UpdateKeyWeight

	if total != 100 {
		return fmt.Errorf("operation percentages must sum to 100, got %d", total)
	}

	return nil
}

func (lg *LoadGenerator) setupAccounts() error {
	Q := api.Querier2{Querier: lg.client}

	// Create 10 lite accounts for operations
	for i := 0; i < 10; i++ {
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}

		addr := &address.PrivateKey{
			PublicKey: address.PublicKey{
				Type: protocol.SignatureTypeED25519,
				Key:  pk,
			},
			Key: sk,
		}

		kh, _ := addr.GetPublicKeyHash()
		acctUrl := protocol.LiteAuthorityForHash(kh).JoinPath("ACME")

		// Check if account exists
		var account *protocol.LiteTokenAccount
		_, err = Q.QueryAccountAs(lg.ctx, acctUrl, nil, &account)
		switch {
		case err == nil:
			// Account exists
		case errors.Is(err, errors.NotFound):
			// Create account with faucet
			sub, err := lg.client.Faucet(lg.ctx, acctUrl, api.FaucetOptions{Token: protocol.AcmeUrl()})
			if err != nil {
				return err
			}
			lg.logger.Printf("Created lite account %s with faucet tx %v", acctUrl, sub.Status.TxID)
			waitForMessages(lg.ctx, Q, map[[32]byte]struct{}{sub.Status.TxID.Hash(): {}})

			_, err = Q.QueryAccountAs(lg.ctx, acctUrl, nil, &account)
			if err != nil {
				return err
			}
		default:
			return err
		}

		// Get lite identity
		var lid *protocol.LiteIdentity
		_, err = Q.QueryAccountAs(lg.ctx, account.Url.RootIdentity(), nil, &lid)
		if err != nil {
			return err
		}

		// Ensure credits
		if lid.CreditBalance < 1000 {
			nonce := uint64(time.Now().UTC().UnixMilli())
			ns, err := lg.client.NetworkStatus(lg.ctx, api.NetworkStatusOptions{})
			if err != nil {
				return err
			}

			env, err := build.Transaction().
				For(account.Url).
				AddCredits().To(lid.Url).Purchase(1000).
				WithOracle(float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision).
				SignWith(lid.Url).Version(1).Timestamp(&nonce).PrivateKey(sk).
				Done()
			if err != nil {
				return err
			}

			subs, err := lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
			if err != nil {
				return err
			}

			ids := map[[32]byte]struct{}{}
			for _, sub := range subs {
				if sub.Success {
					ids[sub.Status.TxID.Hash()] = struct{}{}
				}
			}
			waitForMessages(lg.ctx, Q, ids)
			lg.logger.Printf("Added credits to lite account %s", acctUrl)
		}

		ta := &testAccount{
			privateKey: addr,
			liteAcct:   account,
			liteID:     lid,
		}
		ta.nonce.Store(uint64(time.Now().UTC().UnixMilli()))
		lg.accounts = append(lg.accounts, ta)
	}

	// Create 3 ADI accounts for ADI operations
	for i := 0; i < 3; i++ {
		if err := lg.createADIAccount(fmt.Sprintf("test-adi-%d", i)); err != nil {
			lg.logger.Printf("Warning: Failed to create ADI account: %v", err)
		}
	}

	lg.logger.Printf("Setup complete: %d lite accounts, %d ADI accounts", len(lg.accounts), len(lg.adiAccounts))
	return nil
}

func (lg *LoadGenerator) createADIAccount(name string) error {
	if len(lg.accounts) == 0 {
		return fmt.Errorf("no lite accounts available")
	}

	Q := api.Querier2{Querier: lg.client}
	ta := lg.accounts[0]

	adiUrl := protocol.AccountUrl(name)

	// Check if ADI already exists
	_, err := Q.QueryAccount(lg.ctx, adiUrl, nil)
	if err == nil {
		lg.logger.Printf("ADI %s already exists, skipping creation", adiUrl)
		return nil
	}

	// Generate key for ADI
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	// Create ADI
	nonce := ta.nonce.Add(1)
	env, err := build.Transaction().
		For(ta.liteAcct.Url).
		CreateIdentity(adiUrl).
		WithKey(pk, protocol.SignatureTypeED25519).
		WithKeyBook(adiUrl.JoinPath("book")).
		SignWith(ta.liteID.Url).Version(1).Timestamp(&nonce).PrivateKey(ta.privateKey.Key).
		Done()
	if err != nil {
		return err
	}

	subs, err := lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	ids := map[[32]byte]struct{}{}
	for _, sub := range subs {
		if sub.Success {
			ids[sub.Status.TxID.Hash()] = struct{}{}
		}
	}
	waitForMessages(lg.ctx, Q, ids)

	lg.adiAccounts = append(lg.adiAccounts, &adiAccount{
		url:        adiUrl,
		keyBook:    adiUrl.JoinPath("book"),
		keyPage:    adiUrl.JoinPath("book", "1"),
		privateKey: sk,
	})

	lg.logger.Printf("Created ADI account: %s", adiUrl)
	return nil
}

func (lg *LoadGenerator) run() {
	startTime := time.Now()
	rampUpDuration := time.Duration(lg.config.RampUpSeconds) * time.Second
	runtime := time.Duration(lg.config.RuntimeSeconds) * time.Second

	// Worker pool
	workers := make(chan struct{}, lg.config.TargetTPS)
	for i := uint64(0); i < lg.config.TargetTPS; i++ {
		workers <- struct{}{}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-lg.ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if runtime > 0 && elapsed > runtime {
				lg.logger.Println("Runtime complete, shutting down...")
				return
			}

			// Calculate current TPS based on ramp-up
			currentTPS := lg.config.TargetTPS
			if rampUpDuration > 0 && elapsed < rampUpDuration {
				ratio := float64(elapsed) / float64(rampUpDuration)
				currentTPS = uint64(float64(lg.config.TargetTPS) * ratio)
				if currentTPS == 0 {
					currentTPS = 1
				}
			}

			// Submit transactions for this second
			for i := uint64(0); i < currentTPS; i++ {
				<-workers
				go func() {
					defer func() { workers <- struct{}{} }()
					lg.submitRandomTransaction()
				}()
			}
		}
	}
}

func (lg *LoadGenerator) submitRandomTransaction() {
	// Choose operation based on configured mix
	config := lg.config.Operations
	roll := randInt(100)

	var opType string
	var err error

	cumulative := 0
	switch {
	case roll < (cumulative + config.LiteToLiteTransfer):
		opType = "liteToLiteTransfer"
		err = lg.submitLiteToLiteTransfer()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer):
		opType = "liteToADITransfer"
		err = lg.submitLiteToADITransfer()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer + config.ADIToADITransfer):
		opType = "adiToAdiTransfer"
		err = lg.submitADIToADITransfer()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer + config.ADIToADITransfer + config.KeyRotation):
		opType = "keyRotation"
		err = lg.submitKeyRotation()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer + config.ADIToADITransfer + config.KeyRotation + config.AddKeyBook):
		opType = "addKeyBook"
		err = lg.submitAddKeyBook()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer + config.ADIToADITransfer + config.KeyRotation + config.AddKeyBook + config.AddKeyPage):
		opType = "addKeyPage"
		err = lg.submitAddKeyPage()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer + config.ADIToADITransfer + config.KeyRotation + config.AddKeyBook + config.AddKeyPage + config.WriteData):
		opType = "writeData"
		err = lg.submitWriteData()
	case roll < (cumulative + config.LiteToLiteTransfer + config.LiteToADITransfer + config.ADIToADITransfer + config.KeyRotation + config.AddKeyBook + config.AddKeyPage + config.WriteData + config.CreateAccount):
		opType = "createAccount"
		err = lg.submitCreateAccount()
	default:
		opType = "updateKeyWeight"
		err = lg.submitUpdateKeyWeight()
	}

	lg.metrics.TotalSubmitted.Add(1)
	if counter, ok := lg.metrics.OperationCounts[opType]; ok {
		counter.Add(1)
	}

	if err != nil {
		lg.metrics.TotalFailed.Add(1)
		lg.logger.Printf("Error in %s: %v", opType, err)
	} else {
		lg.metrics.TotalSuccess.Add(1)
	}
}

func (lg *LoadGenerator) submitLiteToLiteTransfer() error {
	if len(lg.accounts) < 2 {
		return fmt.Errorf("not enough accounts")
	}

	from := lg.accounts[randInt(len(lg.accounts))]
	to := lg.accounts[randInt(len(lg.accounts))]

	nonce := from.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(from.liteAcct.Url).
		SendTokens(100, protocol.AcmePrecisionPower).To(to.liteAcct.Url).
		SignWith(from.liteID.Url).Version(1).Timestamp(&nonce).PrivateKey(from.privateKey.Key).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitLiteToADITransfer() error {
	if len(lg.accounts) == 0 || len(lg.adiAccounts) == 0 {
		return fmt.Errorf("not enough accounts")
	}

	from := lg.accounts[randInt(len(lg.accounts))]
	toADI := lg.adiAccounts[randInt(len(lg.adiAccounts))]
	toUrl := toADI.url.JoinPath("tokens")

	nonce := from.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(from.liteAcct.Url).
		SendTokens(100, protocol.AcmePrecisionPower).To(toUrl).
		SignWith(from.liteID.Url).Version(1).Timestamp(&nonce).PrivateKey(from.privateKey.Key).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitADIToADITransfer() error {
	if len(lg.adiAccounts) < 2 {
		return fmt.Errorf("not enough ADI accounts")
	}

	from := lg.adiAccounts[randInt(len(lg.adiAccounts))]
	to := lg.adiAccounts[randInt(len(lg.adiAccounts))]

	fromUrl := from.url.JoinPath("tokens")
	toUrl := to.url.JoinPath("tokens")

	nonce := from.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(fromUrl).
		SendTokens(100, protocol.AcmePrecisionPower).To(toUrl).
		SignWith(from.keyPage).Version(1).Timestamp(&nonce).PrivateKey(from.privateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitKeyRotation() error {
	if len(lg.accounts) == 0 {
		return fmt.Errorf("no accounts available")
	}

	account := lg.accounts[randInt(len(lg.accounts))]

	// Generate new key
	newPk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	nonce := account.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(account.liteID.Url).
		UpdateKey(newPk, protocol.SignatureTypeED25519).
		SignWith(account.liteID.Url).Version(1).Timestamp(&nonce).PrivateKey(account.privateKey.Key).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitAddKeyBook() error {
	if len(lg.adiAccounts) == 0 {
		return fmt.Errorf("no ADI accounts available")
	}

	adi := lg.adiAccounts[randInt(len(lg.adiAccounts))]
	newBookUrl := adi.url.JoinPath(fmt.Sprintf("book-%d", time.Now().UnixNano()))

	// Generate key for new book
	newPk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	nonce := adi.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(adi.url).
		CreateKeyBook(newBookUrl).
		WithKey(newPk, protocol.SignatureTypeED25519).
		SignWith(adi.keyPage).Version(1).Timestamp(&nonce).PrivateKey(adi.privateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitAddKeyPage() error {
	if len(lg.adiAccounts) == 0 {
		return fmt.Errorf("no ADI accounts available")
	}

	adi := lg.adiAccounts[randInt(len(lg.adiAccounts))]

	// Generate key for new page
	newPk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	nonce := adi.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(adi.keyBook).
		CreateKeyPage().
		WithEntry().Key(newPk, protocol.SignatureTypeED25519).FinishEntry().
		SignWith(adi.keyPage).Version(1).Timestamp(&nonce).PrivateKey(adi.privateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitWriteData() error {
	if len(lg.accounts) == 0 {
		return fmt.Errorf("no accounts available")
	}

	account := lg.accounts[randInt(len(lg.accounts))]

	// Create data entry
	entry := &protocol.DoubleHashDataEntry{
		Data: [][]byte{
			[]byte(fmt.Sprintf("load-test-data-%d", time.Now().UnixNano())),
		},
	}
	chainId := protocol.ComputeLiteDataAccountId(entry)
	lda, err := protocol.LiteDataAddress(chainId)
	if err != nil {
		return err
	}

	nonce := account.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(lda).
		WriteData(entry).
		SignWith(account.liteID.Url).Version(1).Timestamp(&nonce).PrivateKey(account.privateKey.Key).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitCreateAccount() error {
	if len(lg.adiAccounts) == 0 {
		return fmt.Errorf("no ADI accounts available")
	}

	adi := lg.adiAccounts[randInt(len(lg.adiAccounts))]
	newAcctUrl := adi.url.JoinPath(fmt.Sprintf("account-%d", time.Now().UnixNano()))

	nonce := adi.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(adi.url).
		CreateTokenAccount(newAcctUrl).
		ForToken(protocol.AcmeUrl()).
		SignWith(adi.keyPage).Version(1).Timestamp(&nonce).PrivateKey(adi.privateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) submitUpdateKeyWeight() error {
	if len(lg.adiAccounts) == 0 {
		return fmt.Errorf("no ADI accounts available")
	}

	adi := lg.adiAccounts[randInt(len(lg.adiAccounts))]

	// Generate a new key to add
	newPk, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	nonce := adi.nonce.Add(1)
	startTime := time.Now()

	env, err := build.Transaction().
		For(adi.keyPage).
		UpdateKeyPage().
		Add().Entry().Key(newPk, protocol.SignatureTypeED25519).FinishEntry().FinishOperation().
		SignWith(adi.keyPage).Version(1).Timestamp(&nonce).PrivateKey(adi.privateKey).
		Done()
	if err != nil {
		return err
	}

	_, err = lg.client.Submit(lg.ctx, env, api.SubmitOptions{})
	if err != nil {
		return err
	}

	lg.metrics.recordLatency(uint64(time.Since(startTime).Milliseconds()))
	return nil
}

func (lg *LoadGenerator) watchConfig() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastModTime := time.Now()

	for {
		select {
		case <-lg.ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(lg.configFile)
			if err != nil {
				lg.logger.Printf("Error checking config file: %v", err)
				continue
			}

			if info.ModTime().After(lastModTime) {
				lg.logger.Println("Config file changed, reloading...")
				newConfig, err := loadConfig(lg.configFile)
				if err != nil {
					lg.logger.Printf("Error loading config: %v", err)
					continue
				}

				if err := validateConfig(newConfig); err != nil {
					lg.logger.Printf("Invalid config: %v", err)
					continue
				}

				lg.config = newConfig
				lastModTime = info.ModTime()
				lg.logger.Printf("Config reloaded: %+v", newConfig)
			}
		}
	}
}

func (lg *LoadGenerator) reportMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastSubmitted := uint64(0)
	lastTime := time.Now()

	for {
		select {
		case <-lg.ctx.Done():
			return
		case <-ticker.C:
			submitted := lg.metrics.TotalSubmitted.Load()
			success := lg.metrics.TotalSuccess.Load()
			failed := lg.metrics.TotalFailed.Load()

			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()
			tps := float64(submitted-lastSubmitted) / elapsed

			successRate := float64(0)
			if submitted > 0 {
				successRate = float64(success) / float64(submitted) * 100
			}

			lg.logger.Printf("Metrics: TPS=%.2f, Total=%d, Success=%d, Failed=%d, Success Rate=%.2f%%, Avg Latency=%.2fms",
				tps, submitted, success, failed, successRate, lg.metrics.averageLatency())

			lg.logger.Printf("Operation counts:")
			for op, counter := range lg.metrics.OperationCounts {
				lg.logger.Printf("  %s: %d", op, counter.Load())
			}

			lastSubmitted = submitted
			lastTime = now
		}
	}
}

func randInt(max int) int {
	if max <= 0 {
		return 0
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 {
		n = -n
	}
	return n % max
}
