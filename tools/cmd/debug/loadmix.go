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
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// loadmix is a realistic, paced, multi-account load generator. Unlike
// loadramp (which fires WriteData from a single account, so all load hits one
// BVN), loadmix maintains many distinct "actors", each owning its own lite
// account and ADI. Because accounts route by address, spreading load across
// many accounts naturally spreads it across all BVNs. Each actor walks a
// confirmation-gated state machine (you can't create acc://x.acme/tokens until
// acc://x.acme is committed, and that's async/synthetic), and once fully built
// out it cycles through a weighted-random mix of sends, writes, credit
// top-ups, and multi-sig transactions.

var cmdLoadMix = &cobra.Command{
	Use:   "loadmix [server] [faucet-address]",
	Short: "Realistic multi-account paced load: ADIs, token/data accounts, sends, multi-sig, writes",
	Args:  cobra.ExactArgs(2),
	Run:   loadMix,
}

var loadMixOpts struct {
	tps      int
	duration time.Duration
	actors   int
	interval time.Duration
}

func init() {
	cmdLoadMix.Flags().IntVar(&loadMixOpts.tps, "tps", 20, "Target transactions per second")
	cmdLoadMix.Flags().DurationVar(&loadMixOpts.duration, "duration", 10*time.Minute, "Test duration")
	cmdLoadMix.Flags().IntVar(&loadMixOpts.actors, "actors", 40, "Number of concurrent identities to maintain")
	cmdLoadMix.Flags().DurationVar(&loadMixOpts.interval, "report-interval", 30*time.Second, "How often to print live metrics")
	cmd.AddCommand(cmdLoadMix)
}

// ---------------------------------------------------------------------------
// Partition-aware client
//
// The local docker network is NOT cross-partition meshed: a node only answers
// queries/submits for the partition(s) it hosts (its BVN plus the co-located
// Directory). So we must route every account to the partition that owns it and
// talk to a node that hosts that partition. We discover the routing table from
// network-status and map each partition id to a node endpoint.
// ---------------------------------------------------------------------------

type partClient struct {
	router    routing.Router
	byPart    map[string]*jsonrpc.Client // partition id (upper) -> client
	directory *jsonrpc.Client            // any node (DN is everywhere)
}

func (p *partClient) clientFor(u *url.URL) (*jsonrpc.Client, string) {
	part, err := p.router.RouteAccount(u)
	if err != nil {
		return p.directory, "?"
	}
	if c, ok := p.byPart[strings.ToUpper(part)]; ok {
		return c, part
	}
	// Directory-routed (or unknown) -> any node serves DN.
	return p.directory, part
}

func (p *partClient) querier(u *url.URL) api.Querier2 {
	c, _ := p.clientFor(u)
	return api.Querier2{Querier: c}
}

// ---------------------------------------------------------------------------
// Actor state machine
// ---------------------------------------------------------------------------

type actorStep int

const (
	stepFundLite actorStep = iota
	stepBuyLiteCredits
	stepCreateADI
	stepBuyPageCredits
	stepCreateTokenAcct
	stepFundTokenAcct
	stepCreateDataAcct
	stepAddSecondKey
	stepActive
)

type actor struct {
	id int

	// lite key + derived accounts
	sk  ed25519.PrivateKey
	kh  [32]byte
	lid *url.URL // lite identity
	lta *url.URL // lite token account

	// ADI
	adi    *url.URL
	book   *url.URL
	page   *url.URL // book/1
	tokens *url.URL
	data   *url.URL

	// second key (for multi-sig)
	sk2      ed25519.PrivateKey
	kh2      [32]byte
	multiSig bool // book/1 threshold is 2

	step actorStep

	// per-signer monotonic nonces
	nonceLite uint64
	noncePage uint64

	// page signer version: starts at 1, increments on every committed
	// updateKeyPage. The network rejects signatures with the wrong version.
	pageVer uint64

	// busy guards an actor so at most one op runs against it at a time,
	// preventing nonce/version races between concurrent dispatcher goroutines.
	busy atomic.Bool

	// During build-out, after submitting a step we poll for commitment rather
	// than re-submitting every turn. waitUntil throttles re-submission of a
	// not-yet-committed step (avoids flooding identical fund/create txns).
	waitUntil time.Time
}

