// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/anchoring/proofs"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/anchoring"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// liteDataOn returns a lite data account routed to the given partition, and
// the first entry that names it.
func liteDataOn(t *testing.T, sim *Sim, name, partition string) (*url.URL, *DoubleHashDataEntry) {
	for i := 0; ; i++ {
		// The account ID is the hash of the first entry's ExtIDs, Data[1:]
		first := &DoubleHashDataEntry{Data: [][]byte{[]byte("first"), []byte(fmt.Sprintf("%s-%s-%d", t.Name(), name, i))}}
		u, err := LiteDataAddress(ComputeLiteDataAccountId(first))
		require.NoError(t, err)
		p, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if p == partition {
			return u, first
		}
	}
}

// #4563 (anchoring.md §2.5): the builder reads the block ledgers of each
// partition, and for every entry of a configured account keeps a receipt that
// verifies to the Directory root of the block that committed the partition's
// anchor. An account not configured yields nothing.
func TestAnchoringProofBuilder(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 3, 1), simulator.Genesis(GenesisTime))
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	sim.StepN(10)

	// Two data accounts on two partitions, and one that is not configured
	a, aFirst := liteDataOn(t, sim, "a", "BVN0")
	b, bFirst := liteDataOn(t, sim, "b", "BVN1")
	other, otherFirst := liteDataOn(t, sim, "other", "BVN0")
	var timestamp uint64
	submit := func(u *url.URL, entry *DoubleHashDataEntry) *TransactionStatus {
		timestamp++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(u).
				Body(&WriteData{Entry: entry}).
				SignWith(lite).Version(1).Timestamp(timestamp).PrivateKey(liteKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
		return st
	}
	entry := func(s string) *DoubleHashDataEntry { return &DoubleHashDataEntry{Data: [][]byte{[]byte(s)}} }
	stores := map[string]keyvalue.Beginner{}
	builders := map[string]*proofs.Builder{}
	for _, p := range []string{"BVN0", "BVN1"} {
		stores[p] = memory.New(nil)
		builders[p] = &proofs.Builder{
			Network:   t.Name(),
			Partition: p,
			Database:  sim.Database(p),
			Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
			Accounts:  []*url.URL{a, b},
			Store:     stores[p],
		}
	}

	// Every Run is counted for searches (database.md, "Proofs are read, not
	// searched"), and the store is looked at after each for entries waiting on
	// the Directory
	var searches uint64
	var sawPending bool
	run := func() {
		for p, bld := range builders {
			before := indexing.IndexChainSearches()
			_, err := bld.Run()
			require.NoError(t, err)
			searches += indexing.IndexChainSearches() - before
			if len(storeKeys(t, stores[p], "Pending")) > 0 {
				sawPending = true
			}
		}
	}

	// Run right after each write: the Directory has not committed that
	// partition's anchor yet, so the entry waits
	written := []*TransactionStatus{}
	write := func(u *url.URL, entry *DoubleHashDataEntry) {
		written = append(written, submit(u, entry))
		run()
	}
	write(a, aFirst)
	write(b, bFirst)
	for i := 0; i < 2; i++ {
		write(a, entry(fmt.Sprintf("a-%d", i)))
		write(b, entry(fmt.Sprintf("b-%d", i)))
	}
	n := len(written)
	require.True(t, sawPending, "an entry waited for the Directory")

	// Run until every main chain entry is proven and nothing waits: the
	// Directory commits a partition's anchor a few blocks after the block that
	// produced it. Equal, not at least, so an early exit cannot hide a
	// builder that proves too much or too little.
	submit(other, otherFirst)
	mainProofs := func() (n int) {
		for _, st := range stores {
			for _, k := range storeKeys(t, st, "Proof") {
				if k.Get(2) == "main" {
					n++
				}
			}
		}
		return
	}
	pendingKeys := func() (n int) {
		for _, st := range stores {
			n += len(storeKeys(t, st, "Pending"))
		}
		return
	}
	for i := 0; i < 100 && (pendingKeys() > 0 || mainProofs() < n); i++ {
		sim.Step()
		run()
	}
	require.Zero(t, pendingKeys(), "nothing waits")
	require.Equal(t, n, mainProofs(), "every main entry of the configured accounts is proven, and no more")
	require.Zero(t, searches, "the builder never searches an index chain")

	for _, st := range written {
		part, err := sim.Router().RouteAccount(st.TxID.Account())
		require.NoError(t, err)
		pkg, err := builders[part].ByHash(st.TxID.HashSlice())
		require.NoError(t, err, "proof for %v", st.TxID)
		require.Equal(t, "main", pkg.Entry.Chain)
		checkPackage(t, sim, st.TxID, pkg)
	}

	// Every entry of every chain of a configured account is proven, not only
	// the entries the block ledgers name (signature chains included)
	for _, acct := range []*url.URL{a, b} {
		part, err := sim.Router().RouteAccount(acct)
		require.NoError(t, err)
		View(t, sim.Database(part), func(batch *database.Batch) {
			chains, err := batch.Account(acct).Chains().Get()
			require.NoError(t, err)
			for _, md := range chains {
				if md.Type == merkle.ChainTypeIndex {
					continue
				}
				c, err := batch.Account(acct).ChainByName(md.Name)
				require.NoError(t, err)
				ch, err := c.Get()
				require.NoError(t, err)
				have := map[any]bool{}
				for _, k := range storeKeys(t, stores[part], "Proof") {
					if k.Get(1) == acct.String() && k.Get(2) == md.Name {
						have[k.Get(3)] = true
					}
				}
				for i := int64(0); i < ch.Height(); i++ {
					require.True(t, have[uint64(i)], "%v %s entry %d is proven", acct, md.Name, i)
				}
			}
		})
	}

	// The unconfigured account has neither a proof nor a waiting entry
	for _, st := range stores {
		for _, k := range append(storeKeys(t, st, "Proof"), storeKeys(t, st, "Pending")...) {
			require.NotEqual(t, other.String(), k.Get(1), "%v", k)
		}
	}
	_, err := builders["BVN0"].Get(other, "main", 0)
	require.ErrorIs(t, err, errors.NotFound)
	m, err := builders["BVN0"].Run()
	require.NoError(t, err)
	require.Zero(t, m)
}

