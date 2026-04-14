package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	workersPerNode = 4
	totalNodes     = 12
)

// Dynamic TPS control
var currentTPS atomic.Int64
var targetTPS atomic.Int64
var peakTPS atomic.Int64
var lastErrorRatePercent atomic.Int64 // stored as int64(rate * 100)

// Account pool for moving tokens
type accountPool struct {
	mu       sync.RWMutex
	accounts []liteAccount
	nextIdx  atomic.Uint64
}

type liteAccount struct {
	key ed25519.PrivateKey
	url *url.URL
}

func (p *accountPool) add(acc liteAccount) {
	p.mu.Lock()
	p.accounts = append(p.accounts, acc)
	p.mu.Unlock()
}

func (p *accountPool) size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.accounts)
}

func (p *accountPool) getRandom() liteAccount {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.accounts) == 0 {
		return liteAccount{}
	}
	idx := rand.Intn(len(p.accounts))
	return p.accounts[idx]
}

func (p *accountPool) getNext() liteAccount {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.accounts) == 0 {
		return liteAccount{}
	}
	idx := p.nextIdx.Add(1) % uint64(len(p.accounts))
	return p.accounts[idx]
}

func main() {
	startTPS := flag.Int("start-tps", 15000, "Starting TPS (will ramp down to min-tps first)")
	minTPS := flag.Int("min-tps", 10000, "Minimum TPS to ramp down to before ramping up")
	maxTPS := flag.Int("max-tps", 50000, "Maximum TPS to test before stopping")
	rampDownDuration := flag.Duration("ramp-down-duration", 1*time.Minute, "Duration to ramp from start to min TPS")
	rampUpDuration := flag.Duration("ramp-up-duration", 30*time.Second, "Duration to ramp TPS by 1000 while looking for pushback")
	errorThreshold := flag.Float64("error-threshold", 0.05, "Error rate threshold to detect pushback (0.05 = 5%)")
	controlPort := flag.Int("control-port", 8099, "HTTP port for dynamic TPS control")
	duration := flag.Duration("duration", 0, "Test duration (0 for unlimited)")
	tokensPerWorker := flag.Int64("tokens-per-worker", 10000000, "ACME tokens to allocate per worker")
	maxAccounts := flag.Int("max-accounts", 1000000, "Maximum lite accounts in pool")
	flag.Parse()

	currentTPS.Store(int64(*startTPS))
	targetTPS.Store(int64(*maxTPS))

	// Derive funding account from genesis seed "FAUCET"
	var fundingSeed [32]byte
	fundingSeed = sha256.Sum256(append(fundingSeed[:], []byte("FAUCET")...))
	fundingKey := ed25519.NewKeyFromSeed(fundingSeed[:])
	fundingURL, err := protocol.LiteTokenAddress(fundingKey[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Printf("ERROR: Failed to create funding URL: %v\n", err)
		os.Exit(1)
	}

	// 12 validator API endpoints
	nodes := []string{
		"http://localhost:26660/v3",
		"http://localhost:26661/v3",
		"http://localhost:26662/v3",
		"http://localhost:26663/v3",
		"http://localhost:26664/v3",
		"http://localhost:26665/v3",
		"http://localhost:26666/v3",
		"http://localhost:26667/v3",
		"http://localhost:26668/v3",
		"http://localhost:26669/v3",
		"http://localhost:26670/v3",
		"http://localhost:26671/v3",
	}

	totalWorkers := len(nodes) * workersPerNode
	fmt.Printf("Accumulate Load Test\n")
	fmt.Printf("====================\n")
	fmt.Printf("Workers: %d (%d nodes x %d workers/node)\n", totalWorkers, len(nodes), workersPerNode)
	fmt.Printf("Funding account: %s\n", fundingURL)
	fmt.Printf("TPS strategy: %d (start) -> %d (min, after %v) -> ramp up (every %v) until pushback >%.1f%% errors\n",
		*startTPS, *minTPS, *rampDownDuration, *rampUpDuration, (*errorThreshold)*100)
	fmt.Printf("Max accounts: %d\n", *maxAccounts)
	fmt.Printf("Control endpoint: http://localhost:%d/tps\n\n", *controlPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize account pool
	pool := &accountPool{}

	// Generate worker accounts
	fmt.Println("Generating worker accounts...")
	workerAccounts := make([]liteAccount, totalWorkers)
	for i := 0; i < totalWorkers; i++ {
		_, sk, _ := ed25519.GenerateKey(nil)
		workerAccounts[i].key = sk
		workerAccounts[i].url, _ = protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
		pool.add(workerAccounts[i])
	}

	// Fund worker accounts
	fmt.Printf("Funding %d worker accounts with %d ACME each...\n", totalWorkers, *tokensPerWorker)
	client := jsonrpc.NewClient(nodes[0])
	nonce := uint64(time.Now().UnixMilli())

	for i, wa := range workerAccounts {
		env, err := build.Transaction().
			For(fundingURL).
			SendTokens(uint64(*tokensPerWorker), 0).To(wa.url).
			SignWith(fundingURL).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(fundingKey).
			Done()
		if err != nil {
			fmt.Printf("ERROR building funding tx %d: %v\n", i, err)
			os.Exit(1)
		}

		reqCtx, reqCancel := context.WithTimeout(ctx, 30*time.Second)
		subs, err := client.Submit(reqCtx, env, api.SubmitOptions{})
		reqCancel()
		if err != nil {
			fmt.Printf("ERROR submitting funding tx %d: %v\n", i, err)
			os.Exit(1)
		}
		if len(subs) == 0 || !subs[0].Success {
			msg := "unknown"
			if len(subs) > 0 {
				msg = subs[0].Message
			}
			fmt.Printf("ERROR: funding tx %d failed: %s\n", i, msg)
			os.Exit(1)
		}
		fmt.Printf("  Worker %d funded: %s\n", i+1, wa.url)
	}

	fmt.Println("Waiting for funding transactions to settle...")
	time.Sleep(5 * time.Second)

	// Global counters
	var submitted, succeeded, failed atomic.Uint64
	var wg sync.WaitGroup

	// Adaptive TPS ramping goroutine: down to min, then up until pushback detected
	rampStart := time.Now()
	peakTPS.Store(int64(*startTPS))
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		lastCheckTime := time.Now()
		lastCheckSubmitted := uint64(0)
		lastCheckSucceeded := uint64(0)

		phase := "ramp-down" // ramp-down, hold-min, or ramp-up
		rampUpStartTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(rampStart)
				now := time.Now()

				// Phase 1: Ramp down to minimum TPS
				if phase == "ramp-down" && elapsed < *rampDownDuration {
					progress := float64(elapsed) / float64(*rampDownDuration)
					newTPS := float64(*startTPS) - progress*float64(*startTPS-*minTPS)
					currentTPS.Store(int64(newTPS))
				} else if phase == "ramp-down" {
					// Transition to holding at minimum
					currentTPS.Store(int64(*minTPS))
					peakTPS.Store(int64(*minTPS))
					phase = "hold-min"
					rampUpStartTime = now
					lastCheckTime = now
					lastCheckSubmitted = submitted.Load()
					lastCheckSucceeded = succeeded.Load()
					fmt.Printf("[%v] Reached minimum TPS of %d. Starting ramp-up phase...\n", elapsed.Round(time.Second), *minTPS)
				}

				// Phase 2: Ramp up, checking for pushback
				if phase == "hold-min" || phase == "ramp-up" {
					phase = "ramp-up"

					// Every rampUpDuration, check error rate and increase TPS if healthy
					if now.Sub(rampUpStartTime) >= *rampUpDuration {
						timeSinceLastCheck := now.Sub(lastCheckTime).Seconds()
						if timeSinceLastCheck > 0 {
							totalSubmitted := submitted.Load() - lastCheckSubmitted
							totalSucceeded := succeeded.Load() - lastCheckSucceeded

							// Protect against underflow if succeeded > submitted (race condition)
							var totalFailed uint64
							if totalSucceeded > totalSubmitted {
								// Shouldn't happen, but log it and treat as no failures
								fmt.Printf("WARNING: succeeded (%d) > submitted (%d), clamping failures to 0\n", totalSucceeded, totalSubmitted)
								totalFailed = 0
								totalSubmitted = totalSucceeded // Use actual succeeded count as baseline
							} else {
								totalFailed = totalSubmitted - totalSucceeded
							}

							errorRate := 0.0
							if totalSubmitted > 0 {
								errorRate = float64(totalFailed) / float64(totalSubmitted)
							}
							lastErrorRatePercent.Store(int64(errorRate * 100))

							fmt.Printf("[%v] Error rate: %.2f%% (%d failed/%d submitted) | Current TPS: %d\n",
								elapsed.Round(time.Second), errorRate*100, totalFailed, totalSubmitted, currentTPS.Load())

							// If error rate is acceptable, try higher TPS
							if errorRate < *errorThreshold && currentTPS.Load() < int64(*maxTPS) {
								newTPS := currentTPS.Load() + 1000
								if newTPS > int64(*maxTPS) {
									newTPS = int64(*maxTPS)
								}
								currentTPS.Store(newTPS)
								peakTPS.Store(newTPS)
								fmt.Printf("  -> Increasing TPS to %d (error rate OK)\n", newTPS)
								rampUpStartTime = now
								lastCheckTime = now
								lastCheckSubmitted = submitted.Load()
								lastCheckSucceeded = succeeded.Load()
							} else if errorRate >= *errorThreshold {
								// Pushback detected! Stay at current TPS
								fmt.Printf("  -> PUSHBACK DETECTED! Holding at %d TPS\n", currentTPS.Load())
								peakTPS.Store(currentTPS.Load())
							} else {
								// Reached max TPS
								fmt.Printf("  -> Reached maximum TPS limit of %d\n", *maxTPS)
							}
						}
					}
				}
			}
		}
	}()

	// HTTP control server
	mux := http.NewServeMux()
	mux.HandleFunc("/tps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.URL.Query().Has("v") {
			vStr := r.URL.Query().Get("v")
			v, err := strconv.ParseInt(vStr, 10, 64)
			if err != nil || v < 0 {
				http.Error(w, "invalid TPS value", http.StatusBadRequest)
				return
			}
			old := currentTPS.Swap(v)
			fmt.Printf("TPS changed: %d -> %d\n", old, v)
			fmt.Fprintf(w, "TPS changed: %d -> %d\n", old, v)
			return
		}
		fmt.Fprintf(w, "Current TPS: %d\nTarget TPS: %d\nAccounts: %d\n",
			currentTPS.Load(), targetTPS.Load(), pool.size())
	})

	controlServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *controlPort),
		Handler: mux,
	}
	go func() {
		if err := controlServer.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("Control server error: %v\n", err)
		}
	}()

	// Progress reporter
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	start := time.Now()

	type sample struct {
		time      time.Time
		submitted uint64
	}
	samples := make([]sample, 0, 100)

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

				tpsSinceLaunch := float64(s) / elapsed.Seconds()

				samples = append(samples, sample{time: now, submitted: s})

				// Keep last 15 minutes
				cutoff := now.Add(-15 * time.Minute)
				firstValid := 0
				for i, samp := range samples {
					if samp.time.After(cutoff) {
						firstValid = i
						break
					}
				}
				if firstValid > 0 {
					samples = samples[firstValid:]
				}

				// Calculate TPS for different windows
				var tps1min, tps5min, tps15min float64
				for _, window := range []struct {
					dur time.Duration
					ptr *float64
				}{
					{60 * time.Second, &tps1min},
					{5 * time.Minute, &tps5min},
					{15 * time.Minute, &tps15min},
				} {
					windowCutoff := now.Add(-window.dur)
					var oldest *sample
					for i := range samples {
						if samples[i].time.After(windowCutoff) {
							oldest = &samples[i]
							break
						}
					}
					if oldest != nil && len(samples) > 0 {
						newest := samples[len(samples)-1]
						dur := newest.time.Sub(oldest.time).Seconds()
						if dur > 0 {
							*window.ptr = float64(newest.submitted-oldest.submitted) / dur
						}
					}
				}

				totalFailed := failed.Load()
				errorRate := 0.0
				if s > 0 {
					errorRate = float64(totalFailed) / float64(s) * 100
				}
				fmt.Printf("Progress: submitted=%d success=%d failure=%d (%.2f%% error) elapsed=%v tps_1min=%.0f tps_5min=%.0f tps_15min=%.0f tps_total=%.0f current_target=%d accounts=%d\n",
					s, succeeded.Load(), totalFailed, errorRate,
					elapsed.Round(time.Second), tps1min, tps5min, tps15min, tpsSinceLaunch,
					currentTPS.Load(), pool.size())
			}
		}
	}()

	fmt.Println("Starting load generators...")

	// Shared HTTP client
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        500,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 50,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
		Timeout: 30 * time.Second,
	}

	// Start workers
	for i := 0; i < totalWorkers; i++ {
		nodeURL := nodes[i%len(nodes)]
		wg.Add(1)
		go func(workerID int, nodeURL string, account liteAccount) {
			defer wg.Done()
			runWorker(ctx, workerID, nodeURL, totalWorkers, account, pool, *maxAccounts, httpClient, &submitted, &succeeded, &failed)
		}(i+1, nodeURL, workerAccounts[i])
	}

	fmt.Printf("All %d workers started.\n\n", totalWorkers)

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if *duration > 0 {
		timer := time.NewTimer(*duration)
		defer timer.Stop()
		select {
		case <-timer.C:
			fmt.Printf("\n%v test duration reached. Shutting down...\n", *duration)
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v. Shutting down...\n", sig)
		}
	} else {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v. Shutting down...\n", sig)
	}

	cancel()
	controlServer.Shutdown(context.Background())

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All workers stopped.")
	case <-time.After(10 * time.Second):
		fmt.Println("Warning: Some workers did not stop within timeout.")
	}

	httpClient.CloseIdleConnections()

	elapsed := time.Since(start)
	s := submitted.Load()
	succeedCount := succeeded.Load()
	failCount := failed.Load()
	tps := float64(s) / elapsed.Seconds()
	errorRate := 0.0
	if s > 0 {
		errorRate = float64(failCount) / float64(s) * 100
	}
	fmt.Printf("\n=== FINAL RESULTS ===\n")
	fmt.Printf("  Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("  Submitted: %d\n", s)
	fmt.Printf("  Success: %d\n", succeedCount)
	fmt.Printf("  Failed: %d (%.2f%%)\n", failCount, errorRate)
	fmt.Printf("  Average TPS: %.2f\n", tps)
	fmt.Printf("  Peak TPS Reached: %d\n", peakTPS.Load())
	fmt.Printf("  Last Error Rate: %.2f%%\n", float64(lastErrorRatePercent.Load()))
	fmt.Printf("  Total Accounts: %d\n", pool.size())
	fmt.Println("\nTest complete.")
}