func newActor(id int) *actor {
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	pub := sk[32:]
	kh := sha256.Sum256(pub)
	lid := protocol.LiteAuthorityForHash(kh[:])

	_, sk2, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	kh2 := sha256.Sum256(sk2[32:])

	// random ADI name
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	adi, err := url.Parse(fmt.Sprintf("acc://m%s.acme", hex.EncodeToString(rnd[:])))
	check(err)

	now := uint64(time.Now().UTC().UnixNano())
	return &actor{
		id:        id,
		sk:        sk,
		kh:        kh,
		lid:       lid,
		lta:       lid.JoinPath("ACME"),
		adi:       adi,
		book:      adi.JoinPath("book"),
		page:      adi.JoinPath("book", "1"),
		tokens:    adi.JoinPath("tokens"),
		data:      adi.JoinPath("data"),
		sk2:       sk2,
		kh2:       kh2,
		step:      stepFundLite,
		nonceLite: now,
		noncePage: now,
		pageVer:   1,
	}
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

type metrics struct {
	submitted   atomic.Uint64
	succeeded   atomic.Uint64
	mempoolFull atomic.Uint64
	otherErr    atomic.Uint64
	notReady    atomic.Uint64 // gated/retry, not an error

	mu        sync.Mutex
	byType    map[string]uint64
	byPartTx  map[string]uint64 // partition -> count of submitted txns (load spread)
	errSample map[string]uint64 // sampled error text -> count
}

func (m *metrics) sampleErr(txType, msg string) {
	if len(msg) > 120 {
		msg = msg[:120]
	}
	m.mu.Lock()
	m.errSample[txType+": "+msg]++
	m.mu.Unlock()
}

func newMetrics() *metrics {
	return &metrics{byType: map[string]uint64{}, byPartTx: map[string]uint64{}, errSample: map[string]uint64{}}
}

func (m *metrics) countType(t string) {
	m.mu.Lock()
	m.byType[t]++
	m.mu.Unlock()
}

func (m *metrics) countPart(p string) {
	m.mu.Lock()
	m.byPartTx[p]++
	m.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func loadMix(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	server.Client.Timeout = 30 * time.Second

	// Funding key
	faddr, err := address.Parse(args[1])
	checkf(err, "faucet address")
	fsk, ok := faddr.GetPrivateKey()
	if !ok {
		fatalf("faucet address must be a private key (AS1...)")
	}
	fkh, ok := faddr.GetPublicKeyHash()
	if !ok {
		fatalf("faucet address has no public key hash")
	}
	faucetLid := protocol.LiteAuthorityForHash(fkh)
	faucetLta := faucetLid.JoinPath("ACME")

	// Network status: routing table + oracle.
	ns, err := server.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	checkf(err, "network status")
	router := routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})
	oracle := float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision

	// Build the partition->endpoint map. The endpoint list is the local
	// docker layout: bvn1=27680, bvn2=27684, bvn3=27688, and the DN is
	// co-located on every node (we reuse the supplied server for DN).
	pc := buildPartClient(ctx, args[0], router, server)
	fmt.Printf("Oracle price:    %.4f\n", oracle)
	fmt.Printf("Faucet:          %v\n", faucetLta)
	fmt.Printf("Partitions:      ")
	parts := make([]string, 0, len(pc.byPart))
	for p := range pc.byPart {
		parts = append(parts, p)
	}
	sort.Strings(parts)
	fmt.Printf("%s (+Directory)\n", strings.Join(parts, ", "))

	// Verify the faucet account.
	var facct *protocol.LiteTokenAccount
	_, err = pc.querier(faucetLta).QueryAccountAs(ctx, faucetLta, nil, &facct)
	checkf(err, "query faucet token account %v", faucetLta)
	if facct.TokenBalance().Sign() == 0 {
		fatalf("faucet %v has zero ACME", faucetLta)
	}
	fmt.Printf("Faucet ACME:     %v\n", facct.TokenBalance())

	tps := loadMixOpts.tps
	if tps < 1 {
		tps = 1
	}
	maxActors := loadMixOpts.actors
	if maxActors < 1 {
		maxActors = 1
	}

	m := newMetrics()

	// The faucet lite identity signs all funding txns; one global nonce.
	var faucetNonce atomic.Uint64
	faucetNonce.Store(uint64(time.Now().UTC().UnixNano()))

	eng := &engine{
		ctx:         ctx,
		pc:          pc,
		m:           m,
		oracle:      oracle,
		fsk:         fsk,
		faucetLid:   faucetLid,
		faucetLta:   faucetLta,
		faucetNonce: &faucetNonce,
	}

	// Spawn actors over a ramp window, then keep them cycling.
	actors := make([]*actor, 0, maxActors)
	var actorsMu sync.Mutex

	rng := mrand.New(mrand.NewSource(time.Now().UnixNano()))

	tick := time.Second / time.Duration(tps)
	if tick <= 0 {
		tick = time.Millisecond
	}
	rampEvery := loadMixOpts.duration / time.Duration(maxActors+1)
	if rampEvery < 200*time.Millisecond {
		rampEvery = 200 * time.Millisecond
	}

	stop := make(chan struct{})
	done := make(chan struct{})

	// Actor spawner: add a new actor every rampEvery until maxActors.
	go func() {
		t := time.NewTicker(rampEvery)
		defer t.Stop()
		for {
			actorsMu.Lock()
			n := len(actors)
			actorsMu.Unlock()
			if n < maxActors {
				a := newActor(n)
				actorsMu.Lock()
				actors = append(actors, a)
				actorsMu.Unlock()
			}
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
	}()

	// Seed the first actor immediately so we don't idle.
	actors = append(actors, newActor(0))

	fmt.Printf("Running ~%d TPS for %v, ramping to %d actors (tick=%v)\n",
		tps, loadMixOpts.duration, maxActors, tick)

	// Dispatcher: one transaction per tick, advancing a randomly chosen actor.
	go func() {
		defer close(done)
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		// Bounded concurrency for the (blocking) confirmation queries inside
		// advance(): we don't want a slow query to stall the whole rate.
		sem := make(chan struct{}, tps*2+8)
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			actorsMu.Lock()
			n := len(actors)
			if n == 0 {
				actorsMu.Unlock()
				continue
			}
			// Pick an actor that isn't already mid-operation. Try a few.
			var a *actor
			for try := 0; try < 5; try++ {
				cand := actors[rng.Intn(n)]
				if cand.busy.CompareAndSwap(false, true) {
					a = cand
					break
				}
			}
			actorsMu.Unlock()
			if a == nil {
				continue
			}

			select {
			case sem <- struct{}{}:
			default:
				// All workers busy; skip this tick rather than block the rate.
				a.busy.Store(false)
				continue
			}
			go func(picked *actor) {
				defer func() { <-sem }()
				a := picked
				// If the picked build-out actor is in its wait window (nothing to
				// submit this turn), hand off to an ACTIVE actor so the tick still
				// produces a transaction.
				if a.step != stepActive && time.Now().Before(a.waitUntil) {
					if alt := pickActiveExcept(a); alt != nil && alt.busy.CompareAndSwap(false, true) {
						picked.busy.Store(false)
						a = alt
					}
				}
				defer a.busy.Store(false)
				localRng := mrand.New(mrand.NewSource(time.Now().UnixNano() + int64(a.id)))
				eng.advance(a, localRng)
			}(a)
		}
	}()

	// Reporter.
	deadline := time.Now().Add(loadMixOpts.duration)
	reportTicker := time.NewTicker(loadMixOpts.interval)
	defer reportTicker.Stop()
	var lastSub, lastSucc uint64
	lastT := time.Now()
loop:
	for {
		t := <-reportTicker.C
		s := m.submitted.Load()
		k := m.succeeded.Load()
		dt := t.Sub(lastT).Seconds()
		actorsMu.Lock()
		na := len(actors)
		nActive := 0
		for _, a := range actors {
			if a.step == stepActive {
				nActive++
			}
		}
		actorsMu.Unlock()
		fmt.Printf("[%s] actors=%d active=%d  sub=%d(%.1f/s) ok=%d(%.1f/s) mempoolFull=%d otherErr=%d notReady=%d\n",
			time.Now().Format("15:04:05"), na, nActive, s, float64(s-lastSub)/dt, k, float64(k-lastSucc)/dt,
			m.mempoolFull.Load(), m.otherErr.Load(), m.notReady.Load())
		printTypeLine(m)
		lastSub, lastSucc, lastT = s, k, t
		if t.After(deadline) {
			break loop
		}
	}
	close(stop)
	<-done

	// Final summary.
	s := m.submitted.Load()
	k := m.succeeded.Load()
	fmt.Println()
	fmt.Println("=== loadmix summary ===")
	fmt.Printf("Duration:        %v\n", loadMixOpts.duration)
	fmt.Printf("Submitted:       %d\n", s)
	fmt.Printf("Succeeded:       %d\n", k)
	if s > 0 {
		fmt.Printf("Success rate:    %.1f%%\n", 100*float64(k)/float64(s))
	}
	fmt.Printf("Mempool full:    %d\n", m.mempoolFull.Load())
	fmt.Printf("Other errors:    %d\n", m.otherErr.Load())
	fmt.Printf("Not-ready waits: %d\n", m.notReady.Load())
	fmt.Println("--- per-type (submitted) ---")
	printTypeTable(m)
	fmt.Println("--- per-partition (submitted txns, BVN load spread) ---")
	m.mu.Lock()
	pk := make([]string, 0, len(m.byPartTx))
	for p := range m.byPartTx {
		pk = append(pk, p)
	}
	sort.Strings(pk)
	for _, p := range pk {
		fmt.Printf("  %-12s %d\n", p, m.byPartTx[p])
	}
	if len(m.errSample) > 0 {
		fmt.Println("--- error samples (txType: message -> count) ---")
		ek := make([]string, 0, len(m.errSample))
		for k := range m.errSample {
			ek = append(ek, k)
		}
		sort.Slice(ek, func(i, j int) bool { return m.errSample[ek[i]] > m.errSample[ek[j]] })
		for i, k := range ek {
			if i >= 15 {
				break
			}
			fmt.Printf("  %4d  %s\n", m.errSample[k], k)
		}
	}
	m.mu.Unlock()
}