// checkPackage checks a package for a transaction the way a verifier of the
// receipt would: it starts at the entry, validates, ends at the Directory
// root of the block it names, and encodes its bytes as hex.
func checkPackage(t *testing.T, sim *Sim, txid *url.TxID, pkg *anchoring.Package) {
	t.Helper()
	require.Equal(t, "main", pkg.Entry.Chain)
	require.Equal(t, txid.HashSlice(), []byte(pkg.Entry.Hash), "the entry is the transaction")
	require.Equal(t, []byte(pkg.Entry.Hash), pkg.Receipt.Start, "the receipt starts at the entry")

	// Bytes are hex in the package (anchoring.md §2.5)
	raw, err := json.Marshal(pkg)
	require.NoError(t, err)
	var generic struct {
		Entry     struct{ Hash string }
		Directory struct{ Root string }
	}
	require.NoError(t, json.Unmarshal(raw, &generic))
	require.Equal(t, hex.EncodeToString(pkg.Entry.Hash), generic.Entry.Hash)
	require.Equal(t, hex.EncodeToString(pkg.Directory.Root), generic.Directory.Root)
	var back anchoring.Package
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, pkg.Entry, back.Entry)
	require.Equal(t, pkg.Directory, back.Directory)
	require.True(t, pkg.Receipt.Validate(nil), "the receipt validates")
	require.Equal(t, pkg.Receipt.Anchor, []byte(pkg.Directory.Root))

	// The root is the Directory's root chain anchor at that height
	View(t, sim.Database(Directory), func(batch *database.Batch) {
		root := batch.Account(DnUrl().JoinPath(Ledger)).RootChain()
		idx, err := root.Index().Get()
		require.NoError(t, err)
		found := false
		for i := int64(0); i < idx.Height(); i++ {
			raw, err := idx.Entry(i)
			require.NoError(t, err)
			var e IndexEntry
			require.NoError(t, e.UnmarshalBinary(raw))
			if e.BlockIndex != pkg.Directory.Height {
				continue
			}
			found = true
			r, err := root.Receipt(e.Source, e.Source)
			require.NoError(t, err)
			require.Equal(t, r.Anchor, []byte(pkg.Directory.Root), "root at Directory block %d", pkg.Directory.Height)
		}
		require.True(t, found, "Directory block %d has a root index entry", pkg.Directory.Height)
	})
}

