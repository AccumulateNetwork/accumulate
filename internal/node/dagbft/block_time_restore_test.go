// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// pullStateAt is pullState with the time the ledger records for the block,
// as an executor from v2-canaveral records it: to the second in Timestamp,
// and the offset within that second in TimestampNanosecond.
func pullStateAt(t *testing.T, svc *Service, index uint64, round types.Round, at time.Time) {
	t.Helper()
	putLedger(t, svc, &protocol.SystemLedger{Index: index, LeaderRound: uint64(round), Timestamp: at, TimestampNanosecond: uint64(at.Nanosecond())})
}

// pullStateAtSeconds is pullStateAt as an executor before v2-canaveral records
// it: to the second, in Timestamp alone.
func pullStateAtSeconds(t *testing.T, svc *Service, index uint64, round types.Round, at time.Time) {
	t.Helper()
	putLedger(t, svc, &protocol.SystemLedger{Index: index, LeaderRound: uint64(round), Timestamp: at})
}

func putLedger(t *testing.T, svc *Service, ledger *protocol.SystemLedger) {
	t.Helper()
	ledger.Url = protocol.PartitionUrl(svc.config.Partition.ID).JoinPath(protocol.Ledger)
	require.NoError(t, svc.config.Database.Update(func(batch *database.Batch) error {
		if err := putNetwork(batch, svc.config.Partition.ID, svc.Committee()); err != nil {
			return err
		}
		return batch.Account(ledger.Url).Main().Put(ledger)
	}))
}

// A block's time is its leader's stamp raised to after the previous block's
// time (consensus spec, "Restart"). A node that restarts raises the first
// block after the restart exactly as a peer that did not restart raises it:
// the checkpoint the restart resumes from carries its block's time.
func TestBlockTime_ARestartedNodeTimesTheNextBlockAsItsPeers(t *testing.T) {
	dir := t.TempDir()
	first := time.Unix(100, 700_000_000)
	slow := time.Unix(100, 200_000_000) // the next leader's clock is behind

	// A peer that does not restart, and the node before its restart, produce
	// block 1 from the same group.
	peer, peerBlocks, author := newCommitService(t, 1)
	node, _, _ := newCommitService(t, 1)
	node.config.DataDir = dir
	for _, svc := range []*Service{peer, node} {
		svc.node.Bullshark().SetLastCommitRound(4)
		svc.node.Primary().SetRound(5)
		_, err := svc.processCommittedGroup(group(commitCert(author, 4, first, nil)))
		require.NoError(t, err)
	}

	// The restart: the executor stands at block 1, whose ledger records its
	// time to the second.
	restarted, restartedBlocks, _ := newCommitService(t, 1)
	restarted.config.DataDir = dir
	restartedBlocks.last = 1
	pullStateAt(t, restarted, 1, 4, first.Truncate(time.Second))
	require.NoError(t, restarted.initializeGenesis())
	seeded, err := restarted.seedFromCheckpoint()
	require.NoError(t, err)
	require.True(t, seeded, "precondition: the checkpoint for block 1 is restored")

	// Block 2's leader stamped it before block 1's time.
	for _, svc := range []*Service{peer, restarted} {
		_, err := svc.processCommittedGroup(group(commitCert(author, 6, slow, nil)))
		require.NoError(t, err)
	}

	require.Len(t, peerBlocks.blocks, 2)
	require.Len(t, restartedBlocks.blocks, 1)
	want := first.Add(time.Millisecond).UTC()
	require.Equal(t, want, peerBlocks.blocks[1].Time, "precondition: the peer raises the slow stamp")
	require.Equal(t, uint64(2), restartedBlocks.blocks[0].Index)
	require.Equal(t, want, restartedBlocks.blocks[0].Time, "the restarted node must time block 2 as its peer does")
}

// A checkpoint written before checkpoints carried the block's time restores
// none; the node raises the next block's time above the time its ledger
// records for its last block.
func TestBlockTime_ACheckpointWithoutATimeFallsBackToTheLedger(t *testing.T) {
	dir := t.TempDir()
	store := persist.NewStore(dir)
	store.SetFilename(checkpointFile)
	require.NoError(t, store.Save(&persist.Checkpoint{
		Version: 1, Partition: "bvn1", BlockIndex: 1, CurrentRound: 5, LastCommitRound: 4,
	}))

	restarted, ca, author := newCommitService(t, 1)
	restarted.config.DataDir = dir
	ca.last = 1
	recorded := time.Unix(100, 0).UTC()
	pullStateAtSeconds(t, restarted, 1, 4, recorded)
	require.NoError(t, restarted.initializeGenesis())
	seeded, err := restarted.seedFromCheckpoint()
	require.NoError(t, err)
	require.True(t, seeded)

	_, err = restarted.processCommittedGroup(group(commitCert(author, 6, time.Unix(50, 0), nil)))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 1)
	require.Equal(t, recorded.Add(time.Millisecond), ca.blocks[0].Time)
}

