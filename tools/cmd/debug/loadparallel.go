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
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// loadparallel runs N submitter workers across M endpoints. Each
// worker has its own pre-funded lite token account so nonces never
// collide. Aggregate target TPS is split across workers; each worker
// uses a ticker. A small grace period at startup lets every worker's
// funding settle before the load run begins.
//
// Funding source is the genesis FAUCET account derived from
// `--faucet-seed` (default "FAUCET").

var cmdLoadParallel = &cobra.Command{
	Use:   "loadparallel [endpoint,endpoint,...]",
	Short: "Multi-endpoint, multi-worker load generator",
	Args:  cobra.ExactArgs(1),
	Run:   loadParallel,
}

var loadParallelOpts struct {
	tps           int
	duration      time.Duration
	interval      time.Duration
	workers       int
	faucetSeed    string
	tokensPerW    uint64
	creditsPerW   float64
	fundingClient string
	ramp          string
	rungDur       time.Duration
}

func init() {
	cmdLoadParallel.Flags().IntVar(&loadParallelOpts.tps, "tps", 50, "Aggregate target transactions per second across all workers (single-rung mode)")
	cmdLoadParallel.Flags().DurationVar(&loadParallelOpts.duration, "duration", 30*time.Second, "Test duration (single-rung mode)")
	cmdLoadParallel.Flags().DurationVar(&loadParallelOpts.interval, "report-interval", 5*time.Second, "Report cadence")
	cmdLoadParallel.Flags().IntVar(&loadParallelOpts.workers, "workers-per-endpoint", 4, "Worker goroutines per endpoint")
	cmdLoadParallel.Flags().StringVar(&loadParallelOpts.faucetSeed, "faucet-seed", "FAUCET", "Genesis faucet seed")
	cmdLoadParallel.Flags().Uint64Var(&loadParallelOpts.tokensPerW, "tokens-per-worker", 1_000_000_000_000, "ACME (raw, 10^8/ACME) per worker")
	cmdLoadParallel.Flags().Float64Var(&loadParallelOpts.creditsPerW, "credits-per-worker", 100_000, "Credits to buy per worker (in 1/100 USD units)")
	cmdLoadParallel.Flags().StringVar(&loadParallelOpts.fundingClient, "funding-endpoint", "", "Endpoint to use for funding (defaults to first endpoint)")
	cmdLoadParallel.Flags().StringVar(&loadParallelOpts.ramp, "ramp", "", "Ramp mode: START:STEP:MAX (e.g. 5:5:1000). Funds workers once, then runs each TPS level until break.")
	cmdLoadParallel.Flags().DurationVar(&loadParallelOpts.rungDur, "rung-duration", 20*time.Second, "Duration per ramp rung")
	cmd.AddCommand(cmdLoadParallel)
}

