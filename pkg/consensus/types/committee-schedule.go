// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"errors"
	"fmt"
	"sync"
)

// CommitteeActivationMargin is how many rounds after the leader round of the
// block that carries a committee change the new committee takes over
// (consensus spec, "Committee changes"). A block carrying a change is
// committed at leader round L on every node, so L + CommitteeActivationMargin
// is fixed by the committed sequence, not by when a node executes the block.
//
// A node orders, authors and votes only up to this many rounds above the
// leader round of the newest state it holds (Node.ReportExecuted), so it has
// executed every block that could schedule a change for a round before it
// takes part in that round. The margin is therefore also how far consensus
// may run ahead of execution before it waits. 256 rounds is 128 blocks: far
// above the eight blocks at which a lagging executor stops user work
// (MaxExecutionLag) and the 75-block lags seen under load (#4517), so the
// wait is not met in normal running, and far below the DAG's 2,000 rounds of
// retention, so the rounds a waiting node has not yet ordered are still in
// its DAG and its peers'. It is even, so a leader round and the round that
// votes for it always belong to one committee.
const CommitteeActivationMargin Round = 256

// CommitteeSchedule is a partition's committees by round: each orders and
// verifies the rounds from its ActivationRound up to the next one's, and the
// first also every round below. Ordering, leader election and certificate
// verification take the committee of the round in question, so every node
// uses the same committee for a round whenever it learned of the change.
type CommitteeSchedule struct {
	mu         sync.RWMutex
	committees []*Committee
}

// NewCommitteeSchedule is a schedule of one committee.
func NewCommitteeSchedule(c *Committee) *CommitteeSchedule {
	return &CommitteeSchedule{committees: []*Committee{c}}
}

// At is the committee of the given round.
func (s *CommitteeSchedule) At(round Round) *Committee {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.committees) - 1; i > 0; i-- {
		if s.committees[i].ActivationRound <= round {
			return s.committees[i]
		}
	}
	return s.committees[0]
}

// Latest is the committee scheduled last.
func (s *CommitteeSchedule) Latest() *Committee {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.committees[len(s.committees)-1]
}

// Schedule adds a committee from its ActivationRound. It must be of a later
// epoch and activate at a later round than the committee scheduled last;
// scheduling that committee again is no change.
func (s *CommitteeSchedule) Schedule(c *Committee) error {
	if c == nil {
		return errors.New("committee is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	last := s.committees[len(s.committees)-1]
	switch {
	case c.Epoch == last.Epoch && c.ActivationRound == last.ActivationRound:
		return nil
	case c.Epoch <= last.Epoch:
		return fmt.Errorf("committee of epoch %d is not after epoch %d", c.Epoch, last.Epoch)
	case c.ActivationRound <= last.ActivationRound:
		return fmt.Errorf("committee of epoch %d activates at round %d, not after round %d where epoch %d did",
			c.Epoch, c.ActivationRound, last.ActivationRound, last.Epoch)
	}
	s.committees = append(s.committees, c)
	return nil
}

// Replace makes the schedule the given committees, in activation order.
func (s *CommitteeSchedule) Replace(committees []*Committee) error {
	if len(committees) == 0 {
		return errors.New("no committees")
	}
	next := NewCommitteeSchedule(committees[0])
	for _, c := range committees[1:] {
		if err := next.Schedule(c); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.committees = next.committees
	return nil
}

// Committees is the schedule, in activation order.
func (s *CommitteeSchedule) Committees() []*Committee {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*Committee(nil), s.committees...)
}

// Prune drops the committees that order no round at or above floor.
func (s *CommitteeSchedule) Prune(floor Round) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := 0
	for i+1 < len(s.committees) && s.committees[i+1].ActivationRound <= floor {
		i++
	}
	s.committees = s.committees[i:]
}