func runWorker(ctx context.Context, workerID int, nodeURL string, totalWorkers int, account liteAccount, pool *accountPool, maxAccounts int, httpClient *http.Client, submitted, succeeded, failed *atomic.Uint64) {
	client := jsonrpc.NewClient(nodeURL)
	nonce := uint64(time.Now().UnixMilli()) + uint64(workerID*1000000)
	localRand := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		loopStart := time.Now()

		tps := currentTPS.Load()
		if tps <= 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		tpsPerWorker := tps / int64(totalWorkers)
		if tpsPerWorker < 1 {
			tpsPerWorker = 1
		}
		targetInterval := time.Second / time.Duration(tpsPerWorker)

		// Decide whether to create new account or reuse existing
		var destURL *url.URL
		poolSize := pool.size()

		if poolSize < maxAccounts && localRand.Float32() < 0.3 {
			// 30% chance to create new account if under limit
			_, destKey, _ := ed25519.GenerateKey(localRand)
			destURL, _ = protocol.LiteTokenAddress(destKey[32:], "ACME", protocol.SignatureTypeED25519)
			pool.add(liteAccount{key: destKey, url: destURL})
		} else if poolSize > 0 {
			// Use existing account from pool
			dest := pool.getRandom()
			destURL = dest.url
		} else {
			// Fallback: create new account
			_, destKey, _ := ed25519.GenerateKey(localRand)
			destURL, _ = protocol.LiteTokenAddress(destKey[32:], "ACME", protocol.SignatureTypeED25519)
			pool.add(liteAccount{key: destKey, url: destURL})
		}

		if destURL == nil {
			continue
		}

		// Send 1 ACME
		env, err := build.Transaction().
			For(account.url).
			SendTokens(1, 0).To(destURL).
			SignWith(account.url).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(account.key).
			Done()

		if err != nil {
			failed.Add(1)
			continue
		}

		submitted.Add(1)

		reqCtx, reqCancel := context.WithTimeout(ctx, 10*time.Second)
		_, err = client.Submit(reqCtx, env, api.SubmitOptions{})
		reqCancel()

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failed.Add(1)
		} else {
			succeeded.Add(1)
		}

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