func printTypeLine(m *metrics) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.byType))
	for kk := range m.byType {
		keys = append(keys, kk)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, kk := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", kk, m.byType[kk]))
	}
	fmt.Printf("    types: %s\n", strings.Join(parts, " "))
}

func printTypeTable(m *metrics) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.byType))
	for kk := range m.byType {
		keys = append(keys, kk)
	}
	sort.Strings(keys)
	for _, kk := range keys {
		fmt.Printf("  %-22s %d\n", kk, m.byType[kk])
	}
}

func buildPartClient(ctx context.Context, primary string, router routing.Router, server *jsonrpc.Client) *partClient {
	// Default local docker endpoints, one healthy node per BVN.
	cand := map[string]string{
		"BVN1": "http://127.0.0.1:27680",
		"BVN2": "http://127.0.0.1:27684",
		"BVN3": "http://127.0.0.1:27688",
	}
	pc := &partClient{
		router:    router,
		byPart:    map[string]*jsonrpc.Client{},
		directory: server,
	}
	for part, ep := range cand {
		c := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(ep, "v3"))
		c.Client.Timeout = 30 * time.Second
		// Verify this endpoint actually hosts that partition by asking for its
		// partition-scoped network status.
		_, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: part})
		if err != nil {
			fmt.Printf("WARN: %s (%s) not reachable: %v\n", part, ep, err)
			continue
		}
		pc.byPart[part] = c
	}
	return pc
}

