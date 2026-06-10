// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package orchestrator

import (
	"context"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/protocol/cyclopsrepair"
)

func newObservedDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

// fakeSource backs pull.Source + enumerate.Source + AnchorSource off
// a single src *database.Database, plus a programmable LatestAnchor.
type fakeSource struct {
	db     *database.Database
	anchor [32]byte
	block  uint64
}

func (s *fakeSource) QueryAccount(_ context.Context, u *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
	b := s.db.Begin(false)
	defer b.Discard()
	var acct protocol.Account
	if err := b.Account(u).Main().GetAs(&acct); err != nil {
		return nil, err
	}
	return &api.AccountRecord{Account: acct}, nil
}

func (s *fakeSource) QueryDirectoryUrls(_ context.Context, _ *url.URL, _ *api.DirectoryQuery) (*api.RecordRange[*api.UrlRecord], error) {
	return &api.RecordRange[*api.UrlRecord]{}, nil
}

func (s *fakeSource) QueryPendingIds(_ context.Context, _ *url.URL, _ *api.PendingQuery) (*api.RecordRange[*api.TxIDRecord], error) {
	return &api.RecordRange[*api.TxIDRecord]{}, nil
}

func (s *fakeSource) QueryAccountChains(_ context.Context, u *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
	b := s.db.Begin(false)
	defer b.Discard()
	chains, err := b.Account(u).Chains().Get()
	if err != nil {
		return &api.RecordRange[*api.ChainRecord]{}, nil
	}
	out := &api.RecordRange[*api.ChainRecord]{Total: uint64(len(chains))}
	for _, cm := range chains {
		c2, err := b.Account(u).ChainByName(cm.Name)
		if err != nil {
			return nil, err
		}
		head, err := c2.Head().Get()
		if err != nil {
			return nil, err
		}
		out.Records = append(out.Records, &api.ChainRecord{
			Name:  cm.Name,
			Type:  cm.Type,
			Count: uint64(head.Count),
			State: head.Pending,
		})
	}
	return out, nil
}

func (s *fakeSource) QueryChainEntries(_ context.Context, _ *url.URL, _ *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	return &api.RecordRange[*api.ChainEntryRecord[api.Record]]{}, nil
}

func (s *fakeSource) QueryMessage(_ context.Context, _ *url.TxID, _ *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	return nil, nil
}

func (s *fakeSource) QueryBptPage(_ context.Context, _ *url.URL, query *api.BptPageQuery) (*api.BptPageRecord, error) {
	count := int(query.Count)
	if count <= 0 {
		count = 256
	}
	startKey := query.StartHash
	if startKey == ([32]byte{}) {
		startKey = bptproof.FullScanStart()
	}

	roBatch := s.db.Begin(false)
	defer roBatch.Discard()
	page, err := bptproof.GetPage(roBatch, startKey, count)
	if err != nil {
		return nil, err
	}
	out := &api.BptPageRecord{
		NextStart: page.NextStart,
		BptRoot:   page.BptRoot,
		Done:      page.Done,
		Entries:   make([]*api.BptLeafSummary, len(page.Entries)),
	}
	for i, e := range page.Entries {
		out.Entries[i] = &api.BptLeafSummary{
			KeyHash:   e.KeyHash,
			ValueHash: e.ValueHash,
		}
	}
	return out, nil
}

func (s *fakeSource) LatestAnchor(_ context.Context, _ string) (uint64, [32]byte, error) {
	return s.block, s.anchor, nil
}

