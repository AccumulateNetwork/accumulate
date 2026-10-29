// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/lastcomplete"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCheckStoreNamesATornChain: a store whose RootChain head names more
// hashes than its Tail chunk holds -- what the kill left on bvn2-val3 in run
// 20261002T145702Z -- is named by chain and record; a whole store passes,
// its early elements in packed history included (#4507).
func TestCheckStoreNamesATornChain(t *testing.T) {
	store, err := bcdb.Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer store.Close()
	store.StatsEvery, store.CompressEvery, store.PackEvery = 0, 5, 10
	require.NoError(t, store.SetMergeLag(20))
	db := database.New(store, nil)
	part := protocol.PartitionUrl("BVN2")
	ledger := part.JoinPath(protocol.Ledger)

	// 83 blocks each appending one root: the open mark set (mark power 8 on
	// a ledger's root chain) holds 83 in eleven chunks.
	for i := 0; i < 83; i++ {
		batch := db.Begin(true)
		h := sha256.Sum256([]byte{byte(i)})
		require.NoError(t, batch.Account(ledger).RootChain().Inner().AddEntry(h[:], false))
		require.NoError(t, batch.Commit())
	}
	require.NoError(t, CheckStore(db, part, []string{protocol.Directory}), "a whole store passes")

	batch := db.Begin(true)
	c := batch.Account(ledger).RootChain().Inner()
	chunk, err := c.Tail(2).Get()
	require.NoError(t, err)
	chunk.Hashes = chunk.Hashes[:4]
	require.NoError(t, c.Tail(2).Put(chunk))
	require.NoError(t, batch.Commit())

	err = CheckStore(db, part, []string{protocol.Directory})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.InvalidRecord))
	require.Contains(t, err.Error(), "RootChain")
	require.Contains(t, err.Error(), "tail chunk 2 holds 4 hashes, the head says 8")
}

// TestATornStoreIsNeverResumedOnto: a state built with a torn finding reports
// nothing to resume from, whatever else would allow the whole-restart exit.
func TestATornStoreIsNeverResumedOnto(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, block: 10}, ExecutedBlock: 7,
		Torn: errors.InvalidRecord.With("store torn: RootChain tail chunk 2 holds 4 hashes, the head says 8")})
	require.NoError(t, err)
	s.peersJoining = true
	_, ok := s.Resumable()
	require.False(t, ok, "a torn store is no block's state")
}

// TestANodeAtGenesisNeverTakesTheWholeRestartExit: a node whose own block is
// genesis has nothing to resume from. Finding fewer validators serving than
// sign an anchor, it reports no block to resume from and keeps collecting,
// rather than taking an exit that loops on "no consensus checkpoint matched
// block 1" (run 20261002T145702Z, 17:18Z; #4508).
func TestANodeAtGenesisNeverTakesTheWholeRestartExit(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	// The records a resume checks, all saying genesis: the ledger and this
	// node's own executed block.
	id, _ := protocol.ParsePartitionUrl(here)
	batch := db.Begin(true)
	require.NoError(t, batch.Account(here.JoinPath(protocol.Ledger)).Main().Put(&protocol.SystemLedger{Url: here.JoinPath(protocol.Ledger), Index: protocol.GenesisBlock}))
	require.NoError(t, batch.SystemData(id).ExecutedBlock().Put(protocol.GenesisBlock))
	require.NoError(t, batch.Commit())

	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, block: 10}, ExecutedBlock: protocol.GenesisBlock})
	require.NoError(t, err)
	s.peersJoining = true
	_, ok := s.Resumable()
	require.False(t, ok, "genesis is not a block of this node's own")
}

// advancingState is a state pull whose proven block moves on by one each pull.
type advancingState struct{ synced uint64 }

func (s *advancingState) Pull(context.Context) error { s.synced++; return nil }
func (*advancingState) Promote(uint64)               {}
func (*advancingState) Demote(uint64)                {}
func (*advancingState) Resumable() (uint64, bool)    { return 0, false }
func (*advancingState) Backfill(context.Context)     {}
func (*advancingState) Executing(uint64) error       { return nil }
func (*advancingState) Ready() (uint64, bool)        { return 0, false }
func (s *advancingState) Matched(context.Context) (uint64, bool, error) {
	return s.synced, s.synced > 0, nil
}