// A joining node produces the blocks after the state it pulled, and raises
// the first one's time above the time that state's ledger records. A ledger
// written before v2-canaveral records it to the second.
func TestBlockTime_AJoinedNodeTimesTheNextBlockAfterThePulledState(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx
	svc.StartCollecting()
	for i, round := range []types.Round{20, 22} {
		_, err := svc.processCommittedGroup(group(commitCert(author, round, time.Unix(int64(50+i), 0), nil)))
		require.NoError(t, err)
	}

	svc.wg.Add(1)
	go svc.blockProductionLoop()
	defer func() { cancel(); svc.wg.Wait() }()

	recorded := time.Unix(200, 0).UTC()
	pullStateAtSeconds(t, svc, 90, 20, recorded)
	require.NoError(t, svc.Handoff(90))

	require.Len(t, ca.blocks, 1)
	require.Equal(t, uint64(91), ca.blocks[0].Index)
	require.Equal(t, recorded.Add(time.Millisecond), ca.blocks[0].Time)
}

// From v2-canaveral the ledger records the block's time to the nanosecond, so
// a joining node raises the first block after the state it pulled exactly as
// its peers raised it, even when that block's leader stamped it before the
// pulled block's time.
func TestBlockTime_AJoinedNodeTimesTheNextBlockAsItsPeers(t *testing.T) {
	t90 := time.Unix(200, 999_500_000)
	t91 := time.Unix(200, 500_000_000) // the next leader's clock is behind

	peer, peerBlocks, pa := newCommitService(t, 1)
	peer.node.Bullshark().SetLastCommitRound(19)
	peer.node.Primary().SetRound(20)
	for _, g := range []struct {
		round types.Round
		at    time.Time
	}{{20, t90}, {22, t91}} {
		_, err := peer.processCommittedGroup(group(commitCert(pa, g.round, g.at, nil)))
		require.NoError(t, err)
	}
	want := peerBlocks.blocks[1].Time

	svc, ca, author := newJoiningService(t)
	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx
	svc.StartCollecting()
	for _, g := range []struct {
		round types.Round
		at    time.Time
	}{{20, t90}, {22, t91}} {
		_, err := svc.processCommittedGroup(group(commitCert(author, g.round, g.at, nil)))
		require.NoError(t, err)
	}
	svc.wg.Add(1)
	go svc.blockProductionLoop()
	defer func() { cancel(); svc.wg.Wait() }()

	pullStateAt(t, svc, 90, 20, t90)
	require.NoError(t, svc.Handoff(90))
	require.Len(t, ca.blocks, 1)
	require.Equal(t, want, ca.blocks[0].Time, "the joined node must time block 91 as its peer does")
}

// A restart with a checkpoint that records no time reads the block's time
// from the ledger, to the nanosecond from v2-canaveral.
func TestBlockTime_ALedgerTimeIsReadToTheNanosecond(t *testing.T) {
	dir := t.TempDir()
	store := persist.NewStore(dir)
	store.SetFilename(checkpointFile)
	require.NoError(t, store.Save(&persist.Checkpoint{
		Version: 1, Partition: "bvn1", BlockIndex: 1, CurrentRound: 5, LastCommitRound: 4,
	}))

	restarted, ca, author := newCommitService(t, 1)
	restarted.config.DataDir = dir
	ca.last = 1
	recorded := time.Unix(100, 999_500_000).UTC()
	pullStateAt(t, restarted, 1, 4, recorded)
	require.NoError(t, restarted.initializeGenesis())
	seeded, err := restarted.seedFromCheckpoint()
	require.NoError(t, err)
	require.True(t, seeded)

	_, err = restarted.processCommittedGroup(group(commitCert(author, 6, time.Unix(100, 500_000_000), nil)))
	require.NoError(t, err)
	require.Len(t, ca.blocks, 1)
	require.Equal(t, recorded.Add(time.Millisecond), ca.blocks[0].Time)
}

