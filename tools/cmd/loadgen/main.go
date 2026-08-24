// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command loadgen generates load against any Accumulate network that offers a
// faucet. It exercises the full menu of user transaction types and grows an
// ever-expanding set of accounts — ADIs, key books, key pages with a range of
// key counts and thresholds, token accounts and data accounts — then transacts
// against that expanding surface.
//
// Load generation is deliberately network-shape agnostic: it never routes, and
// never chooses an address to land on a particular partition. Where accounts
// fall is whatever their hash decides. Partition and healing statistics are
// reported alongside the transaction mix when asked for, but they are
// observations of the network, not inputs to what gets generated.
//
//	# 5 minutes at 10/s against a local devnet, using its faucet service
//	loadgen -endpoint http://localhost:26660 -tps 10 -duration 5m
//
//	# fixed count against a network whose genesis faucet seed we know
//	loadgen -endpoint http://localhost:26660 -faucet-seed FAUCET -count 500
//
//	# add partition and healing observations to the report
//	loadgen -endpoint ... -tps 2 -duration 24h -report-partitions -report-heals
//
// Do not point this at mainnet.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type config struct {
	growth      float64 // fraction of iterations that grow the account set
	growthScale int     // growth rate decays as the ADI count approaches this
	maxGrowJobs int     // concurrent account-creation sequences
}

// env is everything an action needs to build, sign and submit.
type env struct {
	c   *jsonrpc.Client // primary (faucet, network/consensus status)
	Q   api.Querier2    // rotates across all endpoints (see poolQuerier)
	cfg config

	// clients is the pool of node endpoints; submissions and queries round-robin
	// across them (subIdx) so a single paused/restarted/OOM'd node does not
	// reject transactions or concentrate all load on itself.
	clients []*jsonrpc.Client
	subIdx  atomic.Uint64

	// endpointURLs are the raw -endpoint/-endpoints values, kept so reportHeals
	// can read fields the typed client does not declare (see report.go).
	endpointURLs []string

	treasury *liteAccount // funds everything else
	oracle   float64

	// Cached answer to "does this network produce major blocks?". LockAccount
	// takes a major block height, and a network that never produces one turns
	// every lock into a permanent brick — see majorBlocksExist.
	majorMu      sync.Mutex
	majorSeen    bool
	majorChecked time.Time

	u     *universe
	track *tracker

	// led mirrors per-account balances/credits/txn-counts for verification;
	// observe-only (see accounting.go). fees is the live schedule it dead-reckons
	// credit costs from.
	led  *ledger
	fees *protocol.FeeSchedule

	growSlots chan struct{}
	nonce     nonce

	// signerLocks serializes each signer's draw-and-submit; see muFor.
	signerLocks sync.Map // signer URL string -> *sync.Mutex

	// treasuryCredits is the treasury's credit balance in credit-units, kept
	// fresh by the funding keeper. Actions consult it instead of querying, so
	// the check costs nothing per transaction.
	treasuryCredits atomic.Uint64

	// statsPhase drives the phase field of the live stats file:
	// 0 generating, 1 settling (waiting for delivery), 2 done.
	statsPhase atomic.Int32

	// rateBits is the LIVE generation rate (float64 bits), initialized from
	// -tps and adjustable at runtime through the control API. The generate
	// loop re-reads it every iteration, so a change takes effect within one
	// transaction interval.
	rateBits atomic.Uint64

	// mixOverride maps action name -> weight for actions whose weight has
	// been changed at runtime through the control API. Nil entries never
	// exist; an absent name means "use the compiled-in weight"; a weight of 0
	// disables the action. Replaced wholesale (copy-on-write) so pick() can
	// read it without locks.
	mixOverride atomic.Pointer[map[string]int]
}

// currentTPS returns the live generation rate.
func (e *env) currentTPS() float64 { return math.Float64frombits(e.rateBits.Load()) }

// setTPS changes the live generation rate.
func (e *env) setTPS(tps float64) error {
	if tps <= 0 || math.IsNaN(tps) || math.IsInf(tps, 0) {
		return fmt.Errorf("tps must be a positive number, got %v", tps)
	}
	if tps > 10000 {
		return fmt.Errorf("tps %v is above the sanity cap of 10000", tps)
	}
	e.rateBits.Store(math.Float64bits(tps))
	return nil
}

