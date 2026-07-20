// Copyright 2025 The Accumulate Authors
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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The synthetic types the workload is expected to produce. These are the names
// the tracker reports, so they come from the enums rather than string literals.
var (
	synthDepositTokens  = protocol.TransactionTypeSyntheticDepositTokens.String()
	synthCreateIdentity = protocol.TransactionTypeSyntheticCreateIdentity.String()
	synthDepositCredits = protocol.TransactionTypeSyntheticDepositCredits.String()
	synthBurnTokens     = protocol.TransactionTypeSyntheticBurnTokens.String()
	synthWriteData      = protocol.TransactionTypeSyntheticWriteData.String()
	msgSignatureRequest = messaging.MessageTypeSignatureRequest.String()
	msgCreditPayment    = messaging.MessageTypeCreditPayment.String()
)

// kind is one flavour of transaction in the workload.
type kind struct {
	name   string
	weight int
	// synth lists the cross-partition synthetic types this kind must produce.
	// The verdict fails if any of them is never seen.
	synth []string
	run   func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error)
}

// transferKinds is the original #4064 repro: lite -> lite ACME sends only.
var transferKinds = []kind{liteTransfer}

// mixedKinds is the default workload: every user transaction type that produces
// a cross-partition synthetic, weighted so the cheap high-volume ones dominate
// but every path is exercised regularly.
var mixedKinds = []kind{
	liteTransfer, // 4
	adiTransfer,  // 3
	dataWrite,    // 3
	creditBuy,    // 3
	crossAuth,    // 3
	tokenBurn,    // 1
	adiCreate,    // 1
}

