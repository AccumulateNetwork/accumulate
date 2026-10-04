// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	bptkey "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// bentStates is a peer whose account states carry a body other than the one
// its leaf was hashed from: a reply torn, or wrong.
type bentStates struct {
	pull.Source
	name  string
	asked *int
}

func (b bentStates) String() string { return b.name }

func (b bentStates) QueryAccountStates(ctx context.Context, scope *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	*b.asked++
	r, err := b.Source.QueryAccountStates(ctx, scope, q)
	if err != nil {
		return nil, err
	}
	for _, st := range r.States {
		if a, ok := st.Main.(*protocol.TokenAccount); ok {
			a = a.Copy()
			a.Balance = *big.NewInt(12)
			st.Main = a
		}
	}
	return r, nil
}

type named struct {
	pull.Source
	name string
}

func (n named) String() string { return n.name }

// TestAnAccountWhosePartsDoNotHashToItsLeafIsRefused — #4453, executor spec,
// "Sync", §3: "The node hashes the state it was served, as hashState does, and
// compares it with the leaf hash served beside it; a mismatch is that peer's
// reply refused, and the account is asked of the next peer." Peer a serves
// alice's leaf beside a body it was not hashed from; peer b serves her whole.
// She is refused from a and taken from b; with a alone she is owed, and
// nothing a served is written.
func TestAnAccountWhosePartsDoNotHashToItsLeafIsRefused(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)
	prod := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})}
	var asked int
	bent := bentStates{Source: prod, name: "peer a", asked: &asked}
	honest := named{Source: prod, name: "peer b"}

	joining := func(srcs ...pull.Source) (*PulledState, *database.Database) {
		db := database.OpenInMemory(nil)
		t.Cleanup(func() { _ = db.Close() })
		values, _ := genesisValues(t, 4)
		putNetwork(t, db, here, values)
		s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: srcs}, ExecutedBlock: 1})
		require.NoError(t, err)
		return s, db
	}
	has := func(db *database.Database) bool {
		b := db.Begin(false)
		defer b.Discard()
		_, err := b.Account(alice).Main().Get()
		return err == nil
	}

	// a alone: refused, owed, nothing written.
	s, db := joining(bent)
	loadSectionOf(t, s, alice)
	require.True(t, s.sync.covered(), "precondition: the section alice is in was paged")
	require.False(t, has(db), "alice was written from a reply whose parts do not hash to its leaf")
	require.Contains(t, s.sync.retry, accountKey(alice), "alice was not owed after the only peer's reply was refused")

	// a, then b: taken from b, as b serves her.
	asked = 0
	s, db = joining(bent, honest)
	loadSectionOf(t, s, alice)
	require.NotZero(t, asked, "precondition: peer a was asked for alice")
	require.True(t, has(db), "alice was not asked of the next peer")
	require.Equal(t, int64(5), balanceIn(t, db, alice), "alice was taken from the peer whose reply does not hash to its leaf")
	require.Equal(t, leafIn(t, peerDB, alice).ValueHash, leafIn(t, db, alice).ValueHash)
	require.NotContains(t, s.sync.retry, accountKey(alice))
	require.True(t, s.sync.servedBy["peer b"], "precondition: peer b served alice")
}

