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
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/anchoring/proofs"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/anchoring"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
		require.Equal(t, st.TxID.HashSlice(), []byte(pkg.Entry.Hash), "the entry is the transaction")
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
