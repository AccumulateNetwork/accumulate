// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// putNetwork writes the partition's network definition naming the
// committee's validators, at the committee's epoch as its version.
func putNetwork(batch *database.Batch, partition string, c *types.Committee) error {
	def := &protocol.NetworkDefinition{NetworkName: "test", Version: c.Epoch}
	for _, v := range c.Validators {
		def.AddValidator(v.PublicKey, partition, true)
	}
	g := &network.GlobalValues{Network: def}
	u := protocol.PartitionUrl(partition).JoinPath(protocol.Network)
	return batch.Account(u).Main().Put(&protocol.DataAccount{Url: u, Entry: g.FormatNetwork()})
}

// putState writes the system ledger and network definition a state holds:
// block index at leader round round, naming committee c, which the ledger
// records activating at c.ActivationRound after version prev. It records the
// committee of version prev if prevCommittee is not nil, as a state written
// from v2-canaveral does.
func putState(t *testing.T, svc *Service, index uint64, round types.Round, c *types.Committee, prev uint64, prevCommittee ...*types.Committee) {
	t.Helper()
	u := protocol.PartitionUrl(svc.config.Partition.ID).JoinPath(protocol.Ledger)
	ledger := &protocol.SystemLedger{Url: u, Index: index, LeaderRound: uint64(round),
		CommitteeActivationRound: uint64(c.ActivationRound), PreviousNetworkVersion: prev}
	if len(prevCommittee) > 0 && prevCommittee[0] != nil {
		for _, v := range prevCommittee[0].Validators {
			ledger.PreviousValidators = append(ledger.PreviousValidators, v.PublicKey)
		}
		ledger.PreviousCommitteeActivationRound = uint64(prevCommittee[0].ActivationRound)
	}
	require.NoError(t, svc.config.Database.Update(func(batch *database.Batch) error {
		if err := putNetwork(batch, svc.config.Partition.ID, c); err != nil {
			return err
		}
		return batch.Account(u).Main().Put(ledger)
	}))
}

// changeAdapter reports a validator set change while it produces the block
// of one leader round, as the executor bridge does from the block's end.
type changeAdapter struct {
	commitAdapter
	svc        *Service
	at         types.Round
	validators []adapter.ValidatorInfo
	version    uint64
	executor   protocol.ExecutorVersion // v2-canaveral if zero
}

func (a *changeAdapter) ProduceBlock(ctx context.Context, params adapter.BlockParams) ([32]byte, error) {
	if params.LeaderRound == a.at {
		executor := a.executor
		if executor == 0 {
			executor = protocol.ExecutorVersionV2Canaveral
		}
		a.svc.onValidatorSetChange(a.validators, a.version, executor)
	}
	return a.commitAdapter.ProduceBlock(ctx, params)
}

// A committee change reported while a block is produced is scheduled at that
// block's leader round plus the activation margin, the round the executor
// records in the ledger; the committee of the rounds below it is unchanged
// (#4593). One reported outside block production -- the executor loading or
// reloading its globals -- is not this node's to schedule.
func TestACommitteeChangeTakesEffectAtTheRoundOfItsBlockPlusTheMargin(t *testing.T) {
	svc, _, author := newCommitService(t, 1)
	other, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var key [32]byte
	copy(key[:], other)
	validators := []adapter.ValidatorInfo{{PublicKey: key, Stake: 1, Active: true}}

	svc.onValidatorSetChange(validators, 2, protocol.ExecutorVersionV2Canaveral)
	require.Equal(t, uint64(1), svc.node.Committee().Epoch, "a change reported outside block production")

	ca := &changeAdapter{commitAdapter: commitAdapter{hash: [32]byte{0xAA}}, svc: svc, at: 50, validators: validators, version: 2}
	svc.adapter = ca
	_, err = svc.processCommittedGroup(group(commitCert(author, 48, time.Unix(100, 0), nil)))
	require.NoError(t, err)
	require.Equal(t, uint64(1), svc.node.Committee().Epoch)
	_, err = svc.processCommittedGroup(group(commitCert(author, 50, time.Unix(101, 0), nil)))
	require.NoError(t, err)

	activation := 50 + types.CommitteeActivationMargin
	schedule := svc.node.Committees()
	require.Equal(t, uint64(2), schedule.Latest().Epoch)
	require.Equal(t, activation, schedule.Latest().ActivationRound)
	require.Equal(t, uint64(1), schedule.At(activation-1).Epoch)
	require.Equal(t, uint64(2), schedule.At(activation).Epoch)
	require.Equal(t, uint64(2), svc.Committee().Epoch)
}

