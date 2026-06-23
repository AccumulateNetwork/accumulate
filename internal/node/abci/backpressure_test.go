// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"crypto/ed25519"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// userEnvRaw builds and marshals a real user submission (a lite-account token
// send) for use as a backpressure-subject envelope.
func userEnvRaw(t *testing.T) []byte {
	t.Helper()
	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	lite := protocol.LiteAuthorityForKey(sk[32:], protocol.SignatureTypeED25519)
	lta := lite.JoinPath("ACME")
	ts := uint64(1)
	env, err := build.Transaction().For(lta).
		SendTokens(1, 0).To(lta).
		SignWith(lite).Version(1).Timestamp(&ts).PrivateKey(sk).Done()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := env.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestMempoolBackpressureReject covers the load-shed decision for USER traffic.
// The threshold comes solely from the cached global; this verifies the >=
// boundary for configured values, that an unset (0) or out-of-range value
// disables shedding (and is never misread as "shed everything"), and the
// recheck / not-wired / cap-zero exemptions.
func TestMempoolBackpressureReject(t *testing.T) {
	const poolCap = 5000
	user := userEnvRaw(t)
	cases := []struct {
		name    string
		wire    bool
		size    int
		cap     int
		pct     int64 // cached global threshold; 1-100 active, else disabled
		recheck bool
		want    bool
	}{
		{name: "limiter not wired", wire: false, size: poolCap, pct: 20, want: false},
		{name: "cap zero", wire: true, size: poolCap, cap: 0, pct: 20, want: false},
		{name: "recheck never rejected", wire: true, size: poolCap, cap: poolCap, pct: 20, recheck: true, want: false},
		{name: "empty mempool", wire: true, size: 0, cap: poolCap, pct: 20, want: false},

		// Unset/out-of-range disables shedding — and must NOT be read as a 0%
		// threshold, which would shed even a full mempool.
		{name: "unset (0) disables, full admitted", wire: true, size: poolCap, cap: poolCap, pct: 0, want: false},
		{name: "over 100 disables, full admitted", wire: true, size: poolCap, cap: poolCap, pct: 150, want: false},

		// Genesis default 20% of 5000 => threshold 1000.
		{name: "20pct just below", wire: true, size: 999, cap: poolCap, pct: 20, want: false},
		{name: "20pct at", wire: true, size: 1000, cap: poolCap, pct: 20, want: true},
		{name: "20pct above", wire: true, size: 2000, cap: poolCap, pct: 20, want: true},

		{name: "custom 70 below", wire: true, size: 3499, cap: poolCap, pct: 70, want: false},
		{name: "custom 70 at", wire: true, size: 3500, cap: poolCap, pct: 70, want: true},
		{name: "custom 100 below full", wire: true, size: 4999, cap: poolCap, pct: 100, want: false},
		{name: "custom 100 full", wire: true, size: 5000, cap: poolCap, pct: 100, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := new(Accumulator)
			if c.wire {
				size := c.size
				app.SetMempoolLimiter(func() int { return size }, c.cap)
			}
			app.mempoolBackpressurePct.Store(c.pct)

			_, got := app.mempoolBackpressureReject(c.recheck, user)
			if got != c.want {
				t.Fatalf("reject=%v, want %v", got, c.want)
			}
		})
	}
}

// TestBackpressureAdmitsGarbageAboveThreshold locks the #3 invariant: an
// undecodable envelope is admitted even when the mempool is full, so it reaches
// Validate for a terminal (non-retryable) decode error instead of being shed
// with a retryable "backpressure" rejection that a client would resend in a loop.
func TestBackpressureAdmitsGarbageAboveThreshold(t *testing.T) {
	app := new(Accumulator)
	app.SetMempoolLimiter(func() int { return 5000 }, 5000) // 100% full
	app.mempoolBackpressurePct.Store(50)
	if _, reject := app.mempoolBackpressureReject(false, []byte("not an envelope")); reject {
		t.Fatal("garbage shed as backpressure; must be admitted for a terminal decode error")
	}
}

