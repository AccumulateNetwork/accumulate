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
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// ByzantineAttackConfig holds configuration for the Byzantine attack simulation.
type ByzantineAttackConfig struct {
	// NumMaliciousNodes is the number of Byzantine validators (default: 9 out of 25)
	NumMaliciousNodes int
	// DuplicateVotesPerRound is how many duplicate votes each Byzantine node sends per round
	DuplicateVotesPerRound int
	// TargetAPIEndpoint is the API endpoint to monitor consensus progress
	TargetAPIEndpoint string
	// AttackDuration is how long to run the attack
	AttackDuration time.Duration
	// MonitorInterval is how often to check consensus progress
	MonitorInterval time.Duration
}

// ConsensusMetrics tracks consensus progress during the attack.
type ConsensusMetrics struct {
	Round            uint64
	Epoch            uint64
	CertificatesCreated int
	VotesProcessed   int
	HonestValidators int
	TotalValidators  int
	Timestamp        time.Time
}

// ByzantineAttacker simulates Byzantine validators sending duplicate votes.
type ByzantineAttacker struct {
	config           ByzantineAttackConfig
	maliciousKeys    []ed25519.PrivateKey
	targetEndpoint   string
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	metricsHistory   []ConsensusMetrics
	mu               sync.Mutex
}

// NewByzantineAttacker creates a new Byzantine attack simulator.
func NewByzantineAttacker(config ByzantineAttackConfig) *ByzantineAttacker {
	ctx, cancel := context.WithCancel(context.Background())

	return &ByzantineAttacker{
		config:         config,
		targetEndpoint: config.TargetAPIEndpoint,
		ctx:            ctx,
		cancel:         cancel,
		metricsHistory: make([]ConsensusMetrics, 0),
	}
}

// Initialize generates malicious validator keys.
func (ba *ByzantineAttacker) Initialize() error {
	log.Printf("Initializing %d malicious validators...\n", ba.config.NumMaliciousNodes)

	ba.maliciousKeys = make([]ed25519.PrivateKey, ba.config.NumMaliciousNodes)
	for i := 0; i < ba.config.NumMaliciousNodes; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generate key %d: %w", i, err)
		}
		ba.maliciousKeys[i] = priv
		log.Printf("  Malicious validator %d: %x...\n", i, pub[:8])
	}

	return nil
}

// Start begins the Byzantine attack.
func (ba *ByzantineAttacker) Start() error {
	log.Println("Starting Byzantine attack simulation...")
	log.Printf("  Malicious validators: %d\n", ba.config.NumMaliciousNodes)
	log.Printf("  Duplicate votes per round: %d\n", ba.config.DuplicateVotesPerRound)
	log.Printf("  Attack duration: %s\n", ba.config.AttackDuration)

	// Start monitoring consensus
	ba.wg.Add(1)
	go ba.monitorConsensus()

	// Start malicious validators
	for i, key := range ba.maliciousKeys {
		ba.wg.Add(1)
		go ba.runMaliciousValidator(i, key)
	}

	// Wait for duration or cancellation
	select {
	case <-time.After(ba.config.AttackDuration):
		log.Println("Attack duration completed")
	case <-ba.ctx.Done():
		log.Println("Attack cancelled")
	}

	ba.cancel()
	ba.wg.Wait()

	return nil
}

// Stop stops the attack.
func (ba *ByzantineAttacker) Stop() {
	ba.cancel()
}

// runMaliciousValidator simulates a single malicious validator sending duplicate votes.
func (ba *ByzantineAttacker) runMaliciousValidator(id int, key ed25519.PrivateKey) {
	defer ba.wg.Done()

	pub := key.Public().(ed25519.PublicKey)
	log.Printf("Malicious validator %d starting: %x...\n", id, pub[:8])

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	round := uint64(0)

	for {
		select {
		case <-ba.ctx.Done():
			return
		case <-ticker.C:
			// Send duplicate votes for current round
			ba.sendDuplicateVotes(id, key, round)
			round++
		}
	}
}

// sendDuplicateVotes sends multiple duplicate votes for the same round/header.
// This tests issue #3869 - duplicate vote detection and handling.
func (ba *ByzantineAttacker) sendDuplicateVotes(validatorID int, key ed25519.PrivateKey, round uint64) {
	pub := key.Public().(ed25519.PublicKey)

	// Create a fake header digest
	var headerDigest types.HeaderDigest
	copy(headerDigest[:], pub) // Use pub key as fake digest for uniqueness

	epoch := uint64(0) // Assume epoch 0 for simplicity

	// Send duplicate votes
	for i := 0; i < ba.config.DuplicateVotesPerRound; i++ {
		vote := types.NewVote(headerDigest, types.Round(round), epoch, pub)
		if err := vote.Sign(key); err != nil {
			log.Printf("Validator %d: failed to sign vote: %v\n", validatorID, err)
			continue
		}

		// In a real scenario, we would broadcast this vote via gossip
		// For this simulation, we just log it
		if i == 0 {
			log.Printf("Validator %d: sending %d duplicate votes for round %d\n",
				validatorID, ba.config.DuplicateVotesPerRound, round)
		}
	}
}

