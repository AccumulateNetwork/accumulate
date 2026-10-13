// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/anchoring/proofs"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
	write := func(u *url.URL, entry *DoubleHashDataEntry) *TransactionStatus {
		timestamp++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(u).
				Body(&WriteData{Entry: entry}).
				SignWith(lite).Version(1).Timestamp(timestamp).PrivateKey(liteKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
		return st
	}
	entry := func(s string) *DoubleHashDataEntry { return &DoubleHashDataEntry{Data: [][]byte{[]byte(s)}} }
	written := []*TransactionStatus{write(a, aFirst), write(b, bFirst)}
	for i := 0; i < 2; i++ {
		written = append(written, write(a, entry(fmt.Sprintf("a-%d", i))), write(b, entry(fmt.Sprintf("b-%d", i))))
	}
	write(other, otherFirst)

	builders := map[string]*proofs.Builder{}
	for _, p := range []string{"BVN0", "BVN1"} {
		builders[p] = &proofs.Builder{
			Network:   t.Name(),
			Partition: p,
			Database:  sim.Database(p),
			Directory: func() (database.Viewer, error) { return sim.Database(Directory), nil },
			Accounts:  []*url.URL{a, b},
			Store:     memory.New(nil),
		}
	}

	// Run until every entry is proven: the Directory commits a partition's
	// anchor a few blocks after the block that produced it.
	proven := 0
	for i := 0; i < 50 && proven < len(written); i++ {
		sim.Step()
		for _, bld := range builders {
			n, err := bld.Run()
			require.NoError(t, err)
			proven += n
		}
	}
	// Every chain of a configured account is proven, the signature chain's
	// entries with the main chain's
	require.GreaterOrEqual(t, proven, len(written), "every entry of the configured accounts is proven")

	for _, st := range written {
		part, err := sim.Router().RouteAccount(st.TxID.Account())
		require.NoError(t, err)
		pkg, err := builders[part].ByHash(st.TxID.HashSlice())
		require.NoError(t, err, "proof for %v", st.TxID)
		require.Equal(t, "main", pkg.Entry.Chain)
		require.True(t, pkg.Receipt.Validate(nil), "the receipt validates")
		require.Equal(t, pkg.Receipt.Anchor, pkg.Directory.Root)

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
				require.Equal(t, r.Anchor, pkg.Directory.Root, "root at Directory block %d", pkg.Directory.Height)
			}
			require.True(t, found, "Directory block %d has a root index entry", pkg.Directory.Height)
		})
	}

	// The unconfigured account has no proof, and the second run proves nothing new
	_, err := builders["BVN0"].Get(other, "main", 0)
	require.ErrorIs(t, err, errors.NotFound)
	n, err := builders["BVN0"].Run()
	require.NoError(t, err)
	require.Zero(t, n)
}
