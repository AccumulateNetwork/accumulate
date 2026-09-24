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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// scriptedPeer is a partition's peers as the join reads them: the block its
// ledger says, the block-ledger record of each block, the BPT pages of its
// walk, and each account's current state -- all set by the test, so that the
// order a page and a record are seen in is the test's to choose.
type scriptedPeer struct {
	partition *url.URL
	block     uint64
	records   map[uint64][]*url.URL
	page      []*api.BptLeafSummary

	// state is the store an account is served from. onPage, if set, runs
	// when the walk asks for a page: a test uses it to make the peer that
	// answers the walk's pulls another one than the peer that answered the
	// records.
	state  *database.Database
	onPage func()
}

func (p *scriptedPeer) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	src := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: p.state, Partition: "BVN0"})}
	return []pull.Source{servedAt{Source: src, db: p.state}}, p.partition, nil
}

func (p *scriptedPeer) Querier(*url.URL) api.Querier { return p }

func (p *scriptedPeer) Query(_ context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	switch q := query.(type) {
	case *api.DefaultQuery:
		if scope.Equal(p.partition.JoinPath(protocol.Ledger)) {
			return &api.AccountRecord{Account: &protocol.SystemLedger{Url: scope, Index: p.block}}, nil
		}
	case *api.BlockQuery:
		named, ok := p.records[*q.Minor]
		if !ok || q.EntryRange.Start > 0 {
			return nil, errors.NotFound.WithFormat("block %d is empty", *q.Minor)
		}
		r := new(api.RecordRange[*api.ChainEntryRecord[api.Record]])
		for _, u := range named {
			r.Records = append(r.Records, &api.ChainEntryRecord[api.Record]{Account: u, Name: "main"})
		}
		r.Total = uint64(len(r.Records))
		return &api.MinorBlockRecord{Index: *q.Minor, Entries: r}, nil
	case *api.BptPageQuery:
		if p.onPage != nil {
			p.onPage()
		}
		return &api.BptPageRecord{Entries: p.page, Done: true}, nil
	}
	// Everything else -- the anchor pool among it -- this peer has nothing
	// of. The walk does not depend on anchors.
	return nil, errors.NotFound.WithFormat("%v: not scripted", scope)
}

// putTokens writes a token account holding balance into db, as a peer that
// executed the block that left it so holds it.
func putTokens(t *testing.T, db *database.Database, u *url.URL, balance int64) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(balance)}))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// leafIn is the BPT leaf an account has in db.
func leafIn(t *testing.T, db *database.Database, u *url.URL) api.BptLeafSummary {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	h, err := b.Account(u).Hash()
	require.NoError(t, err)
	return api.BptLeafSummary{Account: u, KeyHash: b.Account(u).Key().Hash(), ValueHash: h}
}

func balanceIn(t *testing.T, db *database.Database, u *url.URL) int64 {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	var acct *protocol.TokenAccount
	require.NoError(t, b.Account(u).Main().GetAs(&acct))
	return acct.Balance.Int64()
}

// joiningAt is a node whose own store holds its genesis network accounts and
// nothing else, joining a partition whose peers the test scripts.
func joiningAt(t *testing.T, peer *scriptedPeer) (*PulledState, *database.Database) {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, peer.partition, values)
	s, err := NewState(StateOptions{Partition: peer.partition, Database: db, Sources: peer, ExecutedBlock: 1})
	require.NoError(t, err)
	return s, db
}

// TestTheWalkNeverOverwritesWhatARecordWrote — the algorithm, step 1 (executor
// spec, "Sync"): "an account a block-ledger record has already brought current
// is skipped when the walk reaches it, because the walk's page may be older
// than that record."
//
// The pull starts at block 10. Block 11 changes alice's account, 5 to 7; its
// record names it and the join pulls it at 7. The walk's page is older: it
// still carries the leaf of 5, and the peer that answers the walk is one that
// has not executed block 11 yet, so pulling alice there would write 5 over
// the 7 the record wrote -- a state no block after 10 ever had, and no later
// record names alice to put it right.
func TestTheWalkNeverOverwritesWhatARecordWrote(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")

	behind := database.OpenInMemory(nil) // a peer still at block 10
	behind.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = behind.Close() })
	putTokens(t, behind, alice, 5)
	ahead := database.OpenInMemory(nil) // a peer that has executed block 11
	ahead.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = ahead.Close() })
	putTokens(t, ahead, alice, 7)

	peer := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{}, state: ahead}
	leaf := leafIn(t, behind, alice)
	peer.page = []*api.BptLeafSummary{&leaf}
	s, db := joiningAt(t, peer)
	ctx := context.Background()

	// The pull starts at 10 and the walk has not reached alice yet: this
	// round reads no page.
	s.sync = newSyncing()
	s.sync.start, s.sync.last = 10, 10

	// Block 11's record names alice. The records come first in a round; the
	// walk reads its page after them, and that page, and the peer answering
	// the walk's pulls, are older.
	peer.block = 11
	peer.records[11] = []*url.URL{alice}
	peer.onPage = func() { peer.state = behind }
	require.NoError(t, s.Pull(ctx))

	require.True(t, s.sync.walked, "precondition: the walk covered the tree")
	require.Equal(t, uint64(11), s.sync.last, "precondition: block 11's record was processed")
	require.Equal(t, int64(7), balanceIn(t, db, alice),
		"the walk wrote an older page's value over the value block 11's record brought current")
}

// TestAWalkPageNewerThanTheRecordsIsCaughtUpByTheRecord — the other direction
// of step 1, which needs no rule (Paul, 2026-09-25): "when the walk's page is
// newer than the records processed so far, the account it wrote was changed by
// a block the records have not reached yet, and that block's record names it,
// so it is pulled again when the records get there."
//
// The pull starts at block 10. Before the walk reads its page the peer has
// executed block 11, which changed alice to 9: the page, and the pull the walk
// makes, are newer than the records, and the walk writes 9 -- no block
// compared, and none is needed. Block 12 then changes her to 11, and blocks
// 11 and 12's records both name her. The records pull her again and write
// that, whatever the walk wrote: every account ends at the state of the last
// record processed.
func TestAWalkPageNewerThanTheRecordsIsCaughtUpByTheRecord(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")

	peerDB := database.OpenInMemory(nil)
	peerDB.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = peerDB.Close() })
	putTokens(t, peerDB, alice, 9)

	peer := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{}, state: peerDB}
	leaf := leafIn(t, peerDB, alice)
	peer.page = []*api.BptLeafSummary{&leaf}
	s, db := joiningAt(t, peer)
	ctx := context.Background()

	// Round one: the pull starts at 10, no record after it has been read,
	// and the walk reads a page from a peer already at block 11.
	require.NoError(t, s.Pull(ctx))
	require.True(t, s.sync.walked, "precondition: the walk covered the tree")
	require.Equal(t, uint64(10), s.sync.last)
	require.Equal(t, int64(9), balanceIn(t, db, alice),
		"the walk did not write what its page named: a page newer than the records is written, with no block compared")

	// Round two: blocks 11 and 12, and both records name alice. What the
	// peer holds of her now is block 12's.
	putTokens(t, peerDB, alice, 11)
	peer.block = 12
	peer.records[11] = []*url.URL{alice}
	peer.records[12] = []*url.URL{alice}
	s.sync.waitFor = 0 // no anchor is what this is about
	require.NoError(t, s.Pull(ctx))

	require.Equal(t, uint64(12), s.sync.last, "precondition: the records reached block 12")
	require.Equal(t, int64(11), balanceIn(t, db, alice),
		"the record did not write over what the walk wrote: an account ends at the state of the last record processed")
}