// Before v2-canaveral a committee change takes effect at once, as it did
// before #4593: the executor records no activation round for it, so a node
// that scheduled it would order with a committee a restarted or joining
// node, reading the state, does not.
func TestACommitteeChangeBeforeCanaveralTakesEffectAtOnce(t *testing.T) {
	svc, _, author := newCommitService(t, 1)
	other, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var key [32]byte
	copy(key[:], other)
	validators := []adapter.ValidatorInfo{{PublicKey: key, Stake: 1, Active: true}}

	svc.adapter = &changeAdapter{commitAdapter: commitAdapter{hash: [32]byte{0xAA}}, svc: svc, at: 50,
		validators: validators, version: 2, executor: protocol.ExecutorVersionV2Kourou}
	_, err = svc.processCommittedGroup(group(commitCert(author, 50, time.Unix(101, 0), nil)))
	require.NoError(t, err)

	committees := svc.node.Committees().Committees()
	require.Len(t, committees, 1, "the new committee alone")
	require.Equal(t, uint64(2), committees[0].Epoch)
	require.Equal(t, uint64(2), svc.node.Committees().At(0).Epoch, "for every round")
	require.Equal(t, uint64(2), svc.Committee().Epoch)
}

// A restored node schedules the committee its state names before it holds
// the checkpoint's state. The change in the checkpoint's own block activates
// at that block's leader round plus the margin, exactly the round limit the
// state sets, and a change at or below the limit is refused, so held first
// the change was never scheduled and the start ordered with the committee
// before it (#4593).
func TestARestoredNodeSchedulesTheChangeInItsCheckpointsBlock(t *testing.T) {
	const leader = 100
	svc, _, _ := newCommitService(t, 1)
	first := svc.node.Committee()
	cp := persist.NewCheckpoint("bvn1", leader+1, first.Epoch, leader, nil)
	cp.Committees = persist.EncodeCommittees([]*types.Committee{first})
	require.NoError(t, svc.node.Restore(cp))
	svc.lastLeaderRound = leader

	next := types.NewCommittee(first.Validators, 2)
	next.ActivationRound = leader + types.CommitteeActivationMargin
	require.NoError(t, svc.resumeCommittee(next, first.Epoch))
	require.Equal(t, uint64(1), svc.node.Committees().At(next.ActivationRound-1).Epoch)
	require.Equal(t, uint64(2), svc.node.Committees().At(next.ActivationRound).Epoch)
	limit, ok := svc.node.RoundLimit()
	require.True(t, ok)
	require.Equal(t, next.ActivationRound, limit)
}

// A state whose change was made before v2-canaveral records no activation
// round and no previous version: a restored node takes its committee for
// every round, as the change took effect. One the node cannot place stops
// the start.
func TestARestoredNodeTakesAChangeMadeBeforeCanaveralForEveryRound(t *testing.T) {
	restored := func(t *testing.T) *Service {
		svc, _, _ := newCommitService(t, 1)
		first := svc.node.Committee()
		cp := persist.NewCheckpoint("bvn1", 101, first.Epoch, 100, nil)
		cp.Committees = persist.EncodeCommittees([]*types.Committee{first})
		require.NoError(t, svc.node.Restore(cp))
		svc.lastLeaderRound = 100
		return svc
	}

	svc := restored(t)
	next := types.NewCommittee(svc.node.Committee().Validators, 2)
	require.NoError(t, svc.resumeCommittee(next, 0))
	require.Equal(t, []*types.Committee{next}, svc.node.Committees().Committees())

	svc = restored(t)
	later := types.NewCommittee(svc.node.Committee().Validators, 3)
	later.ActivationRound = 400
	err := svc.resumeCommittee(later, 2)
	require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
}

