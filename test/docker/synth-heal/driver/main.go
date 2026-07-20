// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command driver drives a real (dockerized) network across partition boundaries
// and verifies that every synthetic message it produces is eventually delivered
// — i.e. that healing recovers whatever the network drops (#4064, #4067).
//
// # Workloads
//
//	-workload=mixed      (default) a weighted mix of every user transaction type
//	                     that produces a cross-partition synthetic message
//	-workload=transfers  lite -> lite ACME sends only (the original #4064 repro)
//
// The mixed workload bootstraps its accounts (ADIs, key pages, token accounts,
// lite data accounts, one per foreign partition) and then loops the mix. Every
// transaction it emits targets a foreign partition, so every one of them
// produces at least one cross-partition synthetic:
//
//	lite -> lite transfer        SyntheticDepositTokens
//	ADI token account transfer   SyntheticDepositTokens
//	remote ADI creation          SyntheticCreateIdentity
//	credit purchase              SyntheticDepositCredits
//	token burn                   SyntheticBurnTokens
//	write data to remote LDA     SyntheticWriteData
//	cross-partition authority    SignatureRequest, CreditPayment
//	                             (the MessageForTransaction heal path)
//
// The verdict walks the produced-message tree of every tracked transaction and
// reports sent/delivered per synthetic type, so a missing type is a failure
// even if everything that was produced got delivered.
//
//	go run ./test/docker/synth-heal/driver -endpoint http://localhost:26660 \
//	    -workload mixed -count 24
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// env is everything the workload needs to build, sign and submit transactions.
type env struct {
	c     *jsonrpc.Client
	Q     api.Querier2
	tree  *routing.RouteTree
	ns    *api.NetworkStatus
	track *tracker

	oracle float64 // ACME oracle price, for AddCredits

	srcPart   string // the faucet's partition
	faucetKey ed25519.PrivateKey
	faucet    *url.URL // faucet lite token account
	faucetLID *url.URL // faucet lite identity (the signer)

	// nudgeTo maps a partition to an account there worth sending junk traffic
	// to when a stream looks wedged. See env.waitFor.
	nudgeTo map[string]*url.URL

	nonce uint64 // monotonic signature timestamp, shared by every signer
}

// next returns a pointer to a freshly incremented nonce. The builder takes a
// pointer and increments it itself, but sharing one counter across signers
// keeps timestamps strictly increasing regardless of which key signs.
func (e *env) next() *uint64 {
	e.nonce++
	return &e.nonce
}