// storeKeys lists the keys of a builder's store that start with the prefix.
func storeKeys(t *testing.T, store keyvalue.Beginner, prefix string) []*record.Key {
	st := store.Begin(nil, false)
	defer st.Discard()
	var keys []*record.Key
	require.NoError(t, st.ForEach(func(k *record.Key, _ []byte) error {
		if k.Len() > 0 && k.Get(0) == prefix {
			keys = append(keys, k)
		}
		return nil
	}))
	return keys
}

// #4563: a block ledger also names an account a block changed without a chain
// append (a key book whose page a signature touched, #4437). That entry has
// nothing to prove; it must not stop the builder.
func TestAnchoringProofBuilderChainlessEntry(t *testing.T) {
	alice := url.MustParse("alice.acme")
	key1 := acctesting.GenerateKey(t.Name(), 1)
	key2 := acctesting.GenerateKey(t.Name(), 2)
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 1, 1), simulator.Genesis(GenesisTime))
	MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:], key2[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
		p.AcceptThreshold = 2
	})
	data, book := alice.JoinPath("data"), alice.JoinPath("book")
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: data})
	sim.StepN(10)

	part, err := sim.Router().RouteAccount(alice)
	require.NoError(t, err)
	bld := &proofs.Builder{
		Network:   t.Name(),
		Partition: part,
		Database:  sim.Database(part),
		Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
		Accounts:  []*url.URL{data, book},
		Store:     memory.New(nil),
	}

	// The first signature leaves the transaction pending, the second completes it
	var timestamp uint64
	env := MustBuild(t,
		build.Transaction().For(data).
			Body(&WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("hello")}}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(key1))
	st := sim.SubmitTxnSuccessfully(env)
	sim.StepUntil(Txn(st.TxID).IsPending())
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(key2))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// The case is real: some block ledger names the book with no chain
	var chainless int
	View(t, sim.Database(part), func(batch *database.Batch) {
		ledger := batch.Account(PartitionUrl(part).JoinPath(Ledger))
		var sys *SystemLedger
		require.NoError(t, ledger.Main().GetAs(&sys))
		for i := uint64(1); i <= sys.Index; i++ {
			bl, err := ledger.BlockLedger(i).Get()
			if errors.Is(err, errors.NotFound) {
				continue
			}
			require.NoError(t, err)
			for _, e := range bl.Entries {
				if e.Account.Equal(book) && e.Chain == "" {
					chainless++
				}
			}
		}
	})
	require.NotZero(t, chainless, "a block ledger names the key book without a chain")

	for i := 0; i < 100; i++ {
		_, err := bld.Run()
		require.NoError(t, err)
		if _, err := bld.ByHash(st.TxID.HashSlice()); err == nil {
			return
		}
		sim.Step()
	}
	t.Fatal("the data account's entry was never proven")
}

// proveAll runs the builder, stepping, until nothing is pending and every
// transaction has a package, failing on any error or search.
func proveAll(t *testing.T, sim *Sim, bld *proofs.Builder, store keyvalue.Beginner, txns ...*TransactionStatus) {
	t.Helper()
	before := indexing.IndexChainSearches()
	have := func() bool {
		for _, st := range txns {
			if _, err := bld.ByHash(st.TxID.HashSlice()); err != nil {
				return false
			}
		}
		return true
	}
	for i := 0; i < 100 && (!have() || len(storeKeys(t, store, "Pending")) > 0); i++ {
		_, err := bld.Run()
		require.NoError(t, err)
		sim.Step()
	}
	_, err := bld.Run()
	require.NoError(t, err)
	require.Zero(t, len(storeKeys(t, store, "Pending")), "nothing waits")
	require.Zero(t, indexing.IndexChainSearches()-before, "the builder never searches an index chain")
	for _, st := range txns {
		pkg, err := bld.ByHash(st.TxID.HashSlice())
		require.NoError(t, err, "proof for %v", st.TxID)
		checkPackage(t, sim, st.TxID, pkg)
	}
}