// weightOf returns an action's effective weight: the runtime override if one
// is set, the compiled-in weight otherwise.
func (e *env) weightOf(a action) int {
	if m := e.mixOverride.Load(); m != nil {
		if w, ok := (*m)[a.name]; ok {
			return w
		}
	}
	return a.weight
}

// setMix merges weight overrides into the live mix. Unknown action names are
// rejected wholesale (nothing is applied) so a typo cannot silently leave the
// intended action untouched. A weight of 0 disables an action.
func (e *env) setMix(weights map[string]int) error {
	if err := validateMix(weights); err != nil {
		return err
	}
	// Copy-on-write merge.
	next := map[string]int{}
	if m := e.mixOverride.Load(); m != nil {
		for k, v := range *m {
			next[k] = v
		}
	}
	for k, v := range weights {
		next[k] = v
	}
	e.mixOverride.Store(&next)
	return nil
}

// clearMix drops every runtime weight override.
func (e *env) clearMix() { e.mixOverride.Store(nil) }

func actionNames() []string {
	names := make([]string, len(menu))
	for i, a := range menu {
		names[i] = a.name
	}
	return names
}

// canPay reports whether the treasury can cover a fee, in credit-units. A
// signer that cannot pay does not fail cleanly — the signature is rejected and
// the transaction is stranded pending forever — so it is better to skip.
// majorBlocksExist reports whether this network has ever produced a major
// block, cached and re-checked periodically.
//
// LockAccount takes a MAJOR block height, not an ordinary one. DI produces no
// major blocks at all — in run 20260822T050137Z the DN reached block 6,607
// with no majorBlockIndex on its ledger and not one log line about producing
// one — so `LockAccount(1)` locks an account until a moment that never comes.
// The generator bricked a dozen lite accounts that way, 884 failures of
// "account is locked until major block 1 (currently at 0)", progressively
// poisoning its own pool and breaking the funding chains that depend on it.
//
// So: only lock when a major block has actually been seen. This is not a
// workaround for #4129 — that issue asks the real question, whether DI ought
// to be producing major blocks — it just stops the load generator destroying
// accounts while the answer is unknown. If major blocks appear, locking
// resumes on its own.
func (e *env) majorBlocksExist(ctx context.Context) bool {
	e.majorMu.Lock()
	defer e.majorMu.Unlock()
	if e.majorSeen {
		return true // once true, always true
	}
	if time.Since(e.majorChecked) < time.Minute {
		return false
	}
	e.majorChecked = time.Now()

	u := protocol.DnUrl().JoinPath(protocol.Ledger)
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return false
	}
	if l, ok := r.Account.(interface{ GetMajorBlockIndex() uint64 }); ok && l.GetMajorBlockIndex() > 0 {
		e.majorSeen = true
	}
	return e.majorSeen
}

func (e *env) canPay(units uint64) bool {
	return e.treasuryCredits.Load() >= units
}

// txBuilder is the tail of a build chain: it produces the signed envelope.
type txBuilder interface {
	Done() (*messaging.Envelope, error)
}

// muFor returns the submission lock for a signer.
//
// Every transaction a signer makes must reach the mempool in the same order its
// timestamp was drawn. The executor rejects any signature whose timestamp is
// not strictly greater than that signer's last (BadTimestamp,
// internal/core/execute/v2/block/sig_user.go), and a rejected signature does
// not fail loudly — the transaction is stored with no signature and left
// pending forever. The global nonce makes timestamps unique and monotonic in
// draw order, but that is not enough: when several goroutines sign as the same
// signer — above all the treasury, which the main loop and every background
// goroutine draw on — whoever draws a timestamp first may submit it second, and
// the lower timestamp is then executed after the higher one and rejected.
//
// Holding this lock across [draw nonce -> submit] makes submit order match draw
// order for each signer, so the timestamps a signer presents are monotonic in
// execution order too. Different signers take different locks, so unrelated
// signers still submit concurrently.
func (e *env) muFor(signer *url.URL) *sync.Mutex {
	m, _ := e.signerLocks.LoadOrStore(signer.String(), &sync.Mutex{})
	return m.(*sync.Mutex)
}