// seedAccounts puts a few simple data accounts into db so the BPT
// has real leaves to enumerate. Uses db.Update (rather than a manual
// Begin/Commit) so UpdateBPT runs and the observer's per-account
// hash actually lands as a BPT leaf.
func seedAccounts(t *testing.T, db *database.Database, urls []*url.URL) {
	t.Helper()
	err := db.Update(func(b *database.Batch) error {
		for _, u := range urls {
			if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// seedSpine populates the spine accounts for partitionURL with empty
// placeholder records so orchestrator.Run's mandatory spine pull
// (#3997) succeeds. Test sources don't actually need spine state to
// exist — they just need the accounts to be findable.
func seedSpine(t *testing.T, db *database.Database, partitionURL *url.URL) {
	t.Helper()
	urls := []*url.URL{
		partitionURL.JoinPath(protocol.AnchorPool),
		partitionURL.JoinPath(protocol.Ledger),
		partitionURL.JoinPath(protocol.Operators),
		partitionURL.JoinPath(protocol.Operators, "1"),
	}
	err := db.Update(func(b *database.Batch) error {
		for _, u := range urls {
			// Use simple placeholders so the spine pull's
			// QueryAccount succeeds. Real spine state is
			// partition-dependent and out of scope for the
			// orchestrator-only tests.
			if err := b.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// currentRoot reads db's current BPT root.
func currentRoot(t *testing.T, db *database.Database) [32]byte {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	r, err := b.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestRun_BVN_PromotesAfterEnumerate is the BVN happy path: spine
// pull + enumerate brings dst's BPT to match src's, AnchorSource
// returns that root, orchestrator promotes machine to ACTIVE.
func TestRun_BVN_PromotesAfterEnumerate(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())

	src := newObservedDB(t)
	seedSpine(t, src, scope)
	urls := []*url.URL{
		protocol.DnUrl().JoinPath("alpha"),
		protocol.DnUrl().JoinPath("beta"),
		protocol.DnUrl().JoinPath("gamma"),
	}
	seedAccounts(t, src, urls)
	root := currentRoot(t, src)

	// Sanity: the source BPT must actually be populated for this test
	// to be meaningful. seedAccounts goes through Account.Main().Put,
	// which only updates the BPT if the observer is properly wired.
	{
		ro := src.Begin(false)
		page, err := bptproof.GetPage(ro, bptproof.FullScanStart(), 256)
		ro.Discard()
		if err != nil {
			t.Fatalf("source BPT page query: %v", err)
		}
		if len(page.Entries) == 0 {
			t.Fatalf("source BPT empty after seedAccounts (root=%x) — observer not wiring writes into BPT", root)
		}
	}

	dst := newObservedDB(t)
	machine := nodestate.New()

	fs := &fakeSource{db: src, anchor: root, block: 99}
	ch := make(chan api.Event)
	close(ch) // no live events; promotion comes from initial anchor poll

	var phases []string
	err := Run(context.Background(), fs, ch, dst, machine, Options{
		Partition:          "Apollo",
		PartitionURL:       scope,
		IsDirectory:        false,
		PageSize:           2,
		AnchorPollInterval: 10 * time.Millisecond,
		MatchThreshold:     1,
		OnPhase: func(p, m string) {
			phases = append(phases, p+":"+m)
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	ad := machine.Get()
	if ad.State != nodestate.StateActive {
		t.Errorf("state=%v phases=%v, want ACTIVE", ad.State, phases)
	}
	if ad.SinceBlock != 99 {
		t.Errorf("sinceBlock=%d, want 99", ad.SinceBlock)
	}
	if ad.VerifiedAnchor != root {
		t.Errorf("anchor mismatch")
	}
}

// TestRun_AnchorMovesAfterEnumerate — the source advances after the
// initial anchor poll. The orchestrator's ticker observes the new
// anchor; the next event ingestion (or simply re-checking) eventually
// matches, but the dst BPT has to chase. Here we just verify the
// orchestrator stays running and promotes once the anchor settles
// back to a value the dst already has.
func TestRun_AnchorReturnsZeroThenMatches(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())
	src := newObservedDB(t)
	seedSpine(t, src, scope)
	urls := []*url.URL{
		protocol.DnUrl().JoinPath("a"),
		protocol.DnUrl().JoinPath("b"),
	}
	seedAccounts(t, src, urls)
	root := currentRoot(t, src)

	dst := newObservedDB(t)
	machine := nodestate.New()

	// Start with zero anchor (source not ready). Flip after 30ms.
	fs := &fakeSource{db: src, anchor: [32]byte{}, block: 0}
	go func() {
		time.Sleep(30 * time.Millisecond)
		fs.block = 7
		fs.anchor = root
	}()

	ch := make(chan api.Event)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Run(ctx, fs, ch, dst, machine, Options{
		Partition:          "Yutu",
		PartitionURL:       scope,
		PageSize:           4,
		AnchorPollInterval: 10 * time.Millisecond,
		MatchThreshold:     1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if machine.State() != nodestate.StateActive {
		t.Errorf("state=%v, want ACTIVE", machine.State())
	}
}

// TestRun_AppliesEventsBeforePromotion — events change dst state
// while the orchestrator is in steady-state. We seed one account on
// src+dst (so initial enumerate captures it), then add a second
// account on src and feed a BlockEvent; orchestrator pulls it; once
// the anchor reflects the post-event root, ACTIVE flips.
func TestRun_AppliesEventsBeforePromotion(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())
	src := newObservedDB(t)
	seedSpine(t, src, scope)
	uA := protocol.DnUrl().JoinPath("a")
	seedAccounts(t, src, []*url.URL{uA})

	dst := newObservedDB(t)
	machine := nodestate.New()

	fs := &fakeSource{db: src} // no anchor yet

	ch := make(chan api.Event, 4)

	// Pre-queue one BlockEvent that adds account uB.
	uB := protocol.DnUrl().JoinPath("b")
	go func() {
		// Wait for the enumeration phase to commit, then mutate src
		// and feed an event. 50ms is generous on these tiny DBs.
		time.Sleep(50 * time.Millisecond)
		seedAccounts(t, src, []*url.URL{uB})
		fs.anchor = currentRoot(t, src)
		fs.block = 17
		ch <- &api.BlockEvent{
			Partition: "Apollo",
			Index:     17,
			Entries: []*api.ChainEntryRecord[api.Record]{
				{Account: uB, Name: "main", Index: 0},
			},
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Run(ctx, fs, ch, dst, machine, Options{
		Partition:          "Apollo",
		PartitionURL:       scope,
		PageSize:           4,
		AnchorPollInterval: 10 * time.Millisecond,
		MatchThreshold:     1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if machine.State() != nodestate.StateActive {
		t.Errorf("state=%v, want ACTIVE", machine.State())
	}

	// uB was named in a BlockEvent — its full account state must be
	// on dst with a matching observer-computed hash. uA was only ever
	// captured as a BPT leaf via enumeration; its account state was
	// never pulled (correct lazy-fetch bootstrap behavior), so we
	// don't assert observer-hash equality for it. The BPT-root match
	// (proven by the ACTIVE flip above) covers global consistency.
	srcRO := src.Begin(false)
	defer srcRO.Discard()
	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	want, err := srcRO.Account(uB).Hash()
	if err != nil {
		t.Fatal(err)
	}
	got, err := dstRO.Account(uB).Hash()
	if err != nil {
		t.Fatalf("dst %s: %v", uB, err)
	}
	if got != want {
		t.Errorf("hash mismatch for %s (event-touched account)", uB)
	}
}

// TestRun_ContextCancel returns nil on cancel.
func TestRun_ContextCancel(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())
	src := newObservedDB(t)
	seedSpine(t, src, scope)
	dst := newObservedDB(t)
	fs := &fakeSource{db: src} // never returns matching anchor
	ch := make(chan api.Event)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Run(ctx, fs, ch, dst, nodestate.New(), Options{
		Partition:          "Apollo",
		PartitionURL:       scope,
		AnchorPollInterval: 10 * time.Millisecond,
		MatchThreshold:     1,
	})
	if err != nil {
		t.Errorf("expected nil on context timeout, got %v", err)
	}
}

// TestRun_CyclopsRepairExceptionPreservesEnumerateLeaf models the
// 22-account drift on Cyclops mainnet (issue #4020): the source's BPT
// holds a stored leaf hash that diverges from what account.Hash() would
// recompute over the source's stored state. The bootstrap launcher
// must NOT pull and recompute for these accounts — doing so would
// produce a local leaf hash that diverges from the source's BPT root,
// preventing the tracker from ever matching a signed anchor. Instead,
// the orchestrator's hydrate loop skips the cyclopsrepair-listed URLs,
// preserving the enumerate-recorded leaf hash (= source's stored leaf).
//
// The test seeds source with a normal account plus one Cyclops-listed
// account, then directly overwrites the listed account's BPT entry
// with a deliberately-wrong leaf to mimic the mainnet condition (BPT
// stored hash != recomputed account.Hash()). It runs Run with a
// MatchThreshold of 1 and confirms (a) the orchestrator promotes
// because the local root matches the planted source root, and (b) the
// dst BPT's leaf for the excepted account is the planted fake — i.e.
// hydrate did not run.
func TestRun_CyclopsRepairExceptionPreservesEnumerateLeaf(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())

	src := newObservedDB(t)
	seedSpine(t, src, scope)

	// Pick the first listed account on the Cyclops partition. The
	// exception logic is partition-scoped, so the test's Run call
	// below uses Partition: cyclopsrepair.Partition.
	targets := cyclopsrepair.Targets(cyclopsrepair.Partition)
	if len(targets) == 0 {
		t.Fatal("cyclopsrepair.Targets() unexpectedly empty for Cyclops")
	}
	excepted := targets[0]

	// One regular account so the BPT has a leaf the launcher can
	// hydrate normally — gives us a positive control alongside the
	// excepted leaf.
	regular, _ := url.Parse(protocol.DnUrl().String() + "/regular")
	seedAccounts(t, src, []*url.URL{regular})

	// Plant a fake BPT entry directly for the excepted account. We do
	// NOT seedAccounts(excepted) because some excepted URLs (e.g. ADIs)
	// fail commit-path validation when written as a generic
	// DataAccount. Inserting a BPT leaf without ever creating Main is
	// the closer mirror of the mainnet condition anyway: the leaf is
	// stale or stripped while the account body may be intact or absent.
	// db.Update calls UpdateBPT after fn, but UpdateBPT only walks
	// dirty accounts; the manual BPT().Insert isn't dirtied, so the
	// planted leaf survives.
	var fake [32]byte
	for i := range fake {
		fake[i] = 0xC0
	}
	if err := src.Update(func(b *database.Batch) error {
		return b.BPT().Insert(record.NewKey("Account", excepted), fake[:])
	}); err != nil {
		t.Fatalf("plant fake leaf in src: %v", err)
	}

	// Sanity check: src.BPT() now holds the fake for the excepted
	// account.
	{
		ro := src.Begin(false)
		stored, err := ro.BPT().Get(record.NewKey("Account", excepted))
		ro.Discard()
		if err != nil {
			t.Fatalf("src BPT lookup: %v", err)
		}
		if [32]byte(stored) != fake {
			t.Fatalf("planted leaf did not stick: stored=%x want=%x", stored, fake)
		}
	}

	srcRoot := currentRoot(t, src)

	dst := newObservedDB(t)
	machine := nodestate.New()

	fs := &fakeSource{db: src, anchor: srcRoot, block: 99}
	ch := make(chan api.Event)
	close(ch)

	var phases []string
	err := Run(context.Background(), fs, ch, dst, machine, Options{
		Partition:          cyclopsrepair.Partition,
		PartitionURL:       scope,
		IsDirectory:        false,
		PageSize:           4,
		AnchorPollInterval: 10 * time.Millisecond,
		MatchThreshold:     1,
		OnPhase: func(p, m string) {
			phases = append(phases, p+":"+m)
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	ad := machine.Get()
	if ad.State != nodestate.StateActive {
		t.Errorf("state=%v phases=%v, want ACTIVE — exception path may be missing",
			ad.State, phases)
	}
	if ad.VerifiedAnchor != srcRoot {
		t.Errorf("verified anchor mismatch: got %x want %x", ad.VerifiedAnchor, srcRoot)
	}

	// dst's BPT leaf for the excepted account must equal the planted
	// fake — hydrate must have been skipped.
	roDst := dst.Begin(false)
	defer roDst.Discard()
	leaf, err := roDst.BPT().Get(record.NewKey("Account", excepted))
	if err != nil {
		t.Fatalf("dst BPT lookup: %v", err)
	}
	if [32]byte(leaf) != fake {
		t.Errorf("dst BPT leaf for excepted account = %x, want fake %x — hydrate must have run despite the exception",
			leaf, fake)
	}
}

// TestRun_CyclopsRepairExceptionScopedToCyclopsPartition guards against
// the case where a non-Cyclops partition (sim, devnet, other BVN)
// happens to use one of the listed URLs. On such partitions the
// exception path must NOT fire — the URL match alone is insufficient.
// This test plants a fake leaf for a listed URL on a non-Cyclops
// partition and verifies that the orchestrator attempts the regular
// hydrate path (which here returns NotFound from the source, exercising
// the existing notInSource path rather than the cyclops-excepted one).
func TestRun_CyclopsRepairExceptionScopedToCyclopsPartition(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())

	src := newObservedDB(t)
	seedSpine(t, src, scope)

	targets := cyclopsrepair.Targets(cyclopsrepair.Partition)
	if len(targets) == 0 {
		t.Fatal("cyclopsrepair.Targets() unexpectedly empty for Cyclops")
	}
	listedURL := targets[0]

	regular, _ := url.Parse(protocol.DnUrl().String() + "/regular")
	seedAccounts(t, src, []*url.URL{regular})

	var fake [32]byte
	for i := range fake {
		fake[i] = 0xC0
	}
	if err := src.Update(func(b *database.Batch) error {
		return b.BPT().Insert(record.NewKey("Account", listedURL), fake[:])
	}); err != nil {
		t.Fatalf("plant fake leaf in src: %v", err)
	}
	srcRoot := currentRoot(t, src)

	dst := newObservedDB(t)
	machine := nodestate.New()

	fs := &fakeSource{db: src, anchor: srcRoot, block: 99}
	ch := make(chan api.Event)
	close(ch)

	var phases []string
	err := Run(context.Background(), fs, ch, dst, machine, Options{
		Partition:          "Apollo", // NOT Cyclops — exception must not fire
		PartitionURL:       scope,
		IsDirectory:        false,
		PageSize:           4,
		AnchorPollInterval: 10 * time.Millisecond,
		MatchThreshold:     1,
		OnPhase: func(p, m string) {
			phases = append(phases, p+":"+m)
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The orchestrator should still promote: the hydrate path on a
	// non-Cyclops partition routes the listedURL through the regular
	// path. Because src has no Main for it, the existing notInSource
	// path catches the NotFound and preserves the enumerate-recorded
	// leaf. The local root still matches src's anchor, so promotion
	// occurs. What we are guarding here is that a phase log line for
	// the cyclops-excepted path did NOT appear — that path must only
	// fire on the Cyclops partition.
	ad := machine.Get()
	if ad.State != nodestate.StateActive {
		t.Errorf("state=%v phases=%v, want ACTIVE", ad.State, phases)
	}
	for _, p := range phases {
		if containsCyclopsExceptionLog(p) {
			t.Errorf("non-Cyclops partition saw cyclops-excepted phase log: %q (phases=%v)", p, phases)
		}
	}
}

// containsCyclopsExceptionLog reports whether a phase line is the
// cyclops-excepted hydrate-skip log emitted by runEnumerate.
func containsCyclopsExceptionLog(line string) bool {
	const marker = "cyclops-repair exception:"
	for i := 0; i+len(marker) <= len(line); i++ {
		if line[i:i+len(marker)] == marker {
			return true
		}
	}
	return false
}

// TestRun_RejectsMissingInputs — guards.
func TestRun_RejectsMissingInputs(t *testing.T) {
	src := newObservedDB(t)
	dst := newObservedDB(t)
	fs := &fakeSource{db: src}
	ch := make(chan api.Event)
	scope, _ := url.Parse(protocol.DnUrl().String())
	machine := nodestate.New()

	cases := []struct {
		name string
		src  Source
		db   *database.Database
		m    *nodestate.Machine
		opts Options
	}{
		{"no src", nil, dst, machine, Options{Partition: "p", PartitionURL: scope}},
		{"no db", fs, nil, machine, Options{Partition: "p", PartitionURL: scope}},
		{"no machine", fs, dst, nil, Options{Partition: "p", PartitionURL: scope}},
		{"no partition url", fs, dst, machine, Options{Partition: "p"}},
		{"no partition", fs, dst, machine, Options{PartitionURL: scope}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Run(context.Background(), c.src, ch, c.db, c.m, c.opts)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