// ---------------------------------------------------------------------------
// Engine: builds, submits, confirms.
// ---------------------------------------------------------------------------

type engine struct {
	ctx         context.Context
	pc          *partClient
	m           *metrics
	oracle      float64
	fsk         []byte
	faucetLid   *url.URL
	faucetLta   *url.URL
	faucetNonce *atomic.Uint64
}

func (e *engine) fnonce() uint64 { return e.faucetNonce.Add(1) }

// submit routes the envelope by its principal and records metrics. It returns
// true on submit-success (accepted into mempool / delivered as appropriate).
func (e *engine) submit(principal *url.URL, txType string, env *messaging.Envelope) bool {
	client, part := e.pc.clientFor(principal)
	e.m.submitted.Add(1)
	e.m.countType(txType)
	e.m.countPart(part)

	subs, err := client.Submit(e.ctx, env, api.SubmitOptions{})
	if err != nil {
		if isMempoolFull(err) {
			e.m.mempoolFull.Add(1)
		} else {
			e.m.otherErr.Add(1)
			e.m.sampleErr(txType, err.Error())
		}
		return false
	}
	anyFail := false
	for _, s := range subs {
		if s.Success {
			continue
		}
		anyFail = true
		if s.Status != nil && s.Status.Error != nil && isMempoolFull(s.Status.Error) {
			e.m.mempoolFull.Add(1)
		} else {
			e.m.otherErr.Add(1)
			msg := s.Message
			if s.Status != nil && s.Status.Error != nil {
				msg = s.Status.Error.Error()
			}
			e.m.sampleErr(txType, msg)
		}
	}
	if anyFail {
		return false
	}
	e.m.succeeded.Add(1)
	return true
}