// #4563: a block appends every entry submitted to a chain in it, but its block
// ledger names only the last. The earlier entries get a proof as well.
func TestAnchoringProofBuilderSameBlock(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 1, 1), simulator.Genesis(GenesisTime))
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	sim.StepN(10)
	part, err := sim.Router().RouteAccount(lite)
	require.NoError(t, err)
	a, first := liteDataOn(t, sim, "a", part)

	var ts uint64
	submit := func(e *DoubleHashDataEntry) *TransactionStatus {
		ts++
		return sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(a).Body(&WriteData{Entry: e}).
			SignWith(lite).Version(1).Timestamp(ts).PrivateKey(liteKey))
	}
	s0 := submit(first)
	sim.StepUntil(Txn(s0.TxID).Succeeds())

	// Three more before any step: they share a block
	var rest []*TransactionStatus
	for i := 1; i <= 3; i++ {
		rest = append(rest, submit(&DoubleHashDataEntry{Data: [][]byte{[]byte(fmt.Sprintf("x%d", i))}}))
	}
	sim.StepUntil(Txn(rest[0].TxID).Succeeds(), Txn(rest[1].TxID).Succeeds(), Txn(rest[2].TxID).Succeeds())
	sim.StepN(20)

	// The case is real: some block ledger names an index of the main chain
	// more than one above the last it named
	var jumped bool
	View(t, sim.Database(part), func(batch *database.Batch) {
		ledger := batch.Account(PartitionUrl(part).JoinPath(Ledger))
		var sys *SystemLedger
		require.NoError(t, ledger.Main().GetAs(&sys))
		prev := int64(-1)
		for i := uint64(1); i <= sys.Index; i++ {
			bl, err := ledger.BlockLedger(i).Get()
			if errors.Is(err, errors.NotFound) {
				continue
			}
			require.NoError(t, err)
			for _, e := range bl.Entries {
				if e.Account.Equal(a) && e.Chain == "main" {
					jumped = jumped || int64(e.Index) > prev+1
					prev = int64(e.Index)
				}
			}
		}
	})
	require.True(t, jumped, "a block ledger skips entries of the main chain")

	store := memory.New(nil)
	bld := &proofs.Builder{
		Network:   t.Name(),
		Partition: part,
		Database:  sim.Database(part),
		Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
		Accounts:  []*url.URL{a},
		Store:     store,
	}
	proveAll(t, sim, bld, store, append([]*TransactionStatus{s0}, rest...)...)
}

func isBVN0Anchor(env *messaging.Envelope) bool {
	for _, m := range env.Messages {
		a, ok := m.(*messaging.BlockAnchor)
		if !ok {
			continue
		}
		seq, ok := a.Anchor.(*messaging.SequencedMessage)
		if !ok || !seq.Source.Equal(PartitionUrl("BVN0")) {
			continue
		}
		if txn, ok := seq.Message.(*messaging.TransactionMessage); ok && txn.Transaction.Body.Type() == TransactionTypeBlockValidatorAnchor {
			return true
		}
	}
	return false
}