// committeeOf is a committee of n new validators at an epoch, from a round.
func committeeOf(t *testing.T, n int, epoch uint64, activation types.Round) *types.Committee {
	t.Helper()
	validators := make([]types.ValidatorInfo, n)
	for i := range validators {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		validators[i] = types.ValidatorInfo{PublicKey: pub, Stake: 1}
	}
	c := types.NewCommittee(validators, epoch)
	c.ActivationRound = activation
	return c
}

// A joining node whose schedule starts with a committee that took over above
// its seed -- one built from the state when it started -- holds no
// committee for the rounds between. Seeded below it, it would order them
// with the wrong committee. With a state that records the committee before
// it, the node orders those rounds with that one; with an older state that
// records none, the state is refused and the join pulls a later one; a state
// seeded at or above the activation round is taken (#4593).
func TestAJoinerSeededBelowItsFirstCommitteesActivationRound(t *testing.T) {
	t.Run("the state records no previous committee", func(t *testing.T) {
		svc, _, _ := newJoiningService(t)
		only := types.NewCommittee(svc.node.Committee().Validators, 2)
		only.ActivationRound = 1990
		require.NoError(t, svc.node.ReplaceCommittee(only))

		putState(t, svc, 90, 2000, only, 1)
		_, err := svc.seedCommittee(2000)
		require.True(t, errors.Is(err, errors.Conflict), "seeded at %d, below %d: %v", seedRound(2000, svc.config.NodeConfig.DAGGCDepth), only.ActivationRound, err)

		putState(t, svc, 105, 2030, only, 1)
		committees, err := svc.seedCommittee(2030)
		require.NoError(t, err)
		require.Nil(t, committees)
	})

	t.Run("the state records the previous committee", func(t *testing.T) {
		svc, _, _ := newJoiningService(t)
		first := committeeOf(t, 4, 1, 0)
		only := types.NewCommittee(svc.node.Committee().Validators, 2)
		only.ActivationRound = 1990
		require.NoError(t, svc.node.ReplaceCommittee(only))

		putState(t, svc, 90, 2000, only, 1, first)
		committees, err := svc.seedCommittee(2000)
		require.NoError(t, err)
		require.Equal(t, []uint64{1, 2}, epochs(committees))
	})

	// The reviewer's case: the schedule's own first committee is the one
	// before the state's, and it too took over above the seed.
	t.Run("the schedule's first committee is the state's previous", func(t *testing.T) {
		svc, _, _ := newJoiningService(t)
		second := types.NewCommittee(svc.node.Committee().Validators, 2)
		second.ActivationRound = 1990
		require.NoError(t, svc.node.ReplaceCommittee(second))
		third := committeeOf(t, 4, 3, 2200)

		putState(t, svc, 90, 2000, third, 2, second)
		_, err := svc.seedCommittee(2000)
		require.True(t, errors.Is(err, errors.Conflict), "two committees take over above the seed: %v", err)

		putState(t, svc, 105, 2030, third, 2, second)
		committees, err := svc.seedCommittee(2030)
		require.NoError(t, err)
		require.Nil(t, committees, "the schedule covers every round above the seed")
	})
}