func loadParallel(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawEndpoints := strings.Split(args[0], ",")
	endpoints := make([]string, 0, len(rawEndpoints))
	for _, e := range rawEndpoints {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		endpoints = append(endpoints, accumulate.ResolveWellKnownEndpoint(e, "v3"))
	}
	if len(endpoints) == 0 {
		fatalf("no endpoints")
	}

	totalWorkers := len(endpoints) * loadParallelOpts.workers
	tps := loadParallelOpts.tps
	if tps < 1 {
		fatalf("tps must be >= 1")
	}
	tpsPerWorker := float64(tps) / float64(totalWorkers)
	if tpsPerWorker < 0.001 {
		fatalf("tps/workers too low (would tick slower than 1000s)")
	}

	// Funding source: FAUCET key derived from the seed used by `init network`.
	fk := deriveFaucetKey(loadParallelOpts.faucetSeed)
	fundingURL, err := protocol.LiteTokenAddress(fk[32:], "ACME", protocol.SignatureTypeED25519)
	check(err)
	fundingLid := fundingURL.RootIdentity()

	fmt.Printf("Endpoints:    %v\n", endpoints)
	fmt.Printf("Workers:      %d (%d/endpoint)\n", totalWorkers, loadParallelOpts.workers)
	fmt.Printf("Target TPS:   %d (%.2f per worker)\n", tps, tpsPerWorker)
	fmt.Printf("Funding key:  %s\n", fundingURL)

	// Funding client: dedicated, separate from worker clients.
	fundEp := loadParallelOpts.fundingClient
	if fundEp == "" {
		fundEp = endpoints[0]
	}
	fundClient := jsonrpc.NewClient(fundEp)
	fundClient.Client.Timeout = 60 * time.Second

	Q := api.Querier2{Querier: fundClient}
	var fundAcct *protocol.LiteTokenAccount
	_, err = Q.QueryAccountAs(ctx, fundingURL, nil, &fundAcct)
	checkf(err, "query funding lite token account %v", fundingURL)
	fmt.Printf("Funding ACME: %v\n", fundAcct.TokenBalance())

	// Generate N worker keypairs.
	workers := make([]*workerCtx, totalWorkers)
	for i := 0; i < totalWorkers; i++ {
		_, sk, err := ed25519.GenerateKey(rand.Reader)
		check(err)
		workers[i] = &workerCtx{
			id:       i,
			endpoint: endpoints[i%len(endpoints)],
			sk:       sk,
		}
		workers[i].lta, err = protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
		check(err)
		workers[i].lid = workers[i].lta.RootIdentity()
	}

	// Fund every worker (sequential, monotonic nonce on the funding key).
	// Do not wait for synthetic delivery via the message-graph walk — that
	// produces O(N) synthetic-receipt waits which serialize. Instead, after
	// all submits, poll each worker's ACME balance until it's funded.
	fmt.Printf("Funding %d worker accounts...\n", totalWorkers)
	nonce := uint64(time.Now().UTC().UnixMilli())

	ns, err := fundClient.NetworkStatus(ctx, api.NetworkStatusOptions{})
	checkf(err, "network status")
	oraclePrice := float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision

	for _, w := range workers {
		nonce++
		env, err := build.Transaction().
			For(fundingURL).
			SendTokens(loadParallelOpts.tokensPerW, 0).To(w.lta).
			SignWith(fundingURL).Version(1).Timestamp(&nonce).PrivateKey(fk).
			Done()
		check(err)
		subs, err := fundClient.Submit(ctx, env, api.SubmitOptions{})
		checkf(err, "submit funding for worker %d", w.id)
		for _, s := range subs {
			if !s.Success {
				if s.Status != nil && s.Status.Error != nil {
					fatalf("funding worker %d: %v", w.id, s.Status.Error)
				}
				fatalf("funding worker %d: %s", w.id, s.Message)
			}
		}
	}
	fmt.Printf("Submitted %d funding txs; polling worker ACME balances...\n", totalWorkers)
	pollUntilFunded(ctx, Q, workers)

	// Buy credits for each worker (each pays from its own ACME).
	// Same approach: submit all, then poll each lite identity for credits.
	fmt.Printf("Buying credits for %d workers...\n", totalWorkers)
	for _, w := range workers {
		w.nonce = uint64(time.Now().UTC().UnixMilli())
		w.nonce++
		env, err := build.Transaction().
			For(w.lta).
			AddCredits().To(w.lid).Spend(loadParallelOpts.creditsPerW).
			WithOracle(oraclePrice).
			SignWith(w.lid).Version(1).Timestamp(&w.nonce).PrivateKey(w.sk).
			Done()
		check(err)
		subs, err := fundClient.Submit(ctx, env, api.SubmitOptions{})
		checkf(err, "submit credit-buy for worker %d", w.id)
		for _, s := range subs {
			if !s.Success {
				if s.Status != nil && s.Status.Error != nil {
					fatalf("credits worker %d: %v", w.id, s.Status.Error)
				}
				fatalf("credits worker %d: %s", w.id, s.Message)
			}
		}
	}
	fmt.Printf("Submitted %d credit txs; polling worker credit balances...\n", totalWorkers)
	pollUntilCredited(ctx, Q, workers)
	_ = fundingLid

	// Pre-build the lite data account address used by all workers.
	entry := new(protocol.DoubleHashDataEntry)
	entry.Data = append(entry.Data, []byte("loadparallel"))
	entry.Data = append(entry.Data, []byte(time.Now().Format(time.RFC3339Nano)))
	chainId := protocol.ComputeLiteDataAccountId(entry)
	lda, err := protocol.LiteDataAddress(chainId)
	check(err)

	// Pre-build per-worker JSON-RPC clients.
	for _, w := range workers {
		w.client = jsonrpc.NewClient(w.endpoint)
		w.client.Client.Timeout = 30 * time.Second
	}

	// Decide single-rung vs ramp mode.
	if loadParallelOpts.ramp != "" {
		runRamp(ctx, fundClient, workers, lda, entry)
		return
	}

	// Single-rung
	res := runRung(ctx, fundClient, workers, lda, entry, tps, loadParallelOpts.duration)
	res.print(tps, totalWorkers, loadParallelOpts.duration)
	if res.broken() {
		os.Exit(2)
	}
}