// committed reports whether an account exists and is queryable (used to gate
// the next step). Routes the query to the account's partition.
func (e *engine) accountExists(u *url.URL) bool {
	q := e.pc.querier(u)
	_, err := q.QueryAccount(e.ctx, u, nil)
	return err == nil
}

// liteCredits returns the credit balance of a lite identity, or -1 if unknown.
func (e *engine) liteCredits(lid *url.URL) int64 {
	q := e.pc.querier(lid)
	var li *protocol.LiteIdentity
	_, err := q.QueryAccountAs(e.ctx, lid, nil, &li)
	if err != nil {
		return -1
	}
	return int64(li.CreditBalance)
}

// pageCredits returns the credit balance of a key page, or -1 if unknown.
func (e *engine) pageCredits(page *url.URL) int64 {
	c, _ := e.pageInfo(page)
	return c
}

// pageInfo returns (creditBalance, version) for a key page, or (-1, 0) if the
// page is not queryable. Version is the live signer version, which the network
// requires every page signature to match.
func (e *engine) pageInfo(page *url.URL) (int64, uint64) {
	q := e.pc.querier(page)
	var kp *protocol.KeyPage
	_, err := q.QueryAccountAs(e.ctx, page, nil, &kp)
	if err != nil {
		return -1, 0
	}
	return int64(kp.CreditBalance), kp.Version
}

const creditBuyAcme = 5.0 // ACME spent per AddCredits -> thousands of credits at this oracle

// stepCommitTimeout bounds how long we poll for a build-out step to commit
// before re-submitting it (covers dropped/delayed synthetic transactions).
const stepCommitTimeout = 25 * time.Second

// gatedStep runs one build-out step. `done` reports whether the step's effect
// is already observable (so we advance without submitting). Otherwise, if we're
// inside the poll window from a prior submit we just wait; if the window has
// elapsed (or we never submitted) we build+submit and arm the window. Returns
// true when the step completed (caller should advance to next).
func (e *engine) gatedStep(a *actor, label string, principal *url.URL, done func() bool, buildEnv func() (*messaging.Envelope, error)) bool {
	if done() {
		a.waitUntil = time.Time{}
		return true
	}
	if time.Now().Before(a.waitUntil) {
		e.m.notReady.Add(1)
		return false
	}
	env, err := buildEnv()
	if err != nil {
		e.m.otherErr.Add(1)
		return false
	}
	if e.submit(principal, label, env) {
		// Give the (possibly synthetic) effect time to land before re-firing.
		a.waitUntil = time.Now().Add(stepCommitTimeout)
		// Opportunistic immediate check.
		if done() {
			a.waitUntil = time.Time{}
			return true
		}
	}
	return false
}