// TestAMainlessAccountTheRecordsNameDoesNotHoldTheRecords — #4453 review F4.
// The records name an account the peers hold a leaf for and no main state
// (#4437). Every peer holds the same state, so an answer that fails its page
// for that one account fails on every peer and pins L. It is that account's
// answer: served without a body, refused, owed; the records move on, and the
// other accounts they name are taken.
func TestAMainlessAccountTheRecordsNameDoesNotHoldTheRecords(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	ghost := protocol.AccountUrl("ghost", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	peer := &onePeer{partition: here, querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"}), block: 10}
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: peer, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, s.Pull(ctx))

	// Blocks 11 and 12: alice changes, and the records name ghost, whose
	// leaf the peer holds with no main state beneath it.
	putTokens(t, peerDB, alice, 9)
	b := peerDB.Begin(true)
	require.NoError(t, b.BPT().Insert(bptkey.NewKey("Account", ghost), []byte{1, 31: 1}))
	ledger := b.Account(here.JoinPath(protocol.Ledger))
	require.NoError(t, ledger.BlockLedger(11).Put(&database.BlockLedger{Index: 11, Entries: []*protocol.BlockEntry{{Account: ghost, Chain: "main"}}}))
	require.NoError(t, ledger.BlockLedger(12).Put(&database.BlockLedger{Index: 12, Entries: []*protocol.BlockEntry{{Account: alice, Chain: "main"}}}))
	require.NoError(t, b.Commit())
	putLedger(t, peerDB, here, 12)
	peer.block = 12

	require.NoError(t, s.Pull(ctx))
	require.Equal(t, uint64(12), s.sync.last, "a mainless account the records name held the records where they stood")
	require.Equal(t, int64(9), balanceIn(t, db, alice), "the other accounts the records name were not taken")
	require.Contains(t, s.sync.retry, accountKey(ghost), "the mainless account was not owed")
}

// restamping is a peer that says its BPT pages and its AccountStates answers
// were read at other blocks than they were.
type restamping struct {
	api.Querier
	page, states uint64
}

func (q *restamping) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := q.Querier.Query(ctx, scope, query)
	switch r := r.(type) {
	case *api.BptPageRecord:
		if q.page != 0 {
			r.Block = q.page
		}
	case *api.AccountStatesRecord:
		if _, ok := query.(*api.AccountStatesQuery); ok && q.states != 0 {
			r.Block = q.states
		}
	}
	return r, err
}

// joiningOne is a node joining a partition of one peer: the production
// querier over peerDB, behind wrap.
func joiningOne(t *testing.T, peerDB *database.Database, wrap func(api.Querier) api.Querier) (*PulledState, *database.Database) {
	t.Helper()
	here := protocol.PartitionUrl("BVN0")
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	q := wrap(apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"}))
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, querier: q, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)
	return s, db
}

// TestTheStateIsNotReadyBelowTheHighestBlockAReplyWasReadAt — #4453 review
// F3, executor spec, "Sync", §3: "The state is complete when the walk has
// covered the tree, nothing is owed, and L is at or above the highest B any
// reply was read at." The records stand at 10; the walk's page, and the
// accounts taken from it, were read at 12, and an account may have changed at
// 11 or 12 in a way no record processed yet names. Not ready until L is 12.
func TestTheStateIsNotReadyBelowTheHighestBlockAReplyWasReadAt(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)

	// The control: every reply at 10, and the state is ready.
	s, _ := joiningOne(t, peerDB, func(q api.Querier) api.Querier { return q })
	require.NoError(t, s.Pull(context.Background()))
	require.True(t, s.sync.ready, "precondition: with every reply at the records' block the state is ready")

	s, db := joiningOne(t, peerDB, func(q api.Querier) api.Querier { return &restamping{Querier: q, page: 12, states: 12} })
	loadSectionOf(t, s, alice)
	require.True(t, s.sync.covered(), "precondition: the section alice is in was paged")
	require.Empty(t, s.sync.retry, "precondition: nothing is owed")
	require.Equal(t, int64(5), balanceIn(t, db, alice), "precondition: alice was taken")
	require.Equal(t, uint64(10), s.sync.last)
	require.False(t, s.sync.ready, "the state was ready at L = 10 holding accounts read at block 12")
}

// TestAnAccountStatesAnswerBelowTheStartIsRefused — #4453 review F3, executor
// spec, "Sync", §3. The pull starts at S = 10. The walk's page is of block
// 10, and the peer answers for its stale accounts from a view at block 9: an
// account changed at 10 shows its old state there, and no record after S
// names it. The answer is refused, and the account owed.
func TestAnAccountStatesAnswerBelowTheStartIsRefused(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)

	s, db := joiningOne(t, peerDB, func(q api.Querier) api.Querier { return &restamping{Querier: q, states: 9} })
	loadSectionOf(t, s, alice)
	require.True(t, s.sync.covered(), "precondition: the section alice is in was paged")
	b := db.Begin(false)
	defer b.Discard()
	_, err := b.Account(alice).Main().Get()
	require.Error(t, err, "alice was taken from an answer read at block 9, before the pull's start at 10")
	require.Contains(t, s.sync.retry, accountKey(alice), "alice was not owed")
}
