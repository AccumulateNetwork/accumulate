// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A committee change takes effect at a DAG round fixed by the committed
// sequence (consensus spec, "Committee changes"; #4593): the leader round of
// the block that activates the new network definition, plus
// types.CommitteeActivationMargin. The executor records that round in the
// system ledger; a node that executes the block schedules the committee from
// it, and a node that restarts or joins reads it back from its state.

// stateCommittee is the committee the state in the store names for this
// partition: the validators of its active network definition, the
// definition's version as the epoch, and the round the system ledger records
// it activating at. It also returns the version before it, whose committee
// orders the rounds below that one, and that committee as the ledger records
// it, or nil if the ledger records none: a state of a change before
// v2-canaveral, or at genesis.
func (s *Service) stateCommittee() (*types.Committee, uint64, *types.Committee, error) {
	var def *protocol.NetworkDefinition
	var ledger *protocol.SystemLedger
	part := protocol.PartitionUrl(s.config.Partition.ID)
	err := s.config.Database.View(func(batch *database.Batch) error {
		getState := func(u *url.URL, target interface{}) error {
			return batch.Account(u).Main().GetAs(target)
		}
		g := new(network.GlobalValues)
		if err := g.LoadNetwork(part, getState); err != nil {
			return err
		}
		def = g.Network
		return getState(part.JoinPath(protocol.Ledger), &ledger)
	})
	if err != nil {
		return nil, 0, nil, errors.UnknownError.WithFormat("%s: read the state's committee: %w", s.config.Partition.ID, err)
	}
	c := types.NewCommittee(committeeValidators(validatorsOf(def, s.config.Partition.ID)), def.Version)
	c.ActivationRound = types.Round(ledger.CommitteeActivationRound)
	return c, ledger.PreviousNetworkVersion, previousCommittee(ledger), nil
}

// previousCommittee is the committee before the active one as a system
// ledger records it, or nil if it records none.
func previousCommittee(ledger *protocol.SystemLedger) *types.Committee {
	if ledger.PreviousNetworkVersion == 0 {
		return nil
	}
	var validators []types.ValidatorInfo
	for _, key := range ledger.PreviousValidators {
		if len(key) == 32 {
			validators = append(validators, types.ValidatorInfo{PublicKey: key, Stake: 1})
		}
	}
	if len(validators) == 0 {
		return nil
	}
	c := types.NewCommittee(validators, ledger.PreviousNetworkVersion)
	c.ActivationRound = types.Round(ledger.PreviousCommitteeActivationRound)
	return c
}

// validatorsOf is the validators of a network definition active on a
// partition, as the executor bridge reports them.
func validatorsOf(def *protocol.NetworkDefinition, partition string) []adapter.ValidatorInfo {
	var out []adapter.ValidatorInfo
	for _, v := range def.Validators {
		if !v.IsActiveOn(partition) || len(v.PublicKey) != 32 {
			continue
		}
		var key [32]byte
		copy(key[:], v.PublicKey)
		out = append(out, adapter.ValidatorInfo{PublicKey: key, Stake: 1, Active: true})
	}
	return out
}

// committeeValidators converts the bridge's validators to committee members.
func committeeValidators(validators []adapter.ValidatorInfo) []types.ValidatorInfo {
	out := make([]types.ValidatorInfo, len(validators))
	for i, v := range validators {
		key := v.PublicKey
		out[i] = types.ValidatorInfo{PublicKey: key[:], Stake: v.Stake}
	}
	return out
}

// activationRound is the round from which the state in the store says its
// committee orders, and the version before it; zero for a store that holds
// no ledger yet.
func (s *Service) activationRound() (types.Round, uint64) {
	var ledger *protocol.SystemLedger
	err := s.config.Database.View(func(batch *database.Batch) error {
		return batch.Account(protocol.PartitionUrl(s.config.Partition.ID).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	})
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			s.logger.Error("Reading the committee's activation round", "partition", s.config.Partition.ID, "error", err)
		}
		return 0, 0
	}
	return types.Round(ledger.CommitteeActivationRound), ledger.PreviousNetworkVersion
}