func main() {
	endpoint := flag.String("endpoint", "http://localhost:26660", "node JSON-RPC endpoint")
	workload := flag.String("workload", "mixed", "workload: mixed (every transaction type) or transfers (lite->lite only)")
	destID := flag.String("dest", "", "restrict cross-partition traffic to this partition (default: every partition other than the faucet's)")
	faucetSeed := flag.String("faucet-seed", "FAUCET", "genesis faucet seed (matches init --faucet-seed)")
	count := flag.Int("count", 5, "number of workload iterations (ignored in soak mode)")
	timeout := flag.Duration("timeout", 4*time.Minute, "overall timeout")
	tps := flag.Float64("tps", 0, "soak mode: iterations per second (runs for -duration)")
	duration := flag.Duration("duration", 24*time.Hour, "soak mode duration")
	grace := flag.Duration("grace", 10*time.Minute, "how long to wait for everything to be delivered after sending stops")
	trackMax := flag.Int("track-max", 2000, "maximum transactions to follow for the coverage verdict")
	flag.Parse()

	if *tps > 0 && *timeout < *duration+*grace {
		// Don't let the overall timeout cut a soak short.
		*timeout = *duration + *grace + 5*time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	c := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(*endpoint, "v3"))
	c.Client.Timeout = 30 * time.Second
	Q := api.Querier2{Querier: c}

	// Learn the routing table so we can place accounts on specific partitions.
	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	fatalIf(err, "network status")
	tree, err := routing.NewRouteTree(ns.Routing)
	fatalIf(err, "route tree")

	// The genesis faucet account is pre-funded with ACME and credits; it is the
	// root of every account this driver creates.
	faucetKey, faucet := faucetAccount(*faucetSeed)
	srcPart, err := tree.Route(faucet)
	fatalIf(err, "route faucet")

	e := &env{
		c: c, Q: Q, tree: tree, ns: ns,
		oracle:    float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision,
		srcPart:   srcPart,
		faucetKey: faucetKey,
		faucet:    faucet,
		faucetLID: faucet.RootIdentity(),
		nudgeTo:   map[string]*url.URL{},
		nonce:     uint64(time.Now().UTC().UnixMilli()),
	}
	e.track = newTracker(Q, tree)

	parts := e.foreignPartitions(*destID)
	if len(parts) == 0 {
		log.Fatalf("no partition distinct from the faucet's (%s) to target", srcPart)
	}
	log.Printf("faucet %v -> %s; targeting %s", faucet, srcPart, strings.Join(parts, ", "))

	kinds := mixedKinds
	if strings.EqualFold(*workload, "transfers") {
		kinds = transferKinds
	} else if !strings.EqualFold(*workload, "mixed") {
		log.Fatalf("unknown -workload %q (want mixed or transfers)", *workload)
	}

	log.Printf("== bootstrapping accounts ==")
	w := bootstrap(ctx, e, parts, kinds)

	// Decide how many iterations to run and how often to track one for the
	// coverage verdict. Tracking every transaction in a long soak would make
	// the final walk take longer than the soak itself.
	total := *count
	interval := time.Duration(0)
	if *tps > 0 {
		interval = time.Duration(float64(time.Second) / *tps)
		total = int(duration.Seconds() * *tps)
	}
	trackEvery := 1
	if total > *trackMax && *trackMax > 0 {
		trackEvery = total / *trackMax
	}

	log.Printf("== running %s workload ==", *workload)
	run(ctx, e, w, schedule(kinds), total, interval, *duration, trackEvery)

	log.Printf("== waiting for delivery (healing must clear any wedge) ==")
	delivered := e.track.verify(ctx, *grace)
	covered := report(e.track, kinds)
	switch {
	case !delivered:
		fmt.Println("FAIL: some messages were never delivered — the stream did not heal")
		os.Exit(1)
	case !covered:
		fmt.Println("FAIL: the workload never produced every expected synthetic type")
		os.Exit(1)
	}
	fmt.Println("PASS: every tracked transaction was delivered and every expected synthetic type was produced")
}

// foreignPartitions lists the block validator partitions the workload should
// target: every one other than the faucet's, or just -dest if it was given.
func (e *env) foreignPartitions(dest string) []string {
	var parts []string
	for _, p := range e.ns.Network.Partitions {
		if p.Type != protocol.PartitionTypeBlockValidator || strings.EqualFold(p.ID, e.srcPart) {
			continue
		}
		if dest != "" && !strings.EqualFold(p.ID, dest) {
			continue
		}
		parts = append(parts, p.ID)
	}
	sort.Strings(parts)
	return parts
}

// run executes the workload: `total` iterations, one every `interval` (or as
// fast as the node accepts them if interval is zero), stopping after `limit`
// if it is non-zero. Every trackEvery'th iteration is followed for the verdict.
func run(ctx context.Context, e *env, w *world, sched []kind, total int, interval, limit time.Duration, trackEvery int) {
	deadline := time.Time{}
	if limit > 0 && interval > 0 {
		deadline = time.Now().Add(limit)
	}

	lastLog := time.Now()
	var sent, errs int
	for i := 0; i < total; i++ {
		if !deadline.IsZero() && time.Now().After(deadline) {
			break
		}
		if ctx.Err() != nil {
			break
		}

		// Top up the home key page occasionally; a soak outlasts any fixed
		// initial credit purchase.
		if i > 0 && i%200 == 0 {
			w.topUp(ctx, e)
		}

		k := sched[i%len(sched)]
		ids, err := k.run(ctx, e, w, i)
		if err != nil {
			// A node briefly down or paused is expected under chaos; a build
			// error is not, but neither is worth aborting a soak over.
			errs++
			if errs <= 10 || errs%100 == 0 {
				log.Printf("%s failed: %v", k.name, err)
			}
		} else {
			sent++
			if trackEvery <= 1 || i%trackEvery == 0 {
				e.track.follow(k.name, ids)
			}
		}

		if time.Since(lastLog) > time.Minute {
			log.Printf("progress: %d/%d submitted, %d errors", sent, total, errs)
			lastLog = time.Now()
		}
		if interval > 0 {
			time.Sleep(interval)
		}
	}
	log.Printf("done sending: %d submitted, %d errors, %d followed for the verdict", sent, errs, e.track.followed())
}