// A Byzantine leader may stamp its header at the largest time a header can
// carry, and the clamp then raises the next blocks past it, beyond the range
// of Unix nanoseconds (consensus spec, "Restart"). A node that joins from the
// ledger, or restarts from its ledger or its checkpoint, still times the next
// block as a peer that did not stop.
func TestBlockTime_TheLargestStampSurvivesTheLedgerAndTheCheckpoint(t *testing.T) {
	stamps := []struct {
		round types.Round
		at    time.Time
	}{
		{4, time.Unix(0, math.MaxInt64)}, // block 1: the largest stamp
		{6, time.Unix(100, 0)},           // block 2: raised past it
		{8, time.Unix(200, 0)},           // block 3: raised again
	}

	peer, peerBlocks, pa := newCommitService(t, 1)
	peer.node.Bullshark().SetLastCommitRound(3)
	peer.node.Primary().SetRound(4)
	for _, s := range stamps {
		_, err := peer.processCommittedGroup(group(commitCert(pa, s.round, s.at, nil)))
		require.NoError(t, err)
	}
	require.Len(t, peerBlocks.blocks, 3)
	at2, want := peerBlocks.blocks[1].Time, peerBlocks.blocks[2].Time
	require.Equal(t, time.Unix(0, math.MaxInt64).Add(2*time.Millisecond).UTC(), want, "precondition: the peer raises both later stamps")

	// A node that joins from the state at block 2.
	joined, ja, author := newJoiningService(t)
	ctx, cancel := context.WithCancel(context.Background())
	joined.ctx = ctx
	joined.StartCollecting()
	for _, s := range stamps {
		_, err := joined.processCommittedGroup(group(commitCert(author, s.round, s.at, nil)))
		require.NoError(t, err)
	}
	joined.wg.Add(1)
	go joined.blockProductionLoop()
	defer func() { cancel(); joined.wg.Wait() }()
	pullStateAt(t, joined, 2, 6, at2)
	require.NoError(t, joined.Handoff(2))
	require.Len(t, ja.blocks, 1)
	require.Equal(t, want, ja.blocks[0].Time, "a node that joined from the ledger must time block 3 as its peer does")

	// A node that restarts at block 2, from its ledger and from its
	// checkpoint.
	for _, withTime := range []bool{false, true} {
		dir := t.TempDir()
		store := persist.NewStore(dir)
		store.SetFilename(checkpointFile)
		cp := &persist.Checkpoint{Version: 1, Partition: "bvn1", BlockIndex: 2, CurrentRound: 7, LastCommitRound: 6}
		if withTime {
			cp.BlockTime = at2
		}
		require.NoError(t, store.Save(cp))

		restarted, ra, author := newCommitService(t, 1)
		restarted.config.DataDir = dir
		ra.last = 2
		pullStateAt(t, restarted, 2, 6, at2)
		require.NoError(t, restarted.initializeGenesis())
		seeded, err := restarted.seedFromCheckpoint()
		require.NoError(t, err)
		require.True(t, seeded)

		_, err = restarted.processCommittedGroup(group(commitCert(author, 8, stamps[2].at, nil)))
		require.NoError(t, err)
		require.Len(t, ra.blocks, 1)
		require.Equal(t, want, ra.blocks[0].Time, "a node that restarted (checkpoint time %v) must time block 3 as its peer does", withTime)
	}
}

// The ledger's time, as the executor records it from v2-canaveral, survives
// the ledger's binary encoding for every time the clamp can produce: past
// the range of Unix nanoseconds, before 1970, and on a whole second.
func TestBlockTime_TheLedgerTimeSurvivesItsEncoding(t *testing.T) {
	largest := time.Unix(0, math.MaxInt64)
	for _, at := range []time.Time{
		largest,
		largest.Add(time.Millisecond),
		largest.Add(time.Hour),
		time.Unix(0, math.MinInt64),
		time.Unix(-1, 5),
		time.Unix(100, 999_999_999),
		time.Unix(100, 0),
	} {
		ledger := &protocol.SystemLedger{Url: protocol.PartitionUrl("bvn1").JoinPath(protocol.Ledger), Index: 1, Timestamp: at, TimestampNanosecond: uint64(at.Nanosecond())}
		b, err := ledger.MarshalBinary()
		require.NoError(t, err)
		read := new(protocol.SystemLedger)
		require.NoError(t, read.UnmarshalBinary(b))
		require.Equal(t, at.UTC(), ledgerBlockTime(read), "%v", at)
	}
}