// TestBackpressureNeverShedsSynthetics is the crucial property: even when the
// mempool is full, a synthetic/system submission is admitted.
func TestBackpressureNeverShedsSynthetics(t *testing.T) {
	app := new(Accumulator)
	app.SetMempoolLimiter(func() int { return 5000 }, 5000) // 100% full
	app.mempoolBackpressurePct.Store(50)

	// A synthetic deposit (system transaction) must pass.
	synthMsgs := []messaging.Message{
		&messaging.TransactionMessage{Transaction: &protocol.Transaction{Body: &protocol.SyntheticDepositTokens{}}},
	}
	if messagesAreUser(synthMsgs) {
		t.Fatal("synthetic transaction misclassified as user")
	}
	// And the same applies to anchors / sequenced / synthetic wrappers.
	for _, m := range []messaging.Message{
		&messaging.SyntheticMessage{}, &messaging.BlockAnchor{}, &messaging.SequencedMessage{},
	} {
		if messagesAreUser([]messaging.Message{m}) {
			t.Fatalf("%T misclassified as user", m)
		}
	}
}

// TestMessagesAreUser covers the user/non-user classification directly.
func TestMessagesAreUser(t *testing.T) {
	tx := func(b protocol.TransactionBody) messaging.Message {
		return &messaging.TransactionMessage{Transaction: &protocol.Transaction{Body: b}}
	}
	cases := []struct {
		name string
		msgs []messaging.Message
		want bool
	}{
		{"user transaction", []messaging.Message{tx(&protocol.SendTokens{})}, true},
		{"synthetic transaction", []messaging.Message{tx(&protocol.SyntheticDepositTokens{})}, false},
		{"user signature", []messaging.Message{&messaging.SignatureMessage{Signature: &protocol.ED25519Signature{}}}, true},
		{"system signature", []messaging.Message{&messaging.SignatureMessage{Signature: &protocol.PartitionSignature{}}}, false},
		{"synthetic message", []messaging.Message{&messaging.SyntheticMessage{}}, false},
		{"block anchor", []messaging.Message{&messaging.BlockAnchor{}}, false},
		{"empty", nil, false},
		{"user tx mixed with system → not user", []messaging.Message{tx(&protocol.SendTokens{}), &messaging.BlockAnchor{}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := messagesAreUser(c.msgs); got != c.want {
				t.Fatalf("messagesAreUser=%v, want %v", got, c.want)
			}
		})
	}
}

// TestIsUserSubmission exercises the full decode→normalize→classify path on a
// real marshaled user envelope, and confirms garbage decodes to "not user"
// (admit) rather than being shed.
func TestIsUserSubmission(t *testing.T) {
	app := new(Accumulator)
	if !app.isUserSubmission(userEnvRaw(t)) {
		t.Fatal("real user envelope classified as non-user")
	}
	if app.isUserSubmission([]byte("not an envelope")) {
		t.Fatal("garbage classified as user (would be shed); must admit")
	}
}

func TestSetMempoolLimiter(t *testing.T) {
	app := new(Accumulator)
	app.SetMempoolLimiter(func() int { return 42 }, 5000)
	if app.mempoolSize == nil || app.mempoolSize() != 42 {
		t.Fatalf("mempoolSize not wired")
	}
	if app.mempoolCap != 5000 {
		t.Fatalf("mempoolCap = %d, want 5000", app.mempoolCap)
	}
}

// TestWillChangeGlobalsCachesBackpressure verifies the threshold is sourced
// from the network globals via the same event the oracle/fee schedule use,
// and is captured on first load (Old == nil).
func TestWillChangeGlobalsCachesBackpressure(t *testing.T) {
	app := new(Accumulator)
	err := app.willChangeGlobals(events.WillChangeGlobals{
		New: &core.GlobalValues{
			Globals: &protocol.NetworkGlobals{
				Limits: &protocol.NetworkLimits{MempoolBackpressurePercent: 73},
			},
		},
	})
	if err != nil {
		t.Fatalf("willChangeGlobals: %v", err)
	}
	if got := app.mempoolBackpressurePct.Load(); got != 73 {
		t.Fatalf("cached pct = %d, want 73", got)
	}
}

// TestWillChangeGlobalsNilSafe ensures missing globals/limits don't panic and
// leave the cached threshold untouched (so CheckTx falls back to the default).
func TestWillChangeGlobalsNilSafe(t *testing.T) {
	app := new(Accumulator)
	for _, e := range []events.WillChangeGlobals{
		{},
		{New: &core.GlobalValues{}},
		{New: &core.GlobalValues{Globals: &protocol.NetworkGlobals{}}},
	} {
		if err := app.willChangeGlobals(e); err != nil {
			t.Fatalf("willChangeGlobals: %v", err)
		}
	}
	if got := app.mempoolBackpressurePct.Load(); got != 0 {
		t.Fatalf("expected pct unchanged (0), got %d", got)
	}
}