// report prints per-workload and per-synthetic-type coverage, and reports
// whether every synthetic type the workload is supposed to produce was seen. A
// type that is never produced is a failure in its own right: without that
// check, a workload that quietly stopped exercising a heal path would still
// report everything it did produce as delivered.
func report(t *tracker, kinds []kind) bool {
	fmt.Println()
	fmt.Println("== workload (user transactions followed) ==")
	for _, k := range kinds {
		c := t.kindCount(k.name)
		fmt.Printf("  %-24s submitted=%-6d delivered=%d\n", k.name, c.sent, c.delivered)
	}

	fmt.Println()
	// Not only synthetic transactions: a cross-partition transaction also
	// forwards its signature and, for a remote principal, the transaction
	// itself. Those cross a boundary and have to be healed like anything else,
	// so they are counted and reported here too.
	fmt.Println("== cross-partition message coverage ==")
	want := expectedSynthetics(kinds)
	for _, name := range t.synthNames() {
		c := t.synthCount(name)
		mark := " "
		if c.delivered < c.sent {
			mark = "!"
		}
		fmt.Printf(" %s %-32s produced=%-6d delivered=%d\n", mark, name, c.sent, c.delivered)
		delete(want, name)
	}
	missing := make([]string, 0, len(want))
	for name := range want {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	for _, name := range missing {
		fmt.Printf(" ! %-32s produced=0      delivered=0   (EXPECTED BUT NEVER PRODUCED)\n", name)
	}
	fmt.Println()
	return len(missing) == 0
}

// keyRoutingTo brute-forces an ed25519 key whose ACME lite account routes to the
// given partition.
func keyRoutingTo(tree *routing.RouteTree, partition string) (ed25519.PrivateKey, *url.URL) {
	for i := 0; i < 1_000_000; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		fatalIf(err, "generate key")
		kh := sha256.Sum256(pub)
		u := protocol.LiteAuthorityForHash(kh[:]).JoinPath(protocol.ACME)
		part, err := tree.Route(u)
		fatalIf(err, "route")
		if strings.EqualFold(part, partition) {
			return priv, u
		}
	}
	log.Fatalf("could not find a key routing to %s", partition)
	return nil, nil
}

// adiRoutingTo brute-forces an ADI URL with the given prefix that routes to the
// given partition.
func adiRoutingTo(tree *routing.RouteTree, partition, prefix string) *url.URL {
	for i := 0; i < 1_000_000; i++ {
		u := protocol.AccountUrl(fmt.Sprintf("%s-%x.acme", prefix, i))
		part, err := tree.Route(u)
		fatalIf(err, "route")
		if strings.EqualFold(part, partition) {
			return u
		}
	}
	log.Fatalf("could not find an ADI routing to %s", partition)
	return nil
}

// ldaRoutingTo brute-forces a data entry whose lite data account routes to the
// given partition, returning both. The entry must be the first thing written to
// the account, because the protocol derives the account's URL from it.
func ldaRoutingTo(tree *routing.RouteTree, partition string) (protocol.DataEntry, *url.URL) {
	for i := 0; i < 1_000_000; i++ {
		entry := &protocol.DoubleHashDataEntry{Data: [][]byte{
			[]byte("synth-mixed"),
			[]byte(fmt.Sprintf("%s-%d", partition, i)),
		}}
		u, err := protocol.LiteDataAddress(protocol.ComputeLiteDataAccountId(entry))
		fatalIf(err, "lite data address")
		part, err := tree.Route(u)
		fatalIf(err, "route")
		if strings.EqualFold(part, partition) {
			return entry, u
		}
	}
	log.Fatalf("could not find a lite data account routing to %s", partition)
	return nil, nil
}

// faucetAccount derives the genesis faucet key and ACME account from its seed,
// matching cmd/accumulated's createFaucet.
func faucetAccount(seedStr string) (ed25519.PrivateKey, *url.URL) {
	var seed storage.Key
	for _, s := range strings.Split(seedStr, " ") {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])
	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	fatalIf(err, "faucet lite address")
	return sk, u
}

// submit sends an envelope and returns the IDs of every message it contains
// (the transaction and its signature), failing the process if it is rejected.
func submit(ctx context.Context, c *jsonrpc.Client, env *messaging.Envelope) []*url.TxID {
	ids, err := trySubmit(ctx, c, env)
	fatalIf(err, "submit")
	return ids
}

