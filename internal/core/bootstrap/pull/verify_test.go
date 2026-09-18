// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	apierrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// servedBlock is the block the fake peers claim to have served state at.
const servedBlock = 17

// peer serves state out of a database and, with it, the receipt the real
// querier serves: the account's state proven into that database's BPT root.
// corrupt, when set, rewrites the body after the receipt is built — a peer
// that proves one account and hands over another.
type peer struct {
	*dbSource
	corrupt func(protocol.Account) protocol.Account

	// block is the block this peer claims to have served the state at. Zero
	// means servedBlock. A peer naming a block the Directory has not anchored
	// is either running ahead of the anchors or lying, and the puller cannot
	// tell which — see TestAccountFrom_AsksThePeersPastAnEarlyBlock.
	block uint64

	// empty makes the peer answer with a record carrying no account, which is
	// how a peer that has nothing to serve answers.
	empty bool
}

func (s *peer) QueryAccount(_ context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	if s.empty {
		return new(api.AccountRecord), nil
	}

	b := s.db.Begin(false)
	defer b.Discard()

	var acct protocol.Account
	if err := b.Account(u).Main().GetAs(&acct); err != nil {
		return nil, err
	}
	rec := &api.AccountRecord{Account: acct}

	if q != nil && q.IncludeReceipt.Yes() {
		r, err := b.Account(u).StateReceipt()
		if err != nil {
			return nil, err
		}
		block := s.block
		if block == 0 {
			block = servedBlock
		}
		rec.Receipt = &api.Receipt{LocalBlock: block}
		rec.Receipt.Receipt = *r
	}

	if s.corrupt != nil {
		rec.Account = s.corrupt(acct)
	}
	return rec, nil
}

// anchored is a Verifier that answers with one root, as the Directory's anchor
// for servedBlock would.
type anchored struct {
	root  [32]byte
	block uint64
}

func (a anchored) AnchoredRoot(_ context.Context, _ *url.URL, block uint64) ([32]byte, error) {
	if a.block != 0 && block != a.block {
		return [32]byte{}, ErrNotAnchored
	}
	return a.root, nil
}

// alice builds a database holding one non-trivial account and returns it with
// the account's URL and the database's BPT root.
func alice(t *testing.T) (*database.Database, *url.URL, [32]byte) {
	t.Helper()
	u := protocol.DnUrl().JoinPath("alice")
	db := newObservedDB(t)

	b := db.Begin(true)
	if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		e := make([]byte, 32)
		e[0] = byte(i)
		e[31] = 0xab
		if err := b.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Account(u).Directory().Add(u.JoinPath("child")); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}

	ro := db.Begin(false)
	defer ro.Discard()
	root, err := ro.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	return db, u, root
}