// adoptCommittee schedules the committee a state names, if it is newer than
// the one the node has scheduled last. prev is the version before it: the
// node then knows every committee from its last one on only if prev is that
// committee's epoch. A state that records no activation round made its
// change before v2-canaveral, when a change took effect at once: its
// committee replaces the schedule. It reports whether the node now has the
// state's committee scheduled.
func (s *Service) adoptCommittee(c *types.Committee, prev uint64) bool {
	last := s.node.Committees().Latest()
	switch {
	case c.Epoch == last.Epoch:
		return true
	case c.Epoch < last.Epoch:
		return false
	case c.ActivationRound == 0:
		if err := s.node.ReplaceCommittee(c); err != nil {
			s.logger.Error("Replacing the committee with the state's", "partition", s.config.Partition.ID,
				"epoch", c.Epoch, "error", err)
			return false
		}
		s.mu.Lock()
		s.committee = c
		s.mu.Unlock()
		return true
	case prev != last.Epoch:
		return false
	}
	if err := s.node.UpdateCommittee(c); err != nil {
		s.logger.Error("Scheduling the state's committee", "partition", s.config.Partition.ID,
			"epoch", c.Epoch, "activationRound", c.ActivationRound, "error", err)
		return false
	}
	s.mu.Lock()
	s.committee = c
	s.mu.Unlock()
	return true
}

// resumeCommittee schedules the committee the executor's state names on a
// node that restored its checkpoint, and then holds the checkpoint's state
// (Node.StateAt), in the order adoptPulledState takes them. The checkpoint of
// the block that carried a committee change was written before that block
// was produced, so the change is in the state and not in the checkpoint's
// committees, and it activates exactly at the round limit the checkpoint's
// state sets: scheduled after that limit, it would be refused (#4593). A
// committee the node cannot place stops the start: ordering on would order
// rounds with a committee its peers do not.
func (s *Service) resumeCommittee(c *types.Committee, prev uint64) error {
	if !s.adoptCommittee(c, prev) {
		return errors.Conflict.WithFormat("%s: the executor's state names the committee of epoch %d after epoch %d, and the consensus checkpoint leads to epoch %d",
			s.config.Partition.ID, c.Epoch, prev, s.node.Committees().Latest().Epoch)
	}
	s.node.StateAt(s.lastLeaderRound)
	return nil
}

// adoptPulledState makes what a pulled state at leader round round says about
// the committee this node's: the committee it names, and that this node now
// holds a state at that round (Node.StateAt), so it orders up to the round
// limit above it. A state that names a committee this node cannot place --
// more than one change after the last committee it knows -- is refused
// (Conflict): the node cannot know the committee of the rounds between.
// StageThrough then drops the node's restored position (dropPosition).
func (s *Service) adoptPulledState(round types.Round) error {
	c, prev, _, err := s.stateCommittee()
	if err != nil {
		return errors.NotReady.Wrap(err)
	}
	if !s.adoptCommittee(c, prev) {
		last := s.node.Committees().Latest()
		return errors.Conflict.WithFormat("%s: the state at round %d names the committee of epoch %d after epoch %d, and this node's last committee is of epoch %d",
			s.config.Partition.ID, round, c.Epoch, prev, last.Epoch)
	}
	s.node.StateAt(round)
	return nil
}

// dropPosition gives up the consensus position this node restored from its
// checkpoint, when a pulled state at leader round round names a committee it
// cannot place (#4593): it was down across more than one committee change,
// and does not know the committee of the rounds between its last one and the
// state's. It joins from here as a node with no checkpoint does, seeded below
// the state (seedCommittee, seed). Everything it ordered from its checkpoint
// is below the first change it missed, so its buffer stands; the groups in
// it are at or below the state and are counted executed at the handoff. The
// checkpoint files are replaced by the first block it produces after the
// handoff. It is called with the lock held.
func (s *Service) dropPosition(round types.Round) {
	s.awaitingSeed = true
	s.logger.Info("The pulled state names a committee this node cannot place after its own: dropping the checkpoint's position and joining as a node with no checkpoint",
		"partition", s.config.Partition.ID, "stateRound", round, "lastCommittee", s.node.Committee().Epoch,
		"lastCommit", s.node.LastCommitRound())
}