// monitorConsensus periodically checks consensus progress and collects metrics.
func (ba *ByzantineAttacker) monitorConsensus() {
	defer ba.wg.Done()

	ticker := time.NewTicker(ba.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ba.ctx.Done():
			return
		case <-ticker.C:
			metrics, err := ba.fetchConsensusMetrics()
			if err != nil {
				log.Printf("Failed to fetch metrics: %v\n", err)
				continue
			}

			ba.mu.Lock()
			ba.metricsHistory = append(ba.metricsHistory, metrics)
			ba.mu.Unlock()

			log.Printf("Consensus progress: Round=%d, Epoch=%d, Certs=%d, Votes=%d\n",
				metrics.Round, metrics.Epoch, metrics.CertificatesCreated, metrics.VotesProcessed)
		}
	}
}

// fetchConsensusMetrics queries the target node's API for consensus metrics.
func (ba *ByzantineAttacker) fetchConsensusMetrics() (ConsensusMetrics, error) {
	// In a real implementation, this would query the node's API
	// For simulation, return mock data
	return ConsensusMetrics{
		Round:            0,
		Epoch:            0,
		CertificatesCreated: 0,
		VotesProcessed:   0,
		HonestValidators: 25 - ba.config.NumMaliciousNodes,
		TotalValidators:  25,
		Timestamp:        time.Now(),
	}, nil
}

// GenerateReport creates a summary report of the attack results.
func (ba *ByzantineAttacker) GenerateReport() string {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	if len(ba.metricsHistory) == 0 {
		return "No metrics collected"
	}

	initial := ba.metricsHistory[0]
	final := ba.metricsHistory[len(ba.metricsHistory)-1]

	report := fmt.Sprintf(`
Byzantine Attack Test Report
============================

Attack Configuration:
  - Malicious Validators: %d
  - Honest Validators: %d
  - Total Validators: %d
  - Byzantine Fault Percentage: %.1f%%
  - Duplicate Votes Per Round: %d
  - Attack Duration: %s

Consensus Progress:
  - Initial Round: %d
  - Final Round: %d
  - Rounds Advanced: %d
  - Certificates Created: %d
  - Votes Processed: %d

Result:
  %s

Recommendation:
  - With %d Byzantine validators (%.1f%% fault), honest validators should maintain consensus
  - Duplicate votes should be detected and rejected (Issue #3869)
  - Vote processing should remain efficient despite spam
`,
		ba.config.NumMaliciousNodes,
		final.HonestValidators,
		final.TotalValidators,
		float64(ba.config.NumMaliciousNodes)/float64(final.TotalValidators)*100,
		ba.config.DuplicateVotesPerRound,
		ba.config.AttackDuration,
		initial.Round,
		final.Round,
		final.Round-initial.Round,
		final.CertificatesCreated,
		final.VotesProcessed,
		ba.evaluateResult(initial, final),
		ba.config.NumMaliciousNodes,
		float64(ba.config.NumMaliciousNodes)/float64(final.TotalValidators)*100,
	)

	return report
}

// evaluateResult determines if consensus continued despite Byzantine attack.
func (ba *ByzantineAttacker) evaluateResult(initial, final ConsensusMetrics) string {
	if final.Round > initial.Round {
		return "SUCCESS: Consensus continued despite Byzantine attack"
	}
	return "FAILURE: Consensus stalled under Byzantine attack"
}

// SaveReportToFile writes the report to a file.
func (ba *ByzantineAttacker) SaveReportToFile(filename string) error {
	report := ba.GenerateReport()
	return os.WriteFile(filename, []byte(report), 0644)
}

func main() {
	config := ByzantineAttackConfig{
		NumMaliciousNodes:      9,  // 36% Byzantine fault (9 out of 25)
		DuplicateVotesPerRound: 10, // Each malicious validator sends 10 duplicate votes
		TargetAPIEndpoint:      "http://localhost:8080",
		AttackDuration:         5 * time.Minute,
		MonitorInterval:        10 * time.Second,
	}

	// Override with environment variables if present
	if envNodes := os.Getenv("MALICIOUS_NODES"); envNodes != "" {
		fmt.Sscanf(envNodes, "%d", &config.NumMaliciousNodes)
	}
	if envDupes := os.Getenv("DUPLICATE_VOTES"); envDupes != "" {
		fmt.Sscanf(envDupes, "%d", &config.DuplicateVotesPerRound)
	}
	if envDuration := os.Getenv("ATTACK_DURATION"); envDuration != "" {
		if d, err := time.ParseDuration(envDuration); err == nil {
			config.AttackDuration = d
		}
	}

	attacker := NewByzantineAttacker(config)

	if err := attacker.Initialize(); err != nil {
		log.Fatalf("Failed to initialize attacker: %v\n", err)
	}

	// Handle SIGINT/SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received interrupt signal, stopping attack...")
		attacker.Stop()
	}()

	if err := attacker.Start(); err != nil {
		log.Fatalf("Attack failed: %v\n", err)
	}

	// Generate and save report
	report := attacker.GenerateReport()
	fmt.Println(report)

	reportFile := "/tmp/byzantine-attack-report.txt"
	if err := attacker.SaveReportToFile(reportFile); err != nil {
		log.Printf("Failed to save report: %v\n", err)
	} else {
		log.Printf("Report saved to %s\n", reportFile)
	}
}