var liteTransfer = kind{
	name: "lite-transfer", weight: 4,
	synth: []string{synthDepositTokens},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		t := w.target(i)
		env, err := build.Transaction().For(e.faucet).
			SendTokens(1, protocol.AcmePrecisionPower).To(t.lite).
			SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

var adiTransfer = kind{
	name: "adi-transfer", weight: 3,
	synth: []string{synthDepositTokens},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		t := w.target(i)
		env, err := build.Transaction().For(w.homeTokens).
			SendTokens(1, protocol.AcmePrecisionPower).To(t.lite).
			SignWith(w.homePage).Version(1).Timestamp(e.next()).PrivateKey(w.homeKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

var dataWrite = kind{
	name: "data-write", weight: 3,
	synth: []string{synthWriteData},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		t := w.target(i)
		env, err := build.Transaction().For(e.faucet).
			WriteData().DoubleHash("synth-mixed", fmt.Sprintf("write-%d", i)).To(t.lda).
			SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

var creditBuy = kind{
	name: "credit-purchase", weight: 3,
	synth: []string{synthDepositCredits},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		t := w.target(i)
		env, err := build.Transaction().For(e.faucet).
			AddCredits().WithOracle(e.oracle).Purchase(100).To(t.lid).
			SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

// crossAuth exercises the MessageForTransaction heal path: the principal lives
// on one partition and the sole authority that can sign for it lives on
// another, so initiating produces a cross-partition SignatureRequest (to the
// principal) and CreditPayment (to the principal, from the paying signer).
var crossAuth = kind{
	name: "cross-auth", weight: 3,
	synth: []string{msgSignatureRequest, msgCreditPayment},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		env, err := build.Transaction().For(w.remoteData).
			WriteData().DoubleHash("synth-mixed", fmt.Sprintf("auth-%d", i)).
			SignWith(w.homePage).Version(1).Timestamp(e.next()).PrivateKey(w.homeKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

// tokenBurn produces a SyntheticBurnTokens addressed to the ACME issuer, which
// is route-overridden to the Directory — always cross-partition from a BVN.
var tokenBurn = kind{
	name: "token-burn", weight: 1,
	synth: []string{synthBurnTokens},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		env, err := build.Transaction().For(e.faucet).
			BurnTokens(1, protocol.AcmePrecisionPower).
			SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

var adiCreate = kind{
	name: "adi-create", weight: 1,
	synth: []string{synthCreateIdentity},
	run: func(ctx context.Context, e *env, w *world, i int) ([]*url.TxID, error) {
		t := w.target(i)
		// A fresh ADI on a foreign partition: because the principal (the faucet
		// lite account) is not local to it, the ADI is created by a synthetic
		// transaction that has to cross the partition boundary.
		adi := adiRoutingTo(e.tree, t.part, fmt.Sprintf("%s-new%d", w.prefix, i))
		key := make([]byte, ed25519.PublicKeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		env, err := build.Transaction().For(e.faucet).
			CreateIdentity(adi).WithKeyHash(key).WithKeyBook(adi, "book").
			SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
		if err != nil {
			return nil, err
		}
		return trySubmit(ctx, e.c, env)
	},
}

// expectedSynthetics is the set of synthetic types the given workload must
// produce for the run to count as covering everything.
func expectedSynthetics(kinds []kind) map[string]bool {
	want := map[string]bool{}
	for _, k := range kinds {
		for _, s := range k.synth {
			want[s] = true
		}
	}
	return want
}

// schedule expands kinds by weight into a cycle the workload indexes into, so
// stepping through it in order reproduces the intended mix.
func schedule(kinds []kind) []kind {
	var s []kind
	for _, k := range kinds {
		for i := 0; i < k.weight; i++ {
			s = append(s, k)
		}
	}
	return s
}

// target is one foreign partition and the accounts the workload aims at it.
type target struct {
	part string
	lite *url.URL // lite ACME token account
	lid  *url.URL // its lite identity, the AddCredits recipient
	lda  *url.URL // lite data account, the WriteDataTo recipient
}

// world holds every account the workload needs, created by bootstrap.
type world struct {
	prefix  string
	targets []*target

	// home is an ADI on the faucet's partition. Its key page pays for, and
	// signs, everything the faucet's lite identity cannot.
	homeKey    ed25519.PrivateKey
	home       *url.URL
	homePage   *url.URL
	homeTokens *url.URL

	// remote is an ADI on a foreign partition whose only authority is
	// home/book — so signing for it always crosses a partition boundary.
	remote     *url.URL
	remoteData *url.URL
}

func (w *world) target(i int) *target { return w.targets[i%len(w.targets)] }

// bootstrap creates every account the workload needs and waits for each step to
// be delivered before building on it.
func bootstrap(ctx context.Context, e *env, parts []string, kinds []kind) *world {
	need := map[string]bool{}
	for _, k := range kinds {
		need[k.name] = true
	}
	wantADI := need[adiTransfer.name] || need[crossAuth.name]
	wantLDA := need[dataWrite.name]

	w := &world{prefix: fmt.Sprintf("mx%x", time.Now().UTC().Unix())}

	// Derive every target account up front, before submitting anything, so
	// that env.waitFor always has somewhere to aim a nudge — including while
	// waiting on the very first transaction.
	entries := map[string]protocol.DataEntry{}
	for _, p := range parts {
		_, lite := keyRoutingTo(e.tree, p)
		t := &target{part: p, lite: lite, lid: lite.RootIdentity()}
		if wantLDA {
			// A lite data account's URL is derived from its first entry, so
			// the entry we brute-forced has to be the one that creates it.
			entries[p], t.lda = ldaRoutingTo(e.tree, p)
		}
		w.targets = append(w.targets, t)
		e.nudgeTo[p] = lite
	}

	for _, t := range w.targets {
		// Fund the lite account. This also creates it and its identity, so
		// later credit purchases have somewhere to land.
		ids := e.send(ctx, build.Transaction().For(e.faucet).
			SendTokens(10, protocol.AcmePrecisionPower).To(t.lite).
			SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey))
		waitForAll(ctx, e.Q, ids, 4*time.Minute)
		e.waitForAccount(ctx, t.lite, 4*time.Minute)

		if wantLDA {
			ids = e.send(ctx, build.Transaction().For(e.faucet).
				WriteData().Entry(entries[t.part]).To(t.lda).
				SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey))
			waitForAll(ctx, e.Q, ids, 4*time.Minute)
			e.waitForAccount(ctx, t.lda, 4*time.Minute)
		}
		log.Printf("target %s: lite=%v lda=%v", t.part, t.lite, t.lda)
	}

	if !wantADI {
		return w
	}

	// The home ADI, on the faucet's own partition.
	_, w.homeKey, _ = ed25519.GenerateKey(rand.Reader)
	w.home = adiRoutingTo(e.tree, e.srcPart, w.prefix+"-home")
	w.homePage = w.home.JoinPath("book", "1")
	w.homeTokens = w.home.JoinPath("tokens")

	ids := e.send(ctx, build.Transaction().For(e.faucet).
		CreateIdentity(w.home).WithKey(w.homeKey, protocol.SignatureTypeED25519).WithKeyBook(w.home, "book").
		SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey))
	waitForAll(ctx, e.Q, ids, 4*time.Minute)
	// The ADI is created by a synthetic transaction, so the key page does not
	// exist the moment the CreateIdentity is delivered — and credits cannot be
	// bought for a page that is not there yet.
	e.waitForAccount(ctx, w.homePage, 4*time.Minute)

	ids = e.send(ctx, build.Transaction().For(e.faucet).
		AddCredits().WithOracle(e.oracle).Purchase(homeCredits).To(w.homePage).
		SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey))
	waitForAll(ctx, e.Q, ids, 4*time.Minute)
	// The page has to actually hold credits before it can sign anything.
	e.waitForCredits(ctx, w.homePage, 4*time.Minute)

	ids = e.send(ctx, build.Transaction().For(w.home).
		CreateTokenAccount(w.homeTokens).ForToken(protocol.AcmeUrl()).
		SignWith(w.homePage).Version(1).Timestamp(e.next()).PrivateKey(w.homeKey))
	waitForAll(ctx, e.Q, ids, 4*time.Minute)
	e.waitForAccount(ctx, w.homeTokens, 4*time.Minute)

	ids = e.send(ctx, build.Transaction().For(e.faucet).
		SendTokens(100000, protocol.AcmePrecisionPower).To(w.homeTokens).
		SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey))
	waitForAll(ctx, e.Q, ids, 4*time.Minute)
	log.Printf("home ADI %v on %s (page %v)", w.home, e.srcPart, w.homePage)

	if !need[crossAuth.name] {
		return w
	}

	// The remote ADI: created on a foreign partition with home/book as its
	// only authority. Creating it already crosses the boundary twice — the
	// signature is processed on home's partition, so the CreditPayment and the
	// SignatureRequest both travel to remote's partition.
	w.remote = adiRoutingTo(e.tree, w.targets[0].part, w.prefix+"-remote")
	w.remoteData = w.remote.JoinPath("data")

	ids = e.send(ctx, build.Transaction().For(w.remote).
		CreateIdentity(w.remote).WithAuthority(w.home, "book").
		SignWith(w.homePage).Version(1).Timestamp(e.next()).PrivateKey(w.homeKey))
	waitForAll(ctx, e.Q, ids, 4*time.Minute)
	e.waitForAccount(ctx, w.remote, 4*time.Minute)

	ids = e.send(ctx, build.Transaction().For(w.remote).
		CreateDataAccount(w.remoteData).
		SignWith(w.homePage).Version(1).Timestamp(e.next()).PrivateKey(w.homeKey))
	waitForAll(ctx, e.Q, ids, 4*time.Minute)
	e.waitForAccount(ctx, w.remoteData, 4*time.Minute)
	log.Printf("remote ADI %v on %s, authority %v on %s", w.remote, w.targets[0].part, w.home.JoinPath("book"), e.srcPart)

	return w
}

// homeCredits is how many credits to buy for the home key page at bootstrap and
// on each top-up. A cross-auth write costs a few credits, so this covers a long
// soak between top-ups.
const homeCredits = 500000

// topUp buys more credits for the home key page if it is running low. A soak
// runs long enough to exhaust any fixed initial purchase.
func (w *world) topUp(ctx context.Context, e *env) {
	if w.homePage == nil {
		return
	}
	var page *protocol.KeyPage
	if _, err := e.Q.QueryAccountAs(ctx, w.homePage, nil, &page); err != nil {
		return
	}
	if page.CreditBalance > homeCredits/10*protocol.CreditPrecision {
		return
	}
	env, err := build.Transaction().For(e.faucet).
		AddCredits().WithOracle(e.oracle).Purchase(homeCredits).To(w.homePage).
		SignWith(e.faucetLID).Version(1).Timestamp(e.next()).PrivateKey(e.faucetKey).Done()
	if err != nil {
		return
	}
	if _, err := trySubmit(ctx, e.c, env); err != nil {
		log.Printf("credit top-up failed: %v", err)
		return
	}
	log.Printf("topped up %v (balance was %d)", w.homePage, page.CreditBalance)
}

// send builds and submits an envelope, failing the process on error. Bootstrap
// steps must succeed; the workload loop uses trySubmit instead.
func (e *env) send(ctx context.Context, b interface {
	Done() (*messaging.Envelope, error)
}) []*url.TxID {
	env, err := b.Done()
	fatalIf(err, "build")
	return submit(ctx, e.c, env)
}