type rungResult struct {
	submitted, succeeded, mempoolFull, otherErr uint64
	startHeight, endHeight                      uint64
	startSyn, endSyn                            synthSnapshot
	elapsed                                     float64
}

type synthSnapshot struct {
	produced, received, delivered uint64
}

func (r *rungResult) print(tps, totalWorkers int, dur time.Duration) {
	fmt.Println()
	fmt.Println("=== summary ===")
	fmt.Printf("Target TPS:          %d\n", tps)
	fmt.Printf("Workers:             %d\n", totalWorkers)
	fmt.Printf("Duration:            %v\n", dur)
	fmt.Printf("Submitted total:     %d  (%.1f/s)\n", r.submitted, float64(r.submitted)/r.elapsed)
	fmt.Printf("Submit-success:      %d  (%.1f/s)\n", r.succeeded, float64(r.succeeded)/r.elapsed)
	fmt.Printf("Mempool full:        %d\n", r.mempoolFull)
	fmt.Printf("Other errors:        %d\n", r.otherErr)
	fmt.Printf("DN height start->end: %d -> %d  (Δ%d)\n", r.startHeight, r.endHeight, r.endHeight-r.startHeight)
	dProd := r.endSyn.produced - r.startSyn.produced
	dRecv := r.endSyn.received - r.startSyn.received
	dDeliv := r.endSyn.delivered - r.startSyn.delivered
	fmt.Printf("Synth Δ:             produced=%d  received=%d  delivered=%d\n", dProd, dRecv, dDeliv)
	if br, why := r.brokenWhy(); br {
		fmt.Println(why)
	}
}

func (r *rungResult) brokenWhy() (bool, string) {
	switch {
	case r.submitted == 0:
		return true, "BROKEN: nothing submitted"
	case float64(r.succeeded)/float64(r.submitted) < 0.95:
		return true, fmt.Sprintf("BROKEN: success rate %.1f%% < 95%%", 100*float64(r.succeeded)/float64(r.submitted))
	case r.mempoolFull > r.submitted/20:
		return true, fmt.Sprintf("BROKEN: mempool-full rate %.1f%% > 5%%", 100*float64(r.mempoolFull)/float64(r.submitted))
	case r.endHeight == r.startHeight:
		return true, "BROKEN: network height did not advance"
	}
	dRecv := r.endSyn.received - r.startSyn.received
	dDeliv := r.endSyn.delivered - r.startSyn.delivered
	if dRecv > 100 && float64(dDeliv)/float64(dRecv) < 0.90 {
		pct := 100 * float64(dDeliv) / float64(dRecv)
		return true, fmt.Sprintf("BROKEN: synthetic delivery behind (received Δ=%d delivered Δ=%d = %.1f%% < 90%%)",
			dRecv, dDeliv, pct)
	}
	return false, ""
}

func (r *rungResult) broken() bool {
	br, _ := r.brokenWhy()
	return br
}