// advance moves an actor forward by at most one submitted transaction. Steps
// before stepActive are confirmation-gated: we only submit the next step once
// the previous step's account/credits are observable. Re-submission of a
// not-yet-committed step is throttled via gatedStep so we don't flood the
// mempool with duplicate fund/create txns.
func (e *engine) advance(a *actor, rng *mrand.Rand) {
	switch a.step {
	case stepFundLite:
		if e.gatedStep(a, "sendTokens(fund-lite)", e.faucetLta,
			func() bool { return e.accountExists(a.lta) },
			func() (*messaging.Envelope, error) {
				nonce := e.fnonce()
				return build.Transaction().For(e.faucetLta).
					SendTokens(10, protocol.AcmePrecisionPower).To(a.lta).
					SignWith(e.faucetLid).Version(1).Timestamp(&nonce).PrivateKey(e.fsk).Done()
			}) {
			a.step = stepBuyLiteCredits
		}

	case stepBuyLiteCredits:
		if e.gatedStep(a, "addCredits(lite)", a.lta,
			func() bool { return e.liteCredits(a.lid) > 1000 },
			func() (*messaging.Envelope, error) {
				a.nonceLite++
				nonce := a.nonceLite
				return build.Transaction().For(a.lta).
					AddCredits().To(a.lid).Spend(creditBuyAcme).WithOracle(e.oracle).
					SignWith(a.lid).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
			}) {
			a.step = stepCreateADI
		}

	case stepCreateADI:
		if e.gatedStep(a, "createIdentity", a.lta,
			func() bool { return e.accountExists(a.adi) },
			func() (*messaging.Envelope, error) {
				a.nonceLite++
				nonce := a.nonceLite
				return build.Transaction().For(a.lta).
					CreateIdentity(a.adi).WithKeyHash(a.kh[:]).WithKeyBook(a.book).
					SignWith(a.lid).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
			}) {
			a.step = stepBuyPageCredits
		}

	case stepBuyPageCredits:
		if e.gatedStep(a, "addCredits(page)", a.lta,
			func() bool { return e.pageCredits(a.page) > 2000 },
			func() (*messaging.Envelope, error) {
				a.nonceLite++
				nonce := a.nonceLite
				return build.Transaction().For(a.lta).
					AddCredits().To(a.page).Spend(creditBuyAcme).WithOracle(e.oracle).
					SignWith(a.lid).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
			}) {
			a.step = stepCreateTokenAcct
		}

	case stepCreateTokenAcct:
		if e.gatedStep(a, "createTokenAccount", a.adi,
			func() bool { return e.accountExists(a.tokens) },
			func() (*messaging.Envelope, error) {
				a.noncePage++
				nonce := a.noncePage
				return build.Transaction().For(a.adi).
					CreateTokenAccount(a.tokens).ForToken(protocol.AcmeUrl()).
					SignWith(a.page).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
			}) {
			a.step = stepFundTokenAcct
		}

	case stepFundTokenAcct:
		// Funding is synthetic; advance as soon as it's accepted (don't gate on
		// balance, which would require tracking exact amounts).
		a.nonceLite++
		nonce := a.nonceLite
		env, err := build.Transaction().For(a.lta).
			SendTokens(5, protocol.AcmePrecisionPower).To(a.tokens).
			SignWith(a.lid).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
		if err != nil {
			e.m.otherErr.Add(1)
			return
		}
		if e.submit(a.lta, "sendTokens(fund-adi)", env) {
			a.step = stepCreateDataAcct
		}

	case stepCreateDataAcct:
		if e.gatedStep(a, "createDataAccount", a.adi,
			func() bool { return e.accountExists(a.data) },
			func() (*messaging.Envelope, error) {
				a.noncePage++
				nonce := a.noncePage
				return build.Transaction().For(a.adi).
					CreateDataAccount(a.data).
					SignWith(a.page).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
			}) {
			a.step = stepAddSecondKey
		}

	case stepAddSecondKey:
		// Add key2 to book/1 and set threshold=2 (multi-sig). Single-signed by
		// the current (1-of-1) page; gate on the page reaching version 2.
		if e.gatedStep(a, "updateKeyPage(2of2)", a.page,
			func() bool { _, v := e.pageInfo(a.page); return v >= 2 },
			func() (*messaging.Envelope, error) {
				a.noncePage++
				nonce := a.noncePage
				return build.Transaction().For(a.page).
					UpdateKeyPage().
					Add().Entry().Hash(a.kh2[:]).FinishEntry().FinishOperation().
					SetThreshold(2).
					SignWith(a.page).Version(1).Timestamp(&nonce).PrivateKey(a.sk).Done()
			}) {
			a.multiSig = true
			a.pageVer = 2
			a.step = stepActive
			registerActive(a)
		}

	case stepActive:
		e.active(a, rng)
	}
}

