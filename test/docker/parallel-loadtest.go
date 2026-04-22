// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const recvStride = 7 // stride for destination walk — coprime to account count

func main() {
	var (
		nodesFlag     string
		numWorkers    int
		startTPS      int
		maxTPS        int
		rampInterval  time.Duration
		rampStep      int
		duration      time.Duration
		label         string
		statusDir     string
		faucetSeed    string
		totalAccounts int
		fundingAmount uint64
		oraclePrice   float64
		errorCutoff   float64
	)

	flag.StringVar(&nodesFlag, "nodes", "", "Comma-separated list of node API endpoints")
	flag.IntVar(&numWorkers, "workers", 16, "Number of worker goroutines (distributed evenly across nodes)")
	flag.IntVar(&startTPS, "start-tps", 1000, "Starting TPS")
	flag.IntVar(&maxTPS, "max-tps", 25000, "Maximum TPS")
	flag.IntVar(&rampStep, "ramp-step", 2000, "TPS increase per ramp interval")
	flag.DurationVar(&rampInterval, "ramp-interval", 30*time.Second, "Time between TPS increases")
	flag.DurationVar(&duration, "duration", 5*time.Minute, "Total test duration")
	flag.StringVar(&label, "label", "", "Test label shown on dashboard")
	flag.StringVar(&statusDir, "status-dir", "/tmp/loadtest-workspace", "Directory for status.json")
	flag.StringVar(&faucetSeed, "faucet-seed", "FAUCET", "Faucet seed (must match init network)")
	flag.IntVar(&totalAccounts, "accounts", 1000, "Total sender accounts (split across workers)")
	flag.Uint64Var(&fundingAmount, "fund-amount", 100000, "Whole ACME per funder")
	flag.Float64Var(&oraclePrice, "oracle", 1000, "ACME oracle price in USD")
	flag.Float64Var(&errorCutoff, "error-cutoff", 5.0, "Stop ramping at this error percentage")
	flag.Parse()

	if totalAccounts < 1 {
		fmt.Fprintf(os.Stderr, "accounts must be >= 1\n")
		os.Exit(1)
	}

	// Parse node list
	var nodes []string
	if nodesFlag != "" {
		for _, n := range strings.Split(nodesFlag, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				nodes = append(nodes, n)
			}
		}
	}
	if len(nodes) == 0 {
		for port := 26660; port <= 26671; port++ {
			nodes = append(nodes, fmt.Sprintf("http://localhost:%d/v3", port))
		}
	}

	if numWorkers < 16 {
		numWorkers = 16
	}

	// Ensure account count per worker is coprime to recvStride
	acctPerWorker := totalAccounts / numWorkers
	for acctPerWorker%recvStride == 0 {
		acctPerWorker++
	}

	fmt.Printf("Load test: %d workers across %d nodes\n", numWorkers, len(nodes))
	fmt.Printf("TPS ramp: %d → %d, +%d every %v (cutoff at %.1f%% errors)\n",
		startTPS, maxTPS, rampStep, rampInterval, errorCutoff)
	fmt.Printf("Accounts: %d per worker (%d total), coprime to stride %d\n",
		acctPerWorker, acctPerWorker*numWorkers, recvStride)
	fmt.Printf("Duration: %v\n\n", duration)

	// Derive faucet
	faucetSK := deriveKey(faucetSeed)
	faucetURL, err := protocol.LiteTokenAddress(faucetSK[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Faucet key error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Faucet: %v\n", faucetURL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Seed funder accounts from faucet (fast, no waiting) ──
	// Submit funding + credit purchases back to back. Workers start
	// immediately — early sender funding may fail if funder hasn't
	// settled yet, but the worker just retries on the next pass.
	fmt.Printf("Seeding %d funder accounts from faucet...\n", numWorkers)
	type funderAcct struct {
		sk  ed25519.PrivateKey
		url *url.URL
	}
	funders := make([]funderAcct, numWorkers)
	{
		client := jsonrpc.NewClient(nodes[0])
		nonce := uint64(time.Now().UnixMilli())
		for i := 0; i < numWorkers; i++ {
			sk := deriveKey(faucetSeed, "funder", i)
			u, _ := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
			funders[i] = funderAcct{sk: sk, url: u}

			// Send ACME to funder
			env, _ := build.Transaction().
				For(faucetURL).
				SendTokens(fundingAmount, protocol.AcmePrecisionPower).To(u).
				SignWith(faucetURL).
				Version(1).
				Timestamp(&nonce).
				PrivateKey(faucetSK).
				Done()
			if env != nil {
				submit(ctx, client, env)
			}

			// Buy credits for funder (AddCredits doesn't need credits)
			env, _ = build.Transaction().
				For(u).
				AddCredits().
				WithOracle(oraclePrice).
				Spend(3).
				To(u.RootIdentity()).
				SignWith(u).
				Version(1).
				Timestamp(&nonce).
				PrivateKey(sk).
				Done()
			if env != nil {
				submit(ctx, client, env)
			}
		}
		fmt.Printf("Seeded %d funders. Workers starting immediately.\n", numWorkers)
	}

	// ── Shared TPS target — ramps up over time ──
	var currentTargetTPS atomic.Int64
	currentTargetTPS.Store(int64(startTPS))

	// ── Counters ──
	var submitted, succeeded, failed atomic.Uint64
	var wg sync.WaitGroup

	// Status reporter + dashboard writer
	_ = os.MkdirAll(statusDir, 0755)
	statusFile := filepath.Join(statusDir, "status.json")
	logFile := filepath.Join(statusDir, "log.jsonl")
	var peakTPS float64 // only accessed by ticker goroutine + main after wg.Wait
	start := time.Now()

	// Windowed TPS tracking (15-second window)
	var prevSubmitted uint64
	var prevTime = start

	// Write initial log entry
	appendLog(logFile, fmt.Sprintf("Test started: %s", label))

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				elapsed := now.Sub(start)
				s := submitted.Load()
				su := succeeded.Load()
				f := failed.Load()

				// Reset window every 15 seconds BEFORE calculating TPS
				if now.Sub(prevTime) >= 15*time.Second {
					prevSubmitted = s
					prevTime = now
				}

				// Windowed TPS: transactions in last window / window duration
				windowDuration := now.Sub(prevTime).Seconds()
				windowTxns := s - prevSubmitted
				var windowTPS float64
				if windowDuration > 0 {
					windowTPS = float64(windowTxns) / windowDuration
				}

				avgTPS := float64(s) / elapsed.Seconds()
				if windowTPS > peakTPS {
					peakTPS = windowTPS
				}
				var errorRate float64
				if s > 0 {
					errorRate = float64(f) / float64(s) * 100
				}
				target := currentTargetTPS.Load()
				fmt.Printf("[%v] target=%d window_tps=%.0f avg_tps=%.0f submitted=%d success=%d failed=%d err=%.2f%%\n",
					elapsed.Round(time.Second), target, windowTPS, avgTPS, s, su, f, errorRate)

				status := map[string]any{
					"stale":       false,
					"test_label":  label,
					"current_tps": windowTPS,
					"target_tps":  target,
					"peak_tps":    peakTPS,
					"submitted":   s,
					"succeeded":   su,
					"failed":      f,
					"error_rate":  errorRate,
					"avg_tps":     avgTPS,
					"elapsed":     elapsed.Round(time.Second).String(),
					"workers":     numWorkers,
					"nodes":       len(nodes),
				}
				if b, err := json.Marshal(status); err == nil {
					_ = os.WriteFile(statusFile, b, 0644)
				}
			}
		}
	}()

	// TPS ramper — increases target TPS on interval until max or error cutoff
	wg.Add(1)
	go func() {
		defer wg.Done()
		rampTicker := time.NewTicker(rampInterval)
		defer rampTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-rampTicker.C:
				s := submitted.Load()
				f := failed.Load()
				if s > 0 {
					errPct := float64(f) / float64(s) * 100
					if errPct > errorCutoff {
						msg := fmt.Sprintf("Error rate %.2f%% exceeds cutoff %.1f%%, holding TPS", errPct, errorCutoff)
						fmt.Printf("── %s ──\n", msg)
						appendLog(logFile, msg)
						continue
					}
				}
				cur := currentTargetTPS.Load()
				next := cur + int64(rampStep)
				if next > int64(maxTPS) {
					next = int64(maxTPS)
				}
				if next != cur {
					currentTargetTPS.Store(next)
					msg := fmt.Sprintf("Ramping TPS: %d -> %d", cur, next)
					fmt.Printf("── %s ──\n", msg)
					appendLog(logFile, msg)
				}
			}
		}
	}()

	// ── Start workers ──
	// Distribute workers evenly across nodes: worker i → nodes[i % len(nodes)]
	for i := 0; i < numWorkers; i++ {
		nodeURL := nodes[i%len(nodes)]
		funder := funders[i]
		wg.Add(1)
		go func(wID int, nURL string, funder funderAcct) {
			defer wg.Done()
			runWorker(ctx, wID, nURL, faucetSeed, funder, acctPerWorker,
				fundingAmount, oraclePrice, numWorkers, &currentTargetTPS,
				&submitted, &succeeded, &failed)
		}(i, nodeURL, funder)
	}

	fmt.Printf("\n── %d workers started across %d nodes. Running for %v ──\n\n", numWorkers, len(nodes), duration)

	// Wait for duration or signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		fmt.Printf("\n%v reached. Shutting down...\n", duration)
	case sig := <-sigChan:
		fmt.Printf("\nSignal %v. Shutting down...\n", sig)
	}
	cancel()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		fmt.Println("All workers stopped.")
	case <-time.After(10 * time.Second):
		fmt.Println("Warning: some workers did not stop in time.")
	}

	elapsed := time.Since(start)
	s := submitted.Load()
	tps := float64(s) / elapsed.Seconds()
	fmt.Printf("\nFinal Results:\n")
	fmt.Printf("  Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("  Submitted: %d\n", s)
	fmt.Printf("  Success: %d\n", succeeded.Load())
	fmt.Printf("  Failed: %d\n", failed.Load())
	fmt.Printf("  Peak TPS: %.0f\n", peakTPS)
	fmt.Printf("  Average TPS: %.2f\n", tps)
	fmt.Println("\nTest complete!")
}