// A joining node whose first state is above the activation round of its
// committee, but seeded below it, orders the rounds between with the
// committee before it and the rest with the state's. Refusing that state, as
// it did, it refused every later one too -- the rounds those name are
// ordered by the new committee, whose quorum may need this node -- and
// ordered nothing for good (#4593).
func TestAJoinerOrdersTheRoundsBelowTheStatesActivationWithThePreviousCommittee(t *testing.T) {
	svc, _, _ := newUnseededService(t)
	first := committeeOf(t, 4, 1, 0)
	second := types.NewCommittee(svc.node.Committee().Validators, 2)
	second.ActivationRound = 1990
	require.NoError(t, svc.node.ReplaceCommittee(second))

	putState(t, svc, 90, 2000, second, 1, first)
	err := svc.stageThroughNow(91)
	require.True(t, errors.Is(err, errors.NotReady), "seeded, and the block after the state is not collected yet: %v", err)
	seed := types.Round(2000 - seedBelow)
	require.Equal(t, seed, svc.node.LastCommitRound())
	require.Equal(t, uint64(1), svc.node.Committees().At(1989).Epoch)
	require.Equal(t, uint64(2), svc.node.Committees().At(1990).Epoch)
}

// A node that joins from genesis, with the genesis committee alone, takes the
// state's committee and the one before it from the state when that one took
// over at or below the seed: it never had it (#4593).
func TestAJoinerFromGenesisTakesThePreviousCommitteeFromTheState(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	second := committeeOf(t, 4, 2, 1000)
	third := committeeOf(t, 4, 3, 2200)
	putState(t, svc, 105, 2030, third, 2, second)
	committees, err := svc.seedCommittee(2030)
	require.NoError(t, err)
	require.Equal(t, []uint64{2, 3}, epochs(committees))
	require.Equal(t, types.Round(1000), committees[0].ActivationRound)
	require.Equal(t, types.Round(2200), committees[1].ActivationRound)
}

// The previous committee a joiner takes from the state is the one the nodes
// that lived through it built from its network definition: the same members
// in the same order, as leader election picks by position. The ledger records
// the keys of the definition's validators active on the partition, in the
// definition's order (#4593).
func TestAJoinersPreviousCommitteeIsTheOneBuiltFromThePreviousDefinition(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	part := svc.config.Partition.ID
	prevDef := &protocol.NetworkDefinition{NetworkName: "test", Version: 2}
	for i, active := range []bool{true, false, true, true, false, true} {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		if i == 4 {
			prevDef.AddValidator(pub, "other", true)
			continue
		}
		prevDef.AddValidator(pub, part, active)
	}
	built := types.NewCommittee(committeeValidators(validatorsOf(prevDef, part)), prevDef.Version)
	require.Len(t, built.Validators, 4)

	var recorded [][]byte
	for _, v := range prevDef.Validators {
		if v.IsActiveOn(part) {
			recorded = append(recorded, v.PublicKey)
		}
	}
	third := committeeOf(t, 4, 3, 2200)
	putState(t, svc, 105, 2030, third, prevDef.Version)
	u := protocol.PartitionUrl(part).JoinPath(protocol.Ledger)
	require.NoError(t, svc.config.Database.Update(func(batch *database.Batch) error {
		var l *protocol.SystemLedger
		if err := batch.Account(u).Main().GetAs(&l); err != nil {
			return err
		}
		l.PreviousValidators = recorded
		l.PreviousCommitteeActivationRound = 1000
		return batch.Account(u).Main().Put(l)
	}))

	committees, err := svc.seedCommittee(2030)
	require.NoError(t, err)
	require.Equal(t, []uint64{2, 3}, epochs(committees))
	require.Equal(t, built.Validators, committees[0].Validators)
}

func epochs(committees []*types.Committee) []uint64 {
	var out []uint64
	for _, c := range committees {
		out = append(out, c.Epoch)
	}
	return out
}