// active performs one weighted-random operation for a fully-built actor. It
// fetches the page's live credit balance + signer version once and threads the
// version through every page-signed op (the network rejects stale versions).
func (e *engine) active(a *actor, rng *mrand.Rand) {
	credits, ver := e.pageInfo(a.page)
	if ver == 0 {
		ver = a.pageVer // page momentarily unqueryable; fall back to tracked
	} else {
		a.pageVer = ver
	}

	// Maintain credits so signed txns don't fail.
	if credits >= 0 && credits < 800 {
		a.nonceLite++
		nonce := a.nonceLite
		env, err := build.Transaction().
			For(a.lta).
			AddCredits().To(a.page).Spend(creditBuyAcme).WithOracle(e.oracle).
			SignWith(a.lid).Version(1).Timestamp(&nonce).PrivateKey(a.sk).
			Done()
		if err == nil {
			e.submit(a.lta, "addCredits(topup)", env)
		}
		return
	}

	// Weighted mix. Multi-sig variants sign 2-of-2 with the page.
	r := rng.Intn(100)
	switch {
	case r < 35:
		e.opWriteData(a, ver)
	case r < 55:
		e.opSendTokensAdi(a, ver, false)
	case r < 65:
		e.opSendTokensLite(a)
	case r < 80:
		e.opSendTokensAdi(a, ver, true)
	case r < 90:
		e.opMultiSigWrite(a, ver)
	default:
		e.opCreateExtraDataAccount(a, ver)
	}
}

// pageSign applies the required page signatures for an actor: 2-of-2 when
// multi-sig is enabled, otherwise a single signature. `b` is the builder right
// after the first `.SignWith(a.page)`. All signatures use version `ver`.
func pageSign(b build.SignatureBuilder, a *actor, ver uint64, ts *uint64) build.SignatureBuilder {
	b = b.Version(ver).Timestamp(ts).PrivateKey(a.sk)
	if a.multiSig {
		b = b.SignWith(a.page).Version(ver).Timestamp(ts).PrivateKey(a.sk2)
	}
	return b
}

func (e *engine) opWriteData(a *actor, ver uint64) {
	a.noncePage += 2
	ts := a.noncePage
	payload := fmt.Sprintf("loadmix:%d:%d", a.id, time.Now().UnixNano())
	env, err := pageSign(
		build.Transaction().For(a.data).WriteData().DoubleHash([]byte("mix"), []byte(payload)).
			SignWith(a.page),
		a, ver, &ts).Done()
	if err != nil {
		e.m.otherErr.Add(1)
		return
	}
	e.submit(a.data, "writeData", env)
}