// trySubmit is submit without the fatal: the workload loop keeps going when a
// node is briefly unavailable.
func trySubmit(ctx context.Context, c *jsonrpc.Client, env *messaging.Envelope) ([]*url.TxID, error) {
	subs, err := c.Submit(ctx, env, api.SubmitOptions{})
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

// waitFor blocks until the message is delivered, failing the process if it
// errors or never arrives. Used for bootstrap steps that later steps depend on.
func waitFor(ctx context.Context, Q api.Querier2, id *url.TxID, limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		r, err := Q.QueryMessage(ctx, id, nil)
		switch {
		case errors.Is(err, errors.NotFound):
		case err != nil:
			fatalIf(err, "query %v", id)
		case r.Status.Delivered():
			if r.Error != nil {
				log.Fatalf("%v failed: %v", id, r.Error)
			}
			return
		}
		time.Sleep(time.Second)
	}
	log.Fatalf("timed out waiting for %v", id)
}

// waitForAll waits for every message in ids to be delivered.
//
// It deliberately does not recurse into produced messages. A produced message
// failing is not in itself a bootstrap failure — votes can be duplicated,
// refunds can fail, an authority signature can lose a race with the credit
// payment that initiates its transaction — and treating any of that as fatal
// makes bootstrap flaky for no benefit. What bootstrap actually depends on is
// that the accounts exist, which waitForAccount checks directly.
func waitForAll(ctx context.Context, Q api.Querier2, ids []*url.TxID, limit time.Duration) {
	for _, id := range ids {
		waitFor(ctx, Q, id, limit)
	}
}

// waitForAccount blocks until the account exists. This is the real
// precondition for each bootstrap step that builds on the previous one.
func (e *env) waitForAccount(ctx context.Context, u *url.URL, limit time.Duration) {
	e.waitFor(ctx, u, limit, fmt.Sprintf("%v to exist", u), func() bool {
		_, err := e.Q.QueryAccount(ctx, u, nil)
		switch {
		case err == nil:
			return true
		case errors.Is(err, errors.NotFound):
			return false
		default:
			fatalIf(err, "query %v", u)
			return false
		}
	})
}

// waitForCredits blocks until the signer has a credit balance, so that the
// transactions it is about to sign can actually pay their fee.
func (e *env) waitForCredits(ctx context.Context, u *url.URL, limit time.Duration) {
	e.waitFor(ctx, u, limit, fmt.Sprintf("%v to have credits", u), func() bool {
		var page *protocol.KeyPage
		_, err := e.Q.QueryAccountAs(ctx, u, nil, &page)
		return err == nil && page.CreditBalance > 0
	})
}

// waitFor polls until done, nudging the destination partition periodically.
//
// The nudge is what makes bootstrap survive a dropped message. Receiver-pull
// healing only notices a gap when a LATER sequence number arrives, so a stream
// wedged at its head with nothing behind it never heals — and bootstrap, which
// submits one transaction and blocks on it, is exactly that shape. Sending
// unrelated traffic toward the same partition supplies the later sequence
// number that exposes the hole.
func (e *env) waitFor(ctx context.Context, scope *url.URL, limit time.Duration, what string, done func() bool) {
	deadline := time.Now().Add(limit)
	part, _ := e.tree.Route(scope)
	for i := 0; time.Now().Before(deadline); i++ {
		if done() {
			return
		}
		// Every ~20s, and not on the first pass: give healing something to
		// detect the gap with.
		if i > 0 && i%10 == 0 {
			log.Printf("still waiting for %s; nudging %s to expose any gap", what, part)
			e.nudge(ctx, part)
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("timed out waiting for %s", what)
}

// nudge sends a cheap transfer toward the given partition (or every known
// partition, if that one has no target yet) purely to advance its synthetic
// sequence number. Failures are ignored — this is a hint, not a step.
func (e *env) nudge(ctx context.Context, part string) {
	targets := e.nudgeTo[part]
	if targets == nil {
		for _, u := range e.nudgeTo {
			e.nudgeOne(ctx, u)
		}
		return
	}
	e.nudgeOne(ctx, targets)
}

func (e *env) nudgeOne(ctx context.Context, to *url.URL) {
	env, err := build.Transaction().For(e.faucet).
		SendTokens(1, protocol.AcmePrecisionPower).To(to).
		SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
	if err != nil {
		return
	}
	_, _ = trySubmit(ctx, e.c, env)
}

func fatalIf(err error, format string, args ...any) {
	if err != nil {
		log.Fatalf("%s: %v", fmt.Sprintf(format, args...), err)
	}
}