// sign serializes a signer's draw-and-submit. build must draw its own timestamp
// (e.nonce.next()), so the draw happens under the lock alongside the submit.
func (e *env) sign(ctx context.Context, signer *url.URL, build func() txBuilder) ([]*url.TxID, error) {
	mu := e.muFor(signer)
	mu.Lock()
	defer mu.Unlock()
	return e.submit(ctx, build())
}

// submitAsTreasury submits a treasury-signed transaction — serialized on the
// treasury's signer lock, see muFor — and debits the cached balance.
//
// Polling alone is not enough: the keeper refreshes every few seconds, and at
// any real rate hundreds of transactions are signed in between. A cache that
// only moves on refresh goes stale, lets unpayable transactions through, and
// each one is stranded pending forever. Debiting an over-estimate as we spend
// keeps the cache conservative between refreshes — the worst case is skipping
// slightly early and refilling slightly sooner.
func (e *env) submitAsTreasury(ctx context.Context, build func() txBuilder) ([]*url.TxID, error) {
	ids, err := e.sign(ctx, e.treasury.id, build)
	if err == nil {
		for {
			have := e.treasuryCredits.Load()
			want := uint64(0)
			if have > estimatedFeeUnits {
				want = have - estimatedFeeUnits
			}
			if e.treasuryCredits.CompareAndSwap(have, want) {
				break
			}
		}
	}
	return ids, err
}