func (e *engine) opMultiSigWrite(a *actor, ver uint64) {
	if !a.multiSig {
		e.opWriteData(a, ver)
		return
	}
	a.noncePage += 2
	ts := a.noncePage
	payload := fmt.Sprintf("multisig-write:%d:%d", a.id, time.Now().UnixNano())
	env, err := pageSign(
		build.Transaction().For(a.data).WriteData().DoubleHash([]byte("ms"), []byte(payload)).
			SignWith(a.page),
		a, ver, &ts).Done()
	if err != nil {
		e.m.otherErr.Add(1)
		return
	}
	e.submit(a.data, "multisig-writeData", env)
}

// opSendTokensAdi sends 1 unit ACME from this actor's ADI token account to
// another actor's ADI token account (cross-ADI, often cross-BVN). If multisig
// is set the label is "multisig-sendTokens".
func (e *engine) opSendTokensAdi(a *actor, ver uint64, multilabel bool) {
	other := e.pickOtherTokens(a)
	if other == nil {
		e.opWriteData(a, ver)
		return
	}
	a.noncePage += 2
	ts := a.noncePage
	env, err := pageSign(
		build.Transaction().For(a.tokens).SendTokens(1, 0).To(other).SignWith(a.page),
		a, ver, &ts).Done()
	if err != nil {
		e.m.otherErr.Add(1)
		return
	}
	label := "sendTokens(adi)"
	if multilabel && a.multiSig {
		label = "multisig-sendTokens"
	}
	e.submit(a.tokens, label, env)
}

func (e *engine) opSendTokensLite(a *actor) {
	other := e.pickOtherLite(a)
	if other == nil {
		e.opWriteData(a, a.pageVer)
		return
	}
	a.nonceLite++
	nonce := a.nonceLite
	env, err := build.Transaction().
		For(a.lta).
		SendTokens(1, 0).To(other).
		SignWith(a.lid).Version(1).Timestamp(&nonce).PrivateKey(a.sk).
		Done()
	if err != nil {
		e.m.otherErr.Add(1)
		return
	}
	e.submit(a.lta, "sendTokens(lite)", env)
}

func (e *engine) opCreateExtraDataAccount(a *actor, ver uint64) {
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	name := "data-" + hex.EncodeToString(rnd[:])
	extra := a.adi.JoinPath(name)
	a.noncePage += 2
	ts := a.noncePage
	env, err := pageSign(
		build.Transaction().For(a.adi).CreateDataAccount(extra).SignWith(a.page),
		a, ver, &ts).Done()
	if err != nil {
		e.m.otherErr.Add(1)
		return
	}
	e.submit(a.adi, "createDataAccount(extra)", env)
}

// pickOtherTokens picks another active actor's ADI token account.
func (e *engine) pickOtherTokens(a *actor) *url.URL {
	o := pickOther(a)
	if o == nil {
		return nil
	}
	return o.tokens
}

func (e *engine) pickOtherLite(a *actor) *url.URL {
	o := pickOther(a)
	if o == nil {
		return nil
	}
	return o.lta
}

// activeActors is a snapshot registry the engine consults to pick send targets.
var (
	activeMu  sync.RWMutex
	activeReg []*actor
)

func registerActive(a *actor) {
	activeMu.Lock()
	activeReg = append(activeReg, a)
	activeMu.Unlock()
}

func pickOther(self *actor) *actor {
	activeMu.RLock()
	defer activeMu.RUnlock()
	n := len(activeReg)
	if n < 2 {
		return nil
	}
	for i := 0; i < 4; i++ {
		o := activeReg[mrand.Intn(n)]
		if o != self && o.tokens != nil {
			return o
		}
	}
	return nil
}

// pickActiveExcept returns a random fully-built (ACTIVE) actor other than self,
// or nil if none. Used so a tick that lands on a throttled build-out actor can
// still drive steady-state load.
func pickActiveExcept(self *actor) *actor {
	activeMu.RLock()
	defer activeMu.RUnlock()
	n := len(activeReg)
	if n == 0 {
		return nil
	}
	for i := 0; i < 5; i++ {
		o := activeReg[mrand.Intn(n)]
		if o != self {
			return o
		}
	}
	return nil
}