// appendLog appends a timestamped JSON log entry to a JSONL file.
func appendLog(path string, msg string) {
	entry := map[string]string{
		"time": time.Now().Format(time.RFC3339),
		"msg":  msg,
	}
	b, _ := json.Marshal(entry)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(b)
	_, _ = f.Write([]byte("\n"))
}

// deriveKey produces a deterministic ed25519 key from seed components.
func deriveKey(components ...any) ed25519.PrivateKey {
	var seed storage.Key
	for _, c := range components {
		seed = seed.Append(c)
	}
	return ed25519.NewKeyFromSeed(seed[:])
}

// acctSlot is a private key / address pair with funding state.
type acctSlot struct {
	privKey    ed25519.PrivateKey
	addr       *url.URL
	hasTokens  bool
	hasCredits bool
}

// submit sends a transaction envelope to the network. Returns true on success.
func submit(ctx context.Context, client *jsonrpc.Client, env *messaging.Envelope) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := client.Submit(reqCtx, env, api.SubmitOptions{})
	return err == nil
}

// runWorker walks through its account slots, funding on first use, then
// sending tokens at the per-worker share of the current TPS target.
// The TPS target is shared across all workers and ramps up over time.
func runWorker(ctx context.Context, workerID int, nodeURL string, faucetSeed string,
	funder struct {
		sk  ed25519.PrivateKey
		url *url.URL
	},
	acctCount int, fundingAmount uint64, oraclePrice float64, totalWorkers int,
	targetTPS *atomic.Int64,
	submitted, succeeded, failed *atomic.Uint64,
) {
	client := jsonrpc.NewClient(nodeURL)

	// Pre-compute account keys into flat array
	slots := make([]acctSlot, acctCount)
	for i := range slots {
		sk := deriveKey(faucetSeed, "sender", workerID, i)
		u, _ := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
		slots[i] = acctSlot{privKey: sk, addr: u}
	}

	nonce := uint64(time.Now().UnixMilli()) + uint64(workerID)*1000000
	sendIdx := 0
	recvIdx := 0

	perAccountFund := fundingAmount / uint64(acctCount)
	if perAccountFund < 2 {
		perAccountFund = 2
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		loopStart := time.Now()
		from := &slots[sendIdx]
		to := &slots[recvIdx]

		// Compute per-worker rate from shared target
		tpsPerWorker := int(targetTPS.Load()) / totalWorkers
		if tpsPerWorker < 1 {
			tpsPerWorker = 1
		}
		targetInterval := time.Second / time.Duration(tpsPerWorker)

		if !from.hasTokens {
			// Fund sender: send ACME from funder
			env, _ := build.Transaction().
				For(funder.url).
				SendTokens(perAccountFund, protocol.AcmePrecisionPower).To(from.addr).
				SignWith(funder.url).
				Version(1).
				Timestamp(&nonce).
				PrivateKey(funder.sk).
				Done()
			if env != nil {
				submitted.Add(1)
				if submit(ctx, client, env) {
					succeeded.Add(1)
				} else {
					failed.Add(1)
				}
			}
			from.hasTokens = true

		} else if !from.hasCredits {
			// Buy credits
			env, _ := build.Transaction().
				For(from.addr).
				AddCredits().
				WithOracle(oraclePrice).
				Spend(1).
				To(from.addr.RootIdentity()).
				SignWith(from.addr).
				Version(1).
				Timestamp(&nonce).
				PrivateKey(from.privKey).
				Done()
			if env != nil {
				submitted.Add(1)
				if submit(ctx, client, env) {
					succeeded.Add(1)
				} else {
					failed.Add(1)
				}
			}
			from.hasCredits = true

		} else {
			// Send tokens
			env, err := build.Transaction().
				For(from.addr).
				SendTokens(1, 0).To(to.addr).
				SignWith(from.addr).
				Version(1).
				Timestamp(&nonce).
				PrivateKey(from.privKey).
				Done()
			if err != nil {
				// Build error — don't count as submitted
				failed.Add(1)
			} else {
				submitted.Add(1)
				if submit(ctx, client, env) {
					succeeded.Add(1)
				} else {
					failed.Add(1)
				}
			}
		}

		if ctx.Err() != nil {
			return
		}

		// Advance walks
		sendIdx = (sendIdx + 1) % acctCount
		recvIdx = (recvIdx + recvStride) % acctCount

		// Rate limit based on current TPS target
		elapsed := time.Since(loopStart)
		if elapsed < targetInterval {
			select {
			case <-ctx.Done():
				return
			case <-time.After(targetInterval - elapsed):
			}
		}
	}
}