// TestJoin_AGapThePeersAreHeldOnDoesNotHoldTheHandoffForGood: a stream's
// entry the peers hold and cannot run -- a Directory holding the anchor of a
// partition below its quorum -- leaves the same gap at every block the state
// is proven at. Waiting for the peers to deliver it waited for good, and made
// the Directory's sync depend on the halted partition (run 20261002T090546Z,
// "bvn-BVN3.acme->dn.acme/anchors missing=6292"). Once the proven state has
// moved on HeldGapBlocks with the gap standing still, the join hands off
// without it (executor spec, "Nothing in a partition's restart or sync waits
// on another partition").
func TestJoin_AGapThePeersAreHeldOnDoesNotHoldTheHandoffForGood(t *testing.T) {
	const b = 20
	src := url.MustParse("acc://bvn-BVN3.acme")
	stream := execute.StreamID{Ledger: protocol.DnUrl().JoinPath(protocol.AnchorPool), Source: src}
	gap := StreamGap{Stream: stream, Delivered: 6291, Missing: 6292, MissingTo: 6292, Received: 6292}
	stage := &fakeStage{gaps: map[uint64][]StreamGap{}}
	// The gap stands at every block the test can reach: a stream held for
	// good, not one that clears on its own.
	for n := uint64(b); n < b+20*HeldGapBlocks; n++ {
		stage.gaps[n] = []StreamGap{gap}
	}
	buf := new(fakeBuffer)
	state := &advancingState{synced: b}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "Directory", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Len(t, buf.handoffs, 1, "the join never handed off past a gap the peers are held on")
	require.GreaterOrEqual(t, buf.handoffs[0], uint64(b+HeldGapBlocks), "it handed off before the gap had stood still for HeldGapBlocks")
	require.Less(t, buf.handoffs[0], uint64(b+2*HeldGapBlocks), "it waited on the held gap long past HeldGapBlocks")
}

// TestTheJoinRefusalNamesTheLastCompleteBlock: the line a node at genesis says
// when no state can be proven to it carries the partition's last complete
// block, or that it is unknown and why (#4512).
func TestTheJoinRefusalNamesTheLastCompleteBlock(t *testing.T) {
	for name, answer := range map[string]lastcomplete.Answer{
		"known":   {Known: true, Block: 33, Root: [32]byte{0xab}, State: lastcomplete.NotObtainable},
		"unknown": {Why: "the Directory could not be read: no peer"},
	} {
		t.Run(name, func(t *testing.T) {
			here := protocol.PartitionUrl("BVN0")
			db := database.OpenInMemory(nil)
			t.Cleanup(func() { _ = db.Close() })
			values, _ := genesisValues(t, 4)
			putNetwork(t, db, here, values)
			id, _ := protocol.ParsePartitionUrl(here)
			batch := db.Begin(true)
			require.NoError(t, batch.Account(here.JoinPath(protocol.Ledger)).Main().Put(&protocol.SystemLedger{Url: here.JoinPath(protocol.Ledger), Index: protocol.GenesisBlock}))
			require.NoError(t, batch.SystemData(id).ExecutedBlock().Put(protocol.GenesisBlock))
			require.NoError(t, batch.Commit())

			var out bytes.Buffer
			s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, block: 10}, ExecutedBlock: protocol.GenesisBlock,
				Logger:       slog.New(slog.NewTextHandler(&out, nil)),
				LastComplete: func(context.Context) lastcomplete.Answer { return answer }})
			require.NoError(t, err)
			s.peersJoining = true
			_, ok := s.Resumable()
			require.False(t, ok)
			require.Contains(t, out.String(), "no state can be proven to it")
			if answer.Known {
				require.Contains(t, out.String(), "lastCompleteBlock=33")
			} else {
				require.Contains(t, out.String(), "lastCompleteBlock=unknown")
				require.Contains(t, out.String(), "no peer")
			}
		})
	}
}