// runRung runs a single TPS rung against an already-funded set of workers.
func runRung(ctx context.Context, fundClient *jsonrpc.Client, workers []*workerCtx,
	lda *url.URL, entry *protocol.DoubleHashDataEntry, tps int, dur time.Duration) *rungResult {

	totalWorkers := len(workers)
	tpsPerWorker := float64(tps) / float64(totalWorkers)

	var submitted, succeeded, mempoolFull, otherErr atomic.Uint64
	startStatus, _ := fundClient.NetworkStatus(ctx, api.NetworkStatusOptions{})
	res := &rungResult{}
	if startStatus != nil {
		res.startHeight = startStatus.DirectoryHeight
	}
	res.startSyn = querySynth(ctx, fundClient)

	tickInterval := time.Duration(float64(time.Second) / tpsPerWorker)
	if tickInterval <= 0 {
		tickInterval = time.Microsecond
	}
	fmt.Printf("Per-worker tick interval: %v\n", tickInterval)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, w := range workers {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(tickInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
				}
				w.nonce++
				env, err := build.Transaction().
					For(lda).
					WriteData(entry).
					SignWith(w.lid).Version(1).Timestamp(&w.nonce).PrivateKey(w.sk).
					Done()
				if err != nil {
					otherErr.Add(1)
					continue
				}
				m, err := env.Normalize()
				if err != nil {
					otherErr.Add(1)
					continue
				}
				submitted.Add(1)
				subs, err := w.client.Submit(ctx, &messaging.Envelope{Messages: m}, api.SubmitOptions{})
				if err != nil {
					if isMempoolFull(err) {
						mempoolFull.Add(1)
					} else {
						otherErr.Add(1)
					}
					continue
				}
				anyFailed := false
				for _, s := range subs {
					if s.Success {
						continue
					}
					anyFailed = true
					if s.Status != nil && s.Status.Error != nil && isMempoolFull(s.Status.Error) {
						mempoolFull.Add(1)
					}
				}
				if anyFailed {
					otherErr.Add(1)
				} else {
					succeeded.Add(1)
				}
			}
		}()
	}

	deadline := time.Now().Add(dur)
	rep := time.NewTicker(loadParallelOpts.interval)
	defer rep.Stop()
	var lastSub, lastSucc uint64
	lastT := time.Now()
	fmt.Printf("Running %d TPS for %v with %d workers\n", tps, dur, totalWorkers)
loop:
	for {
		t := <-rep.C
		s := submitted.Load()
		k := succeeded.Load()
		mf := mempoolFull.Load()
		oe := otherErr.Load()
		dt := t.Sub(lastT).Seconds()
		fmt.Printf("  submitted=%6d (%.1f/s)  ok=%6d (%.1f/s)  mempoolFull=%d  otherErr=%d\n",
			s, float64(s-lastSub)/dt, k, float64(k-lastSucc)/dt, mf, oe)
		lastSub, lastSucc, lastT = s, k, t
		if t.After(deadline) {
			break loop
		}
	}
	close(stop)
	wg.Wait()

	res.submitted = submitted.Load()
	res.succeeded = succeeded.Load()
	res.mempoolFull = mempoolFull.Load()
	res.otherErr = otherErr.Load()
	res.elapsed = dur.Seconds()
	res.endHeight = res.startHeight
	if st, err := fundClient.NetworkStatus(ctx, api.NetworkStatusOptions{}); err == nil {
		res.endHeight = st.DirectoryHeight
	}
	res.endSyn = querySynth(ctx, fundClient)
	return res
}

// runRamp iterates TPS levels per --ramp START:STEP:MAX, reusing the
// pre-funded worker pool. Stops at the first rung that breaks.
func runRamp(ctx context.Context, fundClient *jsonrpc.Client, workers []*workerCtx,
	lda *url.URL, entry *protocol.DoubleHashDataEntry) {
	parts := strings.Split(loadParallelOpts.ramp, ":")
	if len(parts) != 3 {
		fatalf("--ramp must be START:STEP:MAX (got %q)", loadParallelOpts.ramp)
	}
	start, step, maxR := atoiF(parts[0], "start"), atoiF(parts[1], "step"), atoiF(parts[2], "max")
	if start < 1 || step < 1 || maxR < start {
		fatalf("invalid ramp bounds: %d:%d:%d", start, step, maxR)
	}

	dur := loadParallelOpts.rungDur
	totalWorkers := len(workers)
	lastOK := 0
	for tps := start; tps <= maxR; tps += step {
		fmt.Printf("\n=== rung TPS=%d ===\n", tps)
		res := runRung(ctx, fundClient, workers, lda, entry, tps, dur)
		res.print(tps, totalWorkers, dur)
		if res.broken() {
			fmt.Printf("\nrung TPS=%d BROKE — stopping ramp (last clean: %d)\n", tps, lastOK)
			os.Exit(0)
		}
		lastOK = tps
		// brief recovery
		time.Sleep(3 * time.Second)
	}
	fmt.Printf("\nramp completed cleanly through TPS=%d (max)\n", lastOK)
}

