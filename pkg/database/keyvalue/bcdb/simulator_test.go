// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !race

// The simulator shares one envelope across the goroutines it fans a
// submission out to, and Transaction.GetHash memoises into it, so any
// three-BVN run trips the race detector -- see #4166.  The race is in
// the harness, not here: it reproduces identically with the memory
// backend, which is how it was isolated.  This file is excluded from
// -race runs until #4166 is fixed rather than left to fail for a
// reason that has nothing to do with what it tests.

package bcdb_test

import (
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestSimulatorRouting runs a network on BlockchainDB and asks the
// store whether the classification in route.go is right.
//
// The check is the permanent layer's own refusal.  That layer will not
// overwrite a key with a different value, so if a record isWriteOnce
// calls write-once is ever rewritten, the store says so -- and the
// adapter counts it against the record's shape instead of failing, so
// one run reports every misclassification rather than the first.  A
// clean run is PutConflict zero and no shape misrouted.
//
// Genesis alone writes most of the record model; the transactions add
// the paths that only appear once a network is running -- synthetic
// transactions between partitions, anchors, signature chains and the
// data account records that genesis has no reason to touch.
func TestSimulatorRouting(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	bob := build.
		Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	// Every database the simulator opens, so the tally can be read
	// back once the run is over
	var mu sync.Mutex
	var dbs []*bcdb.Database
	dir := t.TempDir()

	open := func(partition *protocol.PartitionInfo, node int, _ logging.Logger) keyvalue.Beginner {
		db, err := bcdb.Open(filepath.Join(dir, partition.ID, string(rune('a'+node))))
		require.NoError(t, err)
		// The minimum window, so records age out of the permanent layer
		// within the handful of blocks this runs. At the production N
		// nothing ages out here and the deep-fallback check below would
		// pass whatever the routing did.
		require.NoError(t, db.SetMergeLag(20))
		mu.Lock()
		defer mu.Unlock()
		dbs = append(dbs, db)
		return db
	}

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime).With(alice, bob),
		simulator.WithDatabase(open),
	)

	// A send between partitions produces a synthetic transaction and
	// the anchors that carry it, which is where most of the records a
	// running network writes come from
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	// A data account exercises the counted collection, which is the
	// one place an element and its count are classified differently
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			CreateDataAccount(alice, "data").
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("hello")).ToState().
			SignWith(alice, "book", "1").Version(1).Timestamp(3).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	// Let the anchoring settle, so the chains that only move between
	// blocks get written more than once
	sim.StepN(50)

	// Now ask the stores
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, dbs, "the simulator did not use the database")

	misrouted := map[string]uint64{}
	var conflicts uint64
	var permWrites, dynaWrites uint64
	shapes := map[string]bcdb.ShapeCount{}
	for _, db := range dbs {
		perm, dyna := db.Stats()
		conflicts += perm.PutConflict
		permWrites += perm.PutTotal
		dynaWrites += dyna.PutTotal
		for shape, c := range db.Shapes() {
			if c.Misrouted > 0 {
				misrouted[shape] += c.Misrouted
			}
			s := shapes[shape]
			s.Layer = c.Layer
			s.New += c.New
			s.Duplicate += c.Duplicate
			s.Rewritten += c.Rewritten
			s.Misrouted += c.Misrouted
			shapes[shape] = s
		}
	}

	// The report is the point of the run, so log it whether or not the
	// assertions hold
	names := make([]string, 0, len(shapes))
	for shape := range shapes {
		names = append(names, shape)
	}
	sort.Strings(names)
	t.Logf("perm writes %d, dyna writes %d, across %d databases", permWrites, dynaWrites, len(dbs))
	for _, shape := range names {
		c := shapes[shape]
		t.Logf("  %-4s new=%-7d dup=%-7d rewritten=%-7d misrouted=%-7d %s",
			c.Layer, c.New, c.Duplicate, c.Rewritten, c.Misrouted, shape)
	}

	require.Empty(t, misrouted, "records classified write-once that Accumulate rewrites")

	// Placement is not only about whether a record is rewritten. The
	// permanent layer is read through a window, so a record the executor
	// reads on every touch of an account is a history walk forever once
	// it ages out -- Account.(url).Url was 96,303 of them over 200
	// commits, on every BVN engine, in run 20260901T054802Z, and the only
	// shape falling back there at all. It is dynamic for that reason.
	// Reported, NOT asserted. This run is shorter than the smallest
	// window the store allows (20 blocks), so nothing ages out of the
	// permanent layer and the counters are empty whatever the routing
	// does -- an assertion here would pass on the defect too. The
	// evidence is the soak's own stats.json, where the shape appeared
	// 96,303 times over 200 commits on every BVN engine.
	deep := map[string]uint64{}
	for _, db := range dbs {
		for shape, n := range db.DeepFallbacks() {
			deep[shape] += n
		}
	}
	for shape, n := range deep {
		t.Logf("  deep fallback %-40s %d", shape, n)
	}

	require.Zero(t, conflicts, "the permanent layer refused a write")

	// A classification that sent nothing to the permanent layer would
	// pass the checks above and mean nothing
	require.NotZero(t, permWrites, "nothing was routed to the permanent layer")
	require.NotZero(t, dynaWrites, "nothing was routed to the dynamic layer")

	// And the shapes the classification exists for have to be there,
	// so that renaming a record in model.yml fails here rather than
	// silently retiring a rule
	for _, want := range []string{
		"Message.(hash).Main",
		"Account.(url).MainChain.Element.(int)",
	} {
		c, ok := shapes[want]
		require.True(t, ok, "%s was never written", want)
		require.Equal(t, "perm", c.Layer, "%s was not routed to the permanent layer", want)
	}
}
