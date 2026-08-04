// Copyright 2025 The Accumulate Authors
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

	// targetTps is the configured -tps, reported in the stats file.
	targetTps float64
}

// canPay reports whether the treasury can cover a fee, in credit-units. A
// signer that cannot pay does not fail cleanly — the signature is rejected and
// the transaction is stranded pending forever — so it is better to skip.
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
	e.nonce.v.Store(uint64(time.Now().UTC().UnixMilli()))
	e.track = newTracker(e.Q)
	e.targetTps = *tps

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
	generate(ctx, e, total, *tps, limit, *trackMax)

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

// generate runs the main loop: pick an action, submit it, occasionally kick off
// an account-creation sequence in the background.
func generate(ctx context.Context, e *env, total int, tps float64, limit time.Duration, trackMax int) {
	var interval time.Duration
	var deadline time.Time
	if tps > 0 {
		interval = time.Duration(float64(time.Second) / tps)
	}
	// A zero limit means "no wall-clock limit, stop after total transactions".
	if limit > 0 {
		deadline = time.Now().Add(limit)
	}

	trackEvery := 1
	if trackMax > 0 && total > trackMax {
		trackEvery = total / trackMax
	}

	started := time.Now()
	lastLog := time.Now()
	var sent, failed, skipped int
	for i := 0; i < total; i++ {
		if ctx.Err() != nil || (!deadline.IsZero() && time.Now().After(deadline)) {
			break
		}

		// Growing the account set happens off the hot path so that creating an
		// ADI — which is a multi-transaction sequence with ordering
		// constraints — never stalls the generation rate.
		if e.u.shouldGrow(e.cfg.growth, e.cfg.growthScale) {
			e.growAsync(ctx)
		}

		act := e.pick()
		ids, err := act.run(ctx, e)
		switch {
		case errors.Is(err, errors.NotReady):
			// The action needs state that does not exist yet. Expected,
			// especially early on; not a failure.
			skipped++
			e.track.skipped(act.name)
		case err != nil:
			failed++
			e.track.failed(act.name)
			if failed <= 20 || failed%200 == 0 {
				log.Printf("%s: %v", act.name, err)
			}
		default:
			sent++
			e.track.generated(act.name)
			// expectFail actions are meant not to deliver — count them, but do
			// not follow them, or they would count toward -max-stranded.
			if !act.expectFail && (trackEvery <= 1 || i%trackEvery == 0) {
				e.track.follow(act.name, ids)
			}
		}

		if time.Since(lastLog) > time.Minute {
			e.logProgress(sent, failed, skipped, total, started)
			lastLog = time.Now()
		}
		if interval > 0 {
			time.Sleep(interval)
		}
	}

	e.logProgress(sent, failed, skipped, total, started)
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
		TargetTps:   e.targetTps,
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