func leafOf(t *testing.T, db *database.Database, u *url.URL) [32]byte {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	h, err := b.Account(u).Hash()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// bendUrl is a corruption a peer could plausibly attempt: the account it
// proved, with its URL changed.
func bendUrl(a protocol.Account) protocol.Account {
	d := a.(*protocol.DataAccount).Copy()
	d.Url = protocol.DnUrl().JoinPath("mallory")
	return d
}

// TestVerifiedPull_AcceptsHonestSource — the happy path. A peer that serves
// the state it proved is believed, and the pulled leaf is the source's leaf.
func TestVerifiedPull_AcceptsHonestSource(t *testing.T) {
	src, u, root := alice(t)

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	err := Account(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if err != nil {
		t.Fatalf("pull.Account: %v", err)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	if got, want := leafOf(t, dst, u), leafOf(t, src, u); got != want {
		t.Fatalf("leaf mismatch after a verified pull:\n  want %x\n  got  %x", want, got)
	}
}

// TestVerifiedPull_RefusesCorruptedAccount — a peer that proves one account
// and serves another is refused, and nothing it served is written.
func TestVerifiedPull_RefusesCorruptedAccount(t *testing.T) {
	src, u, root := alice(t)

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	err := Account(context.Background(), &peer{dbSource: &dbSource{db: src}, corrupt: bendUrl}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if err == nil {
		t.Fatal("a corrupted account was accepted")
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	ro := dst.Begin(false)
	defer ro.Discard()
	if _, err := ro.Account(u).Main().Get(); err == nil {
		t.Fatal("the refused state was written anyway")
	}
}

// TestAccountFrom_RePullsFromAnotherSource is the scenario the issue names: a
// corrupted account is refused and re-pulled from a second source.
func TestAccountFrom_RePullsFromAnotherSource(t *testing.T) {
	src, u, root := alice(t)

	bad := &peer{dbSource: &dbSource{db: src}, corrupt: bendUrl}
	good := &peer{dbSource: &dbSource{db: src}}

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	i, err := AccountFrom(context.Background(), []Source{bad, good}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if err != nil {
		t.Fatalf("AccountFrom: %v", err)
	}
	if i != 1 {
		t.Fatalf("source %d answered, want the second one", i)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}

	if got, want := leafOf(t, dst, u), leafOf(t, src, u); got != want {
		t.Fatalf("leaf mismatch after re-pulling from the second source:\n  want %x\n  got  %x", want, got)
	}
}

// TestAccountFrom_AllSourcesRefused — when nobody serves state that verifies,
// the pull fails rather than keeping the last answer.
func TestAccountFrom_AllSourcesRefused(t *testing.T) {
	src, u, root := alice(t)

	bad := &peer{dbSource: &dbSource{db: src}, corrupt: bendUrl}

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()
	if _, err := AccountFrom(context.Background(), []Source{bad, bad}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	}); err == nil {
		t.Fatal("two corrupt sources produced an accepted account")
	}
}

// TestVerifiedPull_RefusesUnanchoredRoot — a peer whose receipt ends somewhere
// other than the root the Directory anchored is refused, even when the state
// it served is internally consistent with that receipt.
func TestVerifiedPull_RefusesUnanchoredRoot(t *testing.T) {
	src, u, _ := alice(t)

	var other [32]byte
	other[0] = 0xff

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()
	err := Account(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: other, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if err == nil {
		t.Fatal("state proven into an unanchored root was accepted")
	}
}

// TestVerifiedPull_WaitsForTheAnchor — the Directory has not anchored the
// block the only peer served at. That is a wait, not a refusal, so the failure
// carries ErrNotAnchored and the caller asks again.
func TestVerifiedPull_WaitsForTheAnchor(t *testing.T) {
	src, u, root := alice(t)

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()

	p := &peer{dbSource: &dbSource{db: src}}
	_, err := AccountFrom(context.Background(), []Source{p}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock + 1}, // a different block
		Partition: protocol.DnUrl(),
	})
	if err == nil {
		t.Fatal("an unanchored block was accepted")
	}
	if !apierrors.Is(err, ErrNotAnchored) {
		t.Fatalf("err = %v, want ErrNotAnchored", err)
	}
}

// TestAccountFrom_AsksThePeersPastAnEarlyBlock — one peer answers at a block
// the Directory has not anchored. The puller cannot tell a peer running ahead
// of the anchors from one naming a block that will never be anchored, because
// the block it compares is the peer's own word; either way the answer is to
// ask the next source, not to stop the pull.
func TestAccountFrom_AsksThePeersPastAnEarlyBlock(t *testing.T) {
	src, u, root := alice(t)

	early := &peer{dbSource: &dbSource{db: src}, block: servedBlock + 99}
	good := &peer{dbSource: &dbSource{db: src}}

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	i, err := AccountFrom(context.Background(), []Source{early, good}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if err != nil {
		t.Fatalf("one peer at an unanchored block stopped the pull: %v", err)
	}
	if i != 1 {
		t.Fatalf("source %d answered, want the second one", i)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
	if got, want := leafOf(t, dst, u), leafOf(t, src, u); got != want {
		t.Fatalf("leaf mismatch after asking past the early peer:\n  want %x\n  got  %x", want, got)
	}
}

// TestAccountFrom_EveryPeerEarlyIsAWait — when no source could serve an
// anchored state, and only then, the failure is the wait ErrNotAnchored names.
func TestAccountFrom_EveryPeerEarlyIsAWait(t *testing.T) {
	src, u, root := alice(t)

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()

	_, err := AccountFrom(context.Background(), []Source{
		&peer{dbSource: &dbSource{db: src}, block: servedBlock + 1},
		&peer{dbSource: &dbSource{db: src}, block: servedBlock + 2},
	}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if !apierrors.Is(err, ErrNotAnchored) {
		t.Fatalf("err = %v, want ErrNotAnchored", err)
	}
}

// TestAccountFrom_APeerThatServesNothingFails — a peer answering with an empty
// record has served nothing. It used to read as success with no receipt, and a
// fetch with no receipt settled against any root at all.
func TestAccountFrom_APeerThatServesNothingFails(t *testing.T) {
	src, u, root := alice(t)

	nothing := &peer{dbSource: &dbSource{db: src}, empty: true}
	good := &peer{dbSource: &dbSource{db: src}}

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	i, err := AccountFrom(context.Background(), []Source{nothing, good}, batch, u, Options{
		Mode:      ModeStateOnly,
		Verify:    anchored{root: root, block: servedBlock},
		Partition: protocol.DnUrl(),
	})
	if err != nil {
		t.Fatalf("AccountFrom: %v", err)
	}
	if i != 1 {
		t.Fatalf("source %d answered, want the second one", i)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
	if got, want := leafOf(t, dst, u), leafOf(t, src, u); got != want {
		t.Fatalf("leaf mismatch:\n  want %x\n  got  %x", want, got)
	}
}

// TestSettle_RefusesWithoutAReceipt — Settle is the point where an unverified
// fetch could become state the node believes. It cannot succeed on a fetch it
// has no receipt for, nor against a root nobody anchored.
func TestSettle_RefusesWithoutAReceipt(t *testing.T) {
	src, u, root := alice(t)

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()

	// Fetched without asking for a receipt: nothing to settle against.
	p, err := Fetch(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u,
		Options{Mode: ModeStateOnly, Partition: protocol.DnUrl()}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Settle(root); err == nil {
		t.Fatal("a fetch with no receipt settled")
	}

	// And a root nobody anchored is not a root.
	p, err = Fetch(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u,
		Options{Mode: ModeStateOnly, Partition: protocol.DnUrl()}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Settle([32]byte{}); err == nil {
		t.Fatal("a zero root settled")
	}
}

// TestFetch_RequiresThePartition — a receipt proves the state as of a block,
// and a block number without its partition names nothing (#4205).
func TestFetch_RequiresThePartition(t *testing.T) {
	src, u, _ := alice(t)
	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()
	if _, err := Fetch(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u,
		Options{Mode: ModeStateOnly}, true); err == nil {
		t.Fatal("a receipt was asked for without naming the partition")
	}
}

// TestHeld_IsBounded — a fetched account holds an open child batch until it
// settles, so the number outstanding is bounded.
func TestHeld_IsBounded(t *testing.T) {
	src, u, _ := alice(t)
	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()

	before := Held()
	p, err := Fetch(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u,
		Options{Mode: ModeStateOnly, Partition: protocol.DnUrl()}, true)
	if err != nil {
		t.Fatal(err)
	}
	if Held() != before+1 {
		t.Fatalf("Held = %d, want %d while one account is outstanding", Held(), before+1)
	}
	p.Discard()
	if Held() != before {
		t.Fatalf("Held = %d after the fetch was discarded, want %d", Held(), before)
	}
	p.Discard() // idempotent
	if Held() != before {
		t.Fatalf("a second Discard moved Held to %d", Held())
	}
}

// TestVerifiedPull_RequiresPartition — verifying without saying which
// partition's blocks to look for is a programming error, not a silent skip.
func TestVerifiedPull_RequiresPartition(t *testing.T) {
	src, u, root := alice(t)
	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()
	if err := Account(context.Background(), &peer{dbSource: &dbSource{db: src}}, batch, u, Options{
		Mode:   ModeStateOnly,
		Verify: anchored{root: root},
	}); err == nil {
		t.Fatal("expected an error when Verify is set without Partition")
	}
}