// #4563: when the Directory receives several anchors from one partition in a
// block, only the last has stored positions; the builder binds the others
// through it.
func TestAnchoringProofBuilderBatchedAnchors(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 1, 1), simulator.Genesis(GenesisTime))
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	sim.StepN(10)
	a, first := liteDataOn(t, sim, "a", "BVN0")

	// The Directory holds BVN0's anchors, then takes them together
	var mu sync.Mutex
	hold := true
	var held []*messaging.Envelope
	sim.SetBlockHook(Directory, func(p execute.BlockParams, envs []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		mu.Lock()
		defer mu.Unlock()
		if hold {
			var keep []*messaging.Envelope
			for _, e := range envs {
				if isBVN0Anchor(e) {
					held = append(held, e)
				} else {
					keep = append(keep, e)
				}
			}
			return keep, true
		}
		out := append(held, envs...)
		held = nil
		return out, true
	})

	var ts uint64
	submit := func(e *DoubleHashDataEntry) *TransactionStatus {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(a).Body(&WriteData{Entry: e}).
			SignWith(lite).Version(1).Timestamp(ts).PrivateKey(liteKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
		sim.StepN(3)
		return st
	}
	s0 := submit(first)
	s1 := submit(&DoubleHashDataEntry{Data: [][]byte{[]byte("x2")}})
	mu.Lock()
	hold = false
	mu.Unlock()
	sim.StepN(20)

	// The case is real: an anchor(bvn0)-root entry has no chains record
	var missing int
	View(t, sim.Database(Directory), func(batch *database.Batch) {
		c := batch.Account(DnUrl().JoinPath(AnchorPool)).AnchorChain("BVN0").Root()
		ch, err := c.Get()
		require.NoError(t, err)
		for i := int64(0); i < ch.Height(); i++ {
			h, err := ch.Entry(i)
			require.NoError(t, err)
			recs, err := batch.Transaction(h).Chains().Get()
			require.NoError(t, err)
			found := false
			for _, r := range recs {
				found = found || r.Chain == c.Name()
			}
			if !found {
				missing++
			}
		}
	})
	require.NotZero(t, missing, "an anchor entry has no stored positions")

	store := memory.New(nil)
	bld := &proofs.Builder{
		Network:   t.Name(),
		Partition: "BVN0",
		Database:  sim.Database("BVN0"),
		Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
		Accounts:  []*url.URL{a},
		Store:     store,
	}
	proveAll(t, sim, bld, store, s0, s1)
}

// #4563: when the Directory takes a large backlog of one partition's anchors
// in a block, every entry is proven, however far it is from the batch's last
// record: the forward read is as long as the batch.
func TestAnchoringProofBuilderBigBatch(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name())
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 1, 1), simulator.Genesis(GenesisTime))
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e12)
	sim.StepN(10)
	a, first := liteDataOn(t, sim, "a", "BVN0")

	// The Directory holds BVN0's anchors while the account is written, one
	// write per block, then takes them all in one block
	var mu sync.Mutex
	hold := true
	var held []*messaging.Envelope
	maxBatch := 0
	sim.SetBlockHook(Directory, func(p execute.BlockParams, envs []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		mu.Lock()
		defer mu.Unlock()
		if hold {
			var keep []*messaging.Envelope
			for _, e := range envs {
				if isBVN0Anchor(e) {
					held = append(held, e)
				} else {
					keep = append(keep, e)
				}
			}
			return keep, true
		}
		maxBatch = max(maxBatch, len(held))
		out := append(held, envs...)
		held = nil
		return out, true
	})

	var ts uint64
	var written []*TransactionStatus
	submit := func(e *DoubleHashDataEntry) {
		ts++
		written = append(written, sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(a).Body(&WriteData{Entry: e}).
			SignWith(lite).Version(1).Timestamp(ts).PrivateKey(liteKey)))
	}
	submit(first)
	sim.StepUntil(Txn(written[0].TxID).Succeeds())
	for i := 0; i < 300; i++ {
		submit(&DoubleHashDataEntry{Data: [][]byte{[]byte(fmt.Sprintf("x%d", i))}})
		sim.Step()
	}
	sim.StepN(3)
	mu.Lock()
	hold = false
	mu.Unlock()
	sim.StepN(20)

	mu.Lock()
	batch := maxBatch
	mu.Unlock()
	t.Logf("the Directory took %d BVN0 anchors in one block", batch)
	require.Greater(t, batch, 256, "the batch is large")

	store := memory.New(nil)
	bld := &proofs.Builder{
		Network:   t.Name(),
		Partition: "BVN0",
		Database:  sim.Database("BVN0"),
		Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
		Accounts:  []*url.URL{a},
		Store:     store,
	}
	before := indexing.IndexChainSearches()
	for i := 0; i < 3; i++ {
		_, err := bld.Run()
		require.NoError(t, err)
		sim.Step()
	}
	require.Zero(t, indexing.IndexChainSearches()-before, "the builder never searches an index chain")
	for i, st := range written {
		_, err := bld.ByHash(st.TxID.HashSlice())
		require.NoError(t, err, "proof for transaction %d", i)
	}
	// The first is the farthest from the batch's last record
	pkg, err := bld.ByHash(written[0].TxID.HashSlice())
	require.NoError(t, err)
	checkPackage(t, sim, written[0].TxID, pkg)
}
