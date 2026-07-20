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
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
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
	c   *jsonrpc.Client
	Q   api.Querier2
	cfg config

	treasury *liteAccount // funds everything else
	oracle   float64

	u     *universe
	track *tracker

	growSlots chan struct{}
	nonce     nonce

	// treasuryCredits is the treasury's credit balance in credit-units, kept
	// fresh by the funding keeper. Actions consult it instead of querying, so
	// the check costs nothing per transaction.
	treasuryCredits atomic.Uint64
}

// canPay reports whether the treasury can cover a fee, in credit-units. A
// signer that cannot pay does not fail cleanly — the signature is rejected and
// the transaction is stranded pending forever — so it is better to skip.
func (e *env) canPay(units uint64) bool {
	return e.treasuryCredits.Load() >= units
}

// submitAsTreasury submits a transaction signed by the treasury and debits the
// cached balance.
//
// Polling alone is not enough: the keeper refreshes every few seconds, and at
// any real rate hundreds of transactions are signed in between. A cache that
// only moves on refresh goes stale, lets unpayable transactions through, and
// each one is stranded pending forever. Debiting an over-estimate as we spend
// keeps the cache conservative between refreshes — the worst case is skipping
// slightly early and refilling slightly sooner.
func (e *env) submitAsTreasury(ctx context.Context, b interface {
	Done() (*messaging.Envelope, error)
}) ([]*url.TxID, error) {
	ids, err := e.submit(ctx, b)
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
	endpoint := flag.String("endpoint", "http://localhost:26660", "node JSON-RPC endpoint")
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
	growth := flag.Float64("growth", 0.02, "initial fraction of transactions that create new account structure")
	growthScale := flag.Int("growth-scale", 50, "growth rate decays as the ADI count grows past this")
	seed := flag.Int64("seed", 0, "RNG seed for a reproducible mix (0 = random)")
	trackMax := flag.Int("track-max", 2000, "maximum transactions to follow for the delivery report")
	maxStranded := flag.Int("max-stranded", 0, "exit non-zero if more than this many followed transactions never landed")
	reportParts := flag.Bool("report-partitions", false, "also report how accounts and traffic landed across partitions")
	reportHeals := flag.Bool("report-heals", false, "also report each validator's heal counters")
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

	c := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(*endpoint, "v3"))
	c.Client.Timeout = 30 * time.Second

	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	fatalIf(err, "network status")

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	log.Printf("network %s, executor %v, seed %d", ns.Network.NetworkName, ns.ExecutorVersion, *seed)

	e := &env{
		c: c, Q: api.Querier2{Querier: c},
		cfg:       config{growth: *growth, growthScale: *growthScale, maxGrowJobs: 4},
		oracle:    float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision,
		u:         newUniverse(rand.New(rand.NewSource(*seed))),
		growSlots: make(chan struct{}, 4),
	}
	e.nonce.v.Store(uint64(time.Now().UTC().UnixMilli()))
	e.track = newTracker(e.Q)

	log.Printf("== funding the treasury ==")
	e.treasury, err = e.openTreasury(ctx, *faucetSeed)
	fatalIf(err, "fund treasury")
	// Prime the cached balance; otherwise every treasury-signed action would
	// skip until the keeper's first tick.
	e.treasuryCredits.Store(e.creditBalance(ctx, e.treasury.id))
	log.Printf("treasury %v (%d credits)", e.treasury.acct, e.treasuryCredits.Load()/protocol.CreditPrecision)

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

	// Keep the treasury solvent for the length of the run.
	go e.keepTreasuryFunded(ctx, *faucetSeed == "")

	log.Printf("== generating %d transactions ==", total)
	limit := time.Duration(0)
	if timed {
		limit = *duration
	}
	generate(ctx, e, total, *tps, limit, *trackMax)

	// Account creation runs in the background; let anything in flight finish so
	// its transactions are counted rather than abandoned.
	e.drainGrowth(ctx, 2*time.Minute)

	log.Printf("== waiting for delivery ==")
	stranded := e.track.settle(ctx, *grace)

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
			if trackEvery <= 1 || i%trackEvery == 0 {
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
	adis, books, pages, accts := e.u.counts()
	log.Printf("sent=%d/%d rejected=%d skipped=%d rate=%.1f/s | adis=%d books=%d pages=%d accounts=%d",
		sent, total, failed, skipped, rate, adis, books, pages, accts)
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
	ids, err := e.submit(ctx, e.build(l).
		AddCredits().WithOracle(e.oracle).Purchase(60000).To(l.id).
		SignWith(l.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(l.key))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("buy credits for the treasury: %w", err)
	}
	e.awaitAll(ctx, ids, 2*time.Minute)
	if err := e.awaitCredits(ctx, l.id, 2*time.Minute); err != nil {
		return nil, err
	}
	return l, nil
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
// from it concurrently. A plain increment races, two callers get the same
// timestamp, and the second signature for that signer is rejected as a replay
// — which does not fail loudly: the transaction is simply stranded pending
// forever with no signature recorded against it.
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
	subs, err := e.c.Submit(ctx, env, api.SubmitOptions{})
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
	return ids, nil
}