func main() {
	endpoint := flag.String("endpoint", "http://localhost:26660", "single node JSON-RPC endpoint (fallback if -endpoints unset)")
	endpoints := flag.String("endpoints", "", "comma-separated node endpoints to rotate submissions/queries across (skips dead ones)")
	faucetSeed := flag.String("faucet-seed", "", "genesis faucet seed; if unset, funds come from the network's faucet service")
	count := flag.Int("count", 100, "number of transactions to generate (ignored when -tps is set)")
	// Pacing matters even for a fixed count. Submitting as fast as the client
	// can loop is a burst, not load: it finishes before a single block is
	// produced, so nothing the generator creates ever becomes usable and the
	// mix collapses to whatever needs no prior state.
	tps := flag.Float64("tps", 5, "generate at this rate; with -duration, run for that long instead of -count")
	duration := flag.Duration("duration", time.Hour, "how long to generate for when -tps is set")
	timeout := flag.Duration("timeout", 0, "overall timeout (default: duration + grace + slack)")
	grace := flag.Duration("grace", 5*time.Minute, "how long to wait for delivery after generation stops")
	bootstrap := flag.Int("bootstrap", 100, "sub-treasuries to seed before the workload, spreading funding sources across BVNs (0 disables)")
	growth := flag.Float64("growth", 0.02, "initial fraction of transactions that create new account structure")
	growthScale := flag.Int("growth-scale", 50, "growth rate decays as the ADI count grows past this")
	seed := flag.Int64("seed", 0, "RNG seed for a reproducible mix (0 = random)")
	trackMax := flag.Int("track-max", 2000, "maximum transactions to follow for the delivery report")
	maxStranded := flag.Int("max-stranded", 0, "exit non-zero if more than this many followed transactions never landed")
	reportParts := flag.Bool("report-partitions", false, "also report how accounts and traffic landed across partitions")
	reportHeals := flag.Bool("report-heals", false, "also report each validator's heal counters")
	statsFile := flag.String("stats-file", "", "if set, write a live JSON snapshot of the run (per-type mix, totals, account counts) here every few seconds")
	control := flag.String("control", "", "if set, serve the runtime control API on this address (e.g. 127.0.0.1:8091): GET/POST /control adjusts tps and the transaction mix live")
	submitters := flag.Int("submitters", 16, "concurrent submission workers; the pacer hands each tick to a free worker, so the achieved rate is not capped by one submit round-trip at a time")
	maxAccounts := flag.Int("max-accounts", 25000, "cap the lite-account population; past it, sends reuse existing accounts. An unbounded universe outgrows any state cache and turns long runs into state-size death clocks (0 = unlimited)")
	flag.Parse()

	// -duration selects a timed run; otherwise -count transactions are sent at
	// the same rate.
	timed := isSet("duration")
	total := *count
	if timed {
		total = int(duration.Seconds() * *tps)
	}
	if *timeout == 0 {
		*timeout = *grace + 30*time.Minute
		if timed {
			*timeout += *duration
		} else if *tps > 0 {
			*timeout += time.Duration(float64(total)/(*tps)) * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Build the endpoint pool. -endpoints (comma-separated) rotates submissions
	// and queries across every node; -endpoint is the single-node fallback.
	eps := splitEndpoints(*endpoints)
	if len(eps) == 0 {
		eps = []string{*endpoint}
	}
	var clients []*jsonrpc.Client
	for _, ep := range eps {
		cc := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(ep, "v3"))
		cc.Client.Timeout = 30 * time.Second
		clients = append(clients, cc)
	}
	c := clients[0]
	log.Printf("endpoints: %d (%s)", len(clients), strings.Join(eps, ", "))

	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	fatalIf(err, "network status")

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	log.Printf("network %s, executor %v, seed %d", ns.Network.NetworkName, ns.ExecutorVersion, *seed)

	e := &env{
		c:            c,
		cfg:          config{growth: *growth, growthScale: *growthScale, maxGrowJobs: 4},
		oracle:       float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision,
		u:            newUniverse(rand.New(rand.NewSource(*seed))),
		growSlots:    make(chan struct{}, 4),
		fees:         ns.Globals.FeeSchedule,
		clients:      clients,
		endpointURLs: eps,
	}
	e.Q = api.Querier2{Querier: &poolQuerier{clients: clients, idx: &e.subIdx}}
	e.led = newLedger(e.fees)
	e.u.maxLites = *maxAccounts
	e.nonce.v.Store(uint64(time.Now().UTC().UnixMilli()))
	e.track = newTracker(e.Q)
	// Direct store, not setTPS: -tps 0 is legal at launch (count mode, no
	// pacing); the control API's setTPS is stricter.
	e.rateBits.Store(math.Float64bits(*tps))

	// The control API adjusts the rate and mix without a restart (a restart
	// re-bootstraps the whole account universe). Started before the treasury
	// and bootstrap phases so the API answers from launch, not only once
	// generation begins.
	if *control != "" {
		go serveControl(ctx, e, *control)
	}

	log.Printf("== funding the treasury ==")
	e.treasury, err = e.openTreasury(ctx, *faucetSeed)
	fatalIf(err, "fund treasury")
	// Prime the cached balance; otherwise every treasury-signed action would
	// skip until the keeper's first tick.
	e.treasuryCredits.Store(e.creditBalance(ctx, e.treasury.id))
	log.Printf("treasury %v (%d credits)", e.treasury.acct, e.treasuryCredits.Load()/protocol.CreditPrecision)

	// Keep the treasury solvent for the length of the run. Started first so its
	// refills cover the bootstrap burst below.
	go e.keepTreasuryFunded(ctx, *faucetSeed == "")

	// Seed a base of funding sources spread across every BVN BEFORE the workload
	// starts, so random source selection originates cross-partition traffic from
	// all partitions instead of cascading from one. Synchronous by design: the
	// mix should run against the spread base, not build it. Non-fatal on partial
	// failure — a smaller base still beats the single-treasury star.
	if err := e.bootstrapSubTreasuries(ctx, *bootstrap); err != nil {
		log.Printf("bootstrap: %v — continuing with whatever seeded", err)
	}

	// Seed the universe in the background. Creating an identity is a sequence
	// of transactions that each have to land before the next can be built, and
	// blocking on it here would mean the generator emits nothing at all while
	// it waits — which on a lossy network is also the one situation where a
	// dropped message has no later traffic behind it to shake it loose.
	// Lite-account actions need no identity, so load starts immediately and
	// identities join the mix as they become usable.
	e.growAsync(ctx)

	// Confirm and credit lite accounts in the background so that actions which
	// sign as a lite identity always have a valid signer available.
	go e.promoteLites(ctx)

	// Keep the local balance mirror honest: sample and re-sync against the chain
	// every few minutes (dead reckoning drifts on refunds/oracle moves).
	go e.reconcile(ctx, 3*time.Minute, 20)

	// A live stats file lets an external monitor read the per-type mix and
	// totals while the run is in flight, not only from the end-of-run report.
	start := time.Now()
	if *statsFile != "" {
		go e.writeStatsLoop(ctx, *statsFile, start, total)
	}

	log.Printf("== generating %d transactions ==", total)
	limit := time.Duration(0)
	if timed {
		limit = *duration
	}
	generate(ctx, e, total, limit, *trackMax, *submitters)

	// Account creation runs in the background; let anything in flight finish so
	// its transactions are counted rather than abandoned.
	e.drainGrowth(ctx, 2*time.Minute)

	e.statsPhase.Store(1)
	log.Printf("== waiting for delivery ==")
	stranded := e.track.settle(ctx, *grace)
	e.statsPhase.Store(2)
	if *statsFile != "" {
		e.writeStats(*statsFile, start, total) // a final snapshot marked done
	}

	e.reportMix()
	if *reportParts {
		e.reportPartitions(ctx)
	}
	if *reportHeals {
		e.reportHeals(ctx)
	}

	switch {
	case stranded == 0:
		fmt.Println("OK: every followed transaction was delivered")
	case stranded > *maxStranded:
		fmt.Printf("INCOMPLETE: %d followed transactions never landed (see above)\n", stranded)
		os.Exit(1)
	default:
		fmt.Printf("OK: %d transactions never landed, within the -max-stranded tolerance of %d\n", stranded, *maxStranded)
	}
}

// generate runs the main loop. A single PACER owns the rate — one tick per
// 1/tps interval, re-read every tick so the control API takes effect live —
// and hands each tick to a pool of submitter workers. One worker per
// round-trip was the ceiling the 100 tps probe hit at ~84/s: build+sign+
// submit is mostly waiting on the node, so the pacer must never wait on it.
func generate(ctx context.Context, e *env, total int, limit time.Duration, trackMax, submitters int) {
	// A zero limit means "no wall-clock limit, stop after total transactions".
	// With a limit, the wall clock is the ONLY stop: the rate is adjustable at
	// runtime (control API), so a transaction count computed from the launch
	// rate would end a bumped-up run early.
	var deadline time.Time
	if limit > 0 {
		deadline = time.Now().Add(limit)
	}

	trackEvery := 1
	if trackMax > 0 && total > trackMax {
		trackEvery = total / trackMax
	}
	if submitters < 1 {
		submitters = 1
	}

	started := time.Now()
	var mu sync.Mutex // guards the counters and first-failure maps below
	var sent, failed, skipped, lagged int
	seenFail := map[string]bool{}
	seenSkip := map[string]bool{}

	work := make(chan int, submitters)
	var wg sync.WaitGroup
	for w := 0; w < submitters; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				act := e.pick()
				ids, err := act.run(ctx, e)
				mu.Lock()
				switch {
				case errors.Is(err, errors.NotReady):
					skipped++
					e.track.skipped(act.name)
					if !seenSkip[act.name] {
						seenSkip[act.name] = true
						log.Printf("%s: skipped (first time): %v", act.name, err)
					}
				case err != nil:
					failed++
					e.track.failed(act.name)
					first := !seenFail[act.name]
					if first {
						seenFail[act.name] = true
					}
					if first || failed <= 20 || failed%200 == 0 {
						log.Printf("%s: %v", act.name, err)
					}
				default:
					sent++
					e.track.generated(act.name)
					if !act.expectFail && (trackEvery <= 1 || i%trackEvery == 0) {
						e.track.follow(act.name, ids)
					}
				}
				mu.Unlock()
			}
		}()
	}

	lastLog := time.Now()
	for i := 0; deadline.IsZero() && i < total || !deadline.IsZero(); {
		if ctx.Err() != nil || (!deadline.IsZero() && time.Now().After(deadline)) {
			break
		}

		// Sleep granularity caps a one-tick-per-sleep pacer near 200/s: the
		// scheduler overshoots multi-millisecond sleeps by ~1-2ms, measured
		// as a hard ~206/s ceiling whatever the target. Emit a BATCH of
		// ticks per sleep so each sleep covers batch/tps and stays >=5ms.
		tps := e.currentTPS()
		batch := 1
		if tps > 200 {
			batch = int(tps/200) + 1
		}
		for j := 0; j < batch; j++ {
			if deadline.IsZero() && i >= total {
				break
			}

			// Growing the account set happens off the hot path so that
			// creating an ADI — a multi-transaction sequence with ordering
			// constraints — never stalls the generation rate.
			if e.u.shouldGrow(e.cfg.growth, e.cfg.growthScale) {
				e.growAsync(ctx)
			}

			// Hand the tick to a worker. If every worker is busy AND the
			// buffer is full, the generator is submit-bound: count it rather
			// than silently running slow, then block — honest backpressure
			// beats a lie about the achieved rate.
			select {
			case work <- i:
			default:
				mu.Lock()
				lagged++
				mu.Unlock()
				select {
				case work <- i:
				case <-ctx.Done():
				}
			}
			i++
		}

		mu.Lock()
		if time.Since(lastLog) > time.Minute {
			e.logProgress(sent, failed, skipped, total, started)
			if lagged > 0 {
				log.Printf("pacer: %d ticks waited for a free submitter (submit-bound; raise -submitters)", lagged)
			}
			lastLog = time.Now()
		}
		mu.Unlock()

		if tps > 0 {
			time.Sleep(time.Duration(float64(batch) * float64(time.Second) / tps))
		}
	}
	close(work)
	wg.Wait()
}

