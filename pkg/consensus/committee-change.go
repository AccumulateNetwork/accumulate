// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"errors"
	"fmt"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// UpdateCommittee schedules a committee from its ActivationRound
// (consensus spec, "Committee changes"; #4593): the rounds below it are
// ordered and verified with the committee before it on every node, whenever
// that node executed the change. The round must be above every round this
// node may already have ordered, authored or voted at.
func (n *Node) UpdateCommittee(committee *types.Committee) error {
	if committee == nil {
		return errors.New("committee is nil")
	}
	if limit, ok := n.RoundLimit(); ok && committee.ActivationRound <= limit {
		return fmt.Errorf("committee of epoch %d activates at round %d, at or below round %d which this node may already have ordered",
			committee.Epoch, committee.ActivationRound, limit)
	}
	prev := n.committees.Latest()
	if err := n.bullshark.UpdateCommittee(committee); err != nil {
		return err
	}
	slog.Info("Committee change scheduled", "partition", n.config.Partition,
		"epoch", prev.Epoch, "validators", prev.Len(),
		"newEpoch", committee.Epoch, "newValidators", committee.Len(),
		"activationRound", committee.ActivationRound)
	return nil
}

// ReplaceCommittee makes committee the committee of every round, as a change
// made before v2-canaveral takes effect: at once, when the node executes the
// block or takes the state that carries it (#4593).
func (n *Node) ReplaceCommittee(committee *types.Committee) error {
	if committee == nil {
		return errors.New("committee is nil")
	}
	prev := n.committees.Latest()
	if err := n.committees.Replace([]*types.Committee{committee}); err != nil {
		return err
	}
	slog.Info("Committee replaced", "partition", n.config.Partition,
		"epoch", prev.Epoch, "validators", prev.Len(),
		"newEpoch", committee.Epoch, "newValidators", committee.Len())
	return nil
}

// Committees is the committee of each round, as this node orders and
// verifies them.
func (n *Node) Committees() *types.CommitteeSchedule {
	return n.committees
}

// StateAt records that this node holds the committed state of the block
// committed at leader round round: it has executed that block, restored a
// position at it, or holds a pulled state that includes it. Every committee
// change in a block up to there is then scheduled, and none in a later block
// activates at or below round + CommitteeActivationMargin, so the node
// orders, authors and votes up to that round and no further. A lower round
// than one already recorded changes nothing.
func (n *Node) StateAt(round types.Round) {
	for {
		cur := n.stateRound.Load()
		if n.stateKnown.Load() && uint64(round) <= cur {
			return
		}
		if n.stateRound.CompareAndSwap(cur, uint64(round)) {
			break
		}
	}
	n.stateKnown.Store(true)
	select {
	case n.limitRaised <- struct{}{}:
	default:
	}
}

// RoundLimit is the highest round this node may order a leader at, author or
// vote at, and false while it has not been told which state it holds: a node
// that executes nothing learns of no committee change, and orders as it
// always has.
func (n *Node) RoundLimit() (types.Round, bool) {
	if !n.stateKnown.Load() {
		return 0, false
	}
	return types.Round(n.stateRound.Load()) + types.CommitteeActivationMargin, true
}

// beyondLimit reports whether ordering the leader that a certificate of this
// round votes for must wait for the limit to rise, and records that it did.
func (n *Node) beyondLimit(round types.Round) bool {
	limit, ok := n.RoundLimit()
	if !ok || round <= limit+1 {
		return false
	}
	n.deferred.Store(true)
	return true
}

// orderDeferred orders what waited for the limit: the leaders above the last
// commit and at or below the limit, as the certificates already in the DAG
// decide them. Bullshark's order does not depend on when a certificate
// arrived, so ordering them now is ordering them late. It reports false when
// the node is stopping.
func (n *Node) orderDeferred() bool {
	if !n.deferred.Swap(false) {
		return true
	}
	top := n.dag.LatestRound()
	for r := n.bullshark.LastCommitRound() + 1; r <= top; r++ {
		if r%2 == 0 {
			continue
		}
		if n.beyondLimit(r) {
			break
		}
		certs := n.dag.GetRound(r)
		if len(certs) == 0 {
			continue
		}
		if !n.order(certs[0]) {
			return false
		}
	}
	return true
}

// restoreCommittees takes the checkpoint's committees. The committee the node
// was built with is whatever the executor's state names now, which for the
// rounds of the tail may be a committee that had not yet taken over (#4593).
// It does not hold the checkpoint's state (StateAt): the caller does, once it
// has scheduled the committee that state names, since the change in the
// checkpoint's own block activates exactly at the limit that state sets.
func (n *Node) restoreCommittees(cp *persist.Checkpoint) {
	if len(cp.Committees) > 0 {
		committees, err := persist.DecodeCommittees(cp.Committees)
		if err == nil {
			err = n.committees.Replace(committees)
		}
		if err != nil {
			slog.Error("Restoring the checkpoint's committees", "partition", n.config.Partition, "error", err)
		}
	}
}