func querySynth(ctx context.Context, c *jsonrpc.Client) synthSnapshot {
	var snap synthSnapshot
	u, err := url.Parse("acc://bvn-bvn1.acme/synthetic")
	if err != nil {
		return snap
	}
	Q := api.Querier2{Querier: c}
	var acct *protocol.SyntheticLedger
	_, err = Q.QueryAccountAs(ctx, u, nil, &acct)
	if err != nil || acct == nil {
		return snap
	}
	for _, s := range acct.Sequence {
		snap.produced += s.Produced
		snap.received += s.Received
		snap.delivered += s.Delivered
	}
	return snap
}

func atoiF(s, name string) int {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if err != nil {
		fatalf("invalid %s: %q", name, s)
	}
	return n
}

type workerCtx struct {
	id         int
	endpoint   string
	client     *jsonrpc.Client
	sk         ed25519.PrivateKey
	lta        *url.URL
	lid        *url.URL
	nonce      uint64
	fundedOK   bool
	creditedOK bool
}

func deriveFaucetKey(seedStrs ...string) ed25519.PrivateKey {
	// Mirror cmd_init_network.go createFaucet: storage.Key.Append(s) for each
	// seed string, then ed25519.NewKeyFromSeed.
	var seed [32]byte
	for _, s := range seedStrs {
		buf := append(append([]byte{}, seed[:]...), []byte(s)...)
		seed = sha256.Sum256(buf)
	}
	return ed25519.NewKeyFromSeed(seed[:])
}

func pollUntilFunded(ctx context.Context, Q api.Querier2, workers []*workerCtx) {
	deadline := time.Now().Add(3 * time.Minute)
	lastReport := time.Now()
	for {
		remaining := 0
		for _, w := range workers {
			if w.fundedOK {
				continue
			}
			var acct *protocol.LiteTokenAccount
			_, err := Q.QueryAccountAs(ctx, w.lta, nil, &acct)
			if err == nil && acct.TokenBalance().Sign() > 0 {
				w.fundedOK = true
				continue
			}
			remaining++
		}
		if remaining == 0 {
			return
		}
		if time.Since(lastReport) > 5*time.Second {
			fmt.Printf("  funded %d/%d (waiting on %d)\n", len(workers)-remaining, len(workers), remaining)
			lastReport = time.Now()
		}
		if time.Now().After(deadline) {
			// Print which ones are stuck
			for _, w := range workers {
				if !w.fundedOK {
					fmt.Printf("  STUCK: worker %d %v\n", w.id, w.lta)
				}
			}
			fatalf("funding poll timeout: %d workers still unfunded", remaining)
		}
		time.Sleep(2 * time.Second)
	}
}

func pollUntilCredited(ctx context.Context, Q api.Querier2, workers []*workerCtx) {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		remaining := 0
		for _, w := range workers {
			if w.creditedOK {
				continue
			}
			var lid *protocol.LiteIdentity
			_, err := Q.QueryAccountAs(ctx, w.lid, nil, &lid)
			if err == nil && lid.CreditBalance > 0 {
				w.creditedOK = true
				continue
			}
			remaining++
		}
		if remaining == 0 {
			return
		}
		if time.Now().After(deadline) {
			fatalf("credit poll timeout: %d workers still uncredited", remaining)
		}
		time.Sleep(2 * time.Second)
	}
}

// Resolve circular import for AS1: parse private key form when needed.
var _ = address.FormatAS1