func (e *env) logProgress(sent, failed, skipped, total int, started time.Time) {
	elapsed := time.Since(started).Seconds()
	rate := 0.0
	if elapsed > 0 {
		rate = float64(sent) / elapsed
	}
	adis, books, pages, accts, issuers := e.u.counts()
	log.Printf("sent=%d/%d rejected=%d skipped=%d rate=%.1f/s | adis=%d books=%d pages=%d tokens=%d accounts=%d",
		sent, total, failed, skipped, rate, adis, books, pages, issuers, accts)
}

// openTreasury establishes the account everything else is funded from: either
// the genesis faucet (when its seed is known) or a fresh lite account topped up
// from the network's faucet service.
func (e *env) openTreasury(ctx context.Context, seed string) (*liteAccount, error) {
	if seed != "" {
		key, acct := faucetAccount(seed)
		return &liteAccount{key: key, acct: acct, id: acct.RootIdentity()}, nil
	}

	l := newLiteAccount(e.u.rng)
	// The faucet hands out a fixed amount per call; ask a few times so there is
	// enough ACME to buy credits and still fund accounts for a long run.
	for i := 0; i < 5; i++ {
		sub, err := e.c.Faucet(ctx, l.acct, api.FaucetOptions{Token: protocol.AcmeUrl()})
		if err != nil {
			if i == 0 {
				return nil, errors.UnknownError.WithFormat("faucet: %w (does this network offer one? otherwise pass -faucet-seed)", err)
			}
			break
		}
		if sub.Status != nil && sub.Status.TxID != nil {
			e.awaitDelivery(ctx, sub.Status.TxID, 2*time.Minute)
		}
	}

	// The faucet transaction being delivered is not the same as the account
	// existing: the account is created by the synthetic deposit the faucet
	// produces, which lands a block or more later. Wait for the effect.
	if err := e.awaitAccount(ctx, l.acct, 2*time.Minute); err != nil {
		return nil, errors.UnknownError.WithFormat("faucet never funded %v: %w", l.acct, err)
	}

	// A lite identity needs credits before it can sign anything.
	ids, err := e.sign(ctx, l.id, func() txBuilder {
		return e.build(l).
			AddCredits().WithOracle(e.oracle).Purchase(60000).To(l.id).
			SignWith(l.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(l.key)
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("buy credits for the treasury: %w", err)
	}
	e.awaitAll(ctx, ids, 2*time.Minute)
	if err := e.awaitCredits(ctx, l.id, 2*time.Minute); err != nil {
		return nil, err
	}
	return l, nil
}

// liveStats is the JSON an external monitor reads while the run is in flight.
type liveStats struct {
	UpdatedUnix int64               `json:"updatedUnix"`
	StartUnix   int64               `json:"startUnix"`
	ElapsedSec  int64               `json:"elapsedSec"`
	Phase       string              `json:"phase"` // generating | settling | done
	Target      int                 `json:"target"`
	Generated   int                 `json:"generated"`
	Rejected    int                 `json:"rejected"`
	Skipped     int                 `json:"skipped"`
	Rate        float64             `json:"rate"`      // cumulative average user tx/s
	TargetTps   float64             `json:"targetTps"` // configured -tps target
	PerType     map[string]typeStat `json:"perType"`
	Accounts    struct {
		Identities   int `json:"identities"`
		KeyBooks     int `json:"keyBooks"`
		KeyPages     int `json:"keyPages"`
		TokenIssuers int `json:"tokenIssuers"`
		Accounts     int `json:"accounts"`
	} `json:"accounts"`
}

// writeStatsLoop refreshes the stats file every few seconds until the context
// ends. A monitor polls the file; the writer never blocks the generator.
func (e *env) writeStatsLoop(ctx context.Context, path string, start time.Time, total int) {
	for {
		e.writeStats(path, start, total)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// writeStats writes one snapshot atomically (temp file + rename) so a reader
// never sees a half-written file.
func (e *env) writeStats(path string, start time.Time, total int) {
	perType, gen, rej, skip := e.track.snapshot()
	phase := []string{"generating", "settling", "done"}[e.statsPhase.Load()]
	elapsed := time.Since(start).Seconds()
	rate := 0.0
	if elapsed > 0 {
		rate = float64(gen) / elapsed
	}
	s := liveStats{
		UpdatedUnix: time.Now().Unix(),
		StartUnix:   start.Unix(),
		ElapsedSec:  int64(elapsed),
		Phase:       phase,
		Target:      total,
		Generated:   gen,
		Rejected:    rej,
		Skipped:     skip,
		Rate:        rate,
		TargetTps:   e.currentTPS(),
		PerType:     perType,
	}
	adis, books, pages, accts, issuers := e.u.counts()
	s.Accounts.Identities = adis
	s.Accounts.KeyBooks = books
	s.Accounts.KeyPages = pages
	s.Accounts.TokenIssuers = issuers
	s.Accounts.Accounts = accts

	b, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("stats: write: %v", err)
		return
	}
	_ = os.Rename(tmp, path)
}

// isSet reports whether a flag was given on the command line.
func isSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func fatalIf(err error, format string, args ...any) {
	if err != nil {
		log.Fatalf("%s: %v", fmt.Sprintf(format, args...), err)
	}
}

// nonce is the monotonic signature timestamp, shared by every signer so that
// timestamps are strictly increasing no matter which key signs.
//
// It MUST be atomic. The generation loop, the lite-account promoter, the
// treasury keeper and every background account-creation goroutine all draw
// from it concurrently; a plain increment races and hands two callers the same
// timestamp, and the second signature for that signer is rejected as a replay.
//
// Atomicity is necessary but not sufficient: unique, monotonic-in-draw-order
// timestamps still strand transactions if a signer submits them out of order,
// because the executor rejects any timestamp that is not strictly greater than
// the signer's last. Ordering the submissions of each signer is muFor's job;
// this only guarantees the values themselves never collide. Neither failure is
// loud — a rejected signature leaves the transaction pending forever with no
// signature recorded against it.
type nonce struct{ v atomic.Uint64 }

func (n *nonce) next() *uint64 {
	v := n.v.Add(1)
	return &v
}

// submit builds and submits, returning the IDs of every message in the
// envelope (the transaction and its signature).
func (e *env) submit(ctx context.Context, b interface {
	Done() (*messaging.Envelope, error)
}) ([]*url.TxID, error) {
	env, err := b.Done()
	if err != nil {
		return nil, err
	}
	// Choose the endpoint by SIGNER, not round-robin: a signer's transactions
	// carry strictly increasing timestamps and the executor rejects any that
	// reach a mempool out of order, so all of one signer's transactions must go
	// to the same node (the muFor lock keeps them ordered there). Different
	// signers hash to different nodes, which is what spreads the load. On a
	// transport error advance to the next node; a business rejection returns
	// as-is. Queries have no such constraint and rotate freely (poolQuerier).
	var subs []*api.Submission
	n := len(e.clients)
	start := int(e.subIdx.Add(1)) % n // fallback for unsigned/edge envelopes
	if len(env.Signatures) > 0 {
		if s := env.Signatures[0].GetSigner(); s != nil {
			start = int(hashString(s.String()) % uint32(n))
		}
	}
	for i := 0; i < n; i++ {
		subs, err = e.clients[(start+i)%n].Submit(ctx, env, api.SubmitOptions{})
		if err == nil || !isNetErr(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	var ids []*url.TxID
	for _, s := range subs {
		if !s.Success {
			if s.Status != nil && s.Status.Error != nil {
				return nil, s.Status.Error
			}
			return nil, errors.UnknownError.With(s.Message)
		}
		if s.Status != nil && s.Status.TxID != nil {
			ids = append(ids, s.Status.TxID)
		}
	}
	e.led.record(env) // observe-only local mirror; panic-safe
	return ids, nil
}