// A pulled state raises the round limit to its round plus the margin when
// this node can place the committee it names: its own last committee, or the
// one after it. One after a committee this node never had is refused: the
// node cannot know the committee of the rounds between (#4593).
func TestAPulledStateRaisesTheRoundLimitOnlyWithACommitteeTheNodeCanPlace(t *testing.T) {
	t.Run("the node's own committee", func(t *testing.T) {
		svc, _, _ := newCommitService(t, 1)
		svc.node.StateAt(10)
		putState(t, svc, 90, 400, svc.node.Committee(), 0)
		require.NoError(t, svc.adoptPulledState(400))
		limit, ok := svc.node.RoundLimit()
		require.True(t, ok)
		require.Equal(t, 400+types.CommitteeActivationMargin, limit)
	})

	t.Run("the committee after it", func(t *testing.T) {
		svc, _, _ := newCommitService(t, 1)
		svc.node.StateAt(10)
		next := types.NewCommittee(svc.node.Committee().Validators, 2)
		next.ActivationRound = 300
		putState(t, svc, 90, 400, next, 1)
		require.NoError(t, svc.adoptPulledState(400))
		require.Equal(t, uint64(1), svc.node.Committees().At(299).Epoch)
		require.Equal(t, uint64(2), svc.node.Committees().At(300).Epoch)
		limit, _ := svc.node.RoundLimit()
		require.Equal(t, 400+types.CommitteeActivationMargin, limit)
	})

	t.Run("one after a committee the node never had", func(t *testing.T) {
		svc, _, _ := newCommitService(t, 1)
		svc.node.StateAt(10)
		later := types.NewCommittee(svc.node.Committee().Validators, 3)
		later.ActivationRound = 300
		putState(t, svc, 90, 400, later, 2)
		err := svc.adoptPulledState(400)
		require.True(t, errors.Is(err, errors.Conflict), "got %v", err)
		require.Equal(t, uint64(1), svc.node.Committee().Epoch)
		limit, _ := svc.node.RoundLimit()
		require.Equal(t, 10+types.CommitteeActivationMargin, limit)
	})
}

// A restored node that was down across two committee changes cannot place the
// pulled state's committee after its own, and does not know the committee of
// the rounds between. It drops its checkpoint's position and joins as a node
// with no checkpoint does: seeded below the state with the state's committee,
// it hands off there and produces the blocks after it (#4593).
func TestARestoredNodeTwoCommitteeChangesBehindJoinsAsANodeWithNoCheckpoint(t *testing.T) {
	svc, ca, author := newJoiningService(t)
	w := svc.node.Workers()[0]

	// Restored at the block of leader 10, epoch 1, and collecting.
	svc.node.StateAt(10)
	svc.lastBlockIndex, svc.lastLeaderRound = 5, 10
	svc.StartCollecting()
	collect := func(round types.Round) {
		b := types.NewBatch([][]byte{{byte(round)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, round, time.Unix(int64(100+round), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	collect(12)
	collect(14)

	// The state at round 400 names epoch 3, which took over from epoch 2 at
	// round 300; this node never had epoch 2.
	third := types.NewCommittee(svc.node.Committee().Validators, 3)
	third.ActivationRound = 300
	putState(t, svc, 90, 400, third, 2)
	require.True(t, errors.Is(svc.adoptPulledState(400), errors.Conflict), "the state's committee cannot be placed")

	err := svc.stageThroughNow(91)
	require.True(t, errors.Is(err, errors.NotReady), "seeded, and the block after the state is not collected yet: %v", err)
	require.False(t, svc.awaitingSeed)
	seed := types.Round(400 - seedBelow)
	require.Equal(t, seed, svc.node.LastCommitRound(), "ordering from below the state, as a node with no checkpoint does")
	require.Equal(t, seed+bullshark.RescueWindow, svc.seedFloor)
	require.Equal(t, []*types.Committee{third}, svc.node.Committees().Committees(), "with the state's committee alone")
	limit, _ := svc.node.RoundLimit()
	require.Equal(t, 400+types.CommitteeActivationMargin, limit)

	// It follows the network from there.
	collect(400)
	collect(402)
	collect(404)
	require.NoError(t, svc.stageThroughNow(91))
	require.NoError(t, svc.performHandoff(90))
	require.False(t, svc.Collecting())
	require.Len(t, ca.blocks, 2)
	require.Equal(t, uint64(91), ca.blocks[0].Index)
	require.Equal(t, types.Round(402), ca.blocks[0].LeaderRound)
	require.Equal(t, types.Round(404), ca.blocks[1].LeaderRound)
}
