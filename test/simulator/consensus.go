// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"crypto/sha256"
	"runtime"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

func (p *Partition) Submit(envelope *messaging.Envelope, pretend bool) ([]*protocol.TransactionStatus, error) {
	// Apply the hook
	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if p.applySubmitHook(messages) {
		st := make([]*protocol.TransactionStatus, len(messages))
		for i, msg := range messages {
			st[i] = new(protocol.TransactionStatus)
			st[i].TxID = msg.ID()
			st[i].Code = errors.NotAllowed
			st[i].Error = errors.NotAllowed.With("dropped")
		}
		return st, nil
	}

	var resp consensus.Capture[*consensus.EnvelopeSubmitted]
	err = p.sim.hub.With(&resp).Send(&consensus.SubmitEnvelope{
		Network:  p.ID,
		Envelope: envelope,
		Pretend:  pretend,
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if len(resp) == 0 {
		return nil, errors.FatalError.With("no response")
	}
	return resp[0].Results, nil
}

// Step executes a single simulator step
func (s *Simulator) Step() error {
	// TODO Care about s.deterministic

	// A partition below its quorum sits the step out: its nodes hold the
	// block they cannot complete, and starting another would be an error.
	// They are taken off the hub for the step and put back after (#4512).
	var sitOut []*Node
	for _, p := range s.partitions {
		if !p.belowQuorum() {
			continue
		}
		for _, n := range p.nodes {
			if !n.stopped {
				s.hub.Unregister(n.consensus)
				sitOut = append(sitOut, n)
			}
		}
	}
	defer func() {
		for _, n := range sitOut {
			s.hub.Register(n.consensus)
		}
	}()

	// Execute a block
	var resp consensus.Capture[*consensus.ExecutedBlock]
	err := s.hub.With(&resp).Send(&consensus.StartBlock{})
	if err != nil {
		return err
	}

	// Map which networks/nodes have completed the block
	done := map[string]map[[32]byte]bool{}
	for _, p := range s.partIDs {
		done[p] = map[[32]byte]bool{}
	}
	for _, r := range resp {
		done[r.Network][r.Node] = true
	}

	// Verify every node completed -- of every partition that can complete a
	// block. A partition with fewer of its validators running than the
	// consensus threshold cannot, and does not change state: that is the
	// halt the executor spec decides for a partition that lost its quorum
	// ("A partition that loses a quorum of its validators' states"), and the
	// other partitions step on without it (#4512).
	for _, p := range s.partitions {
		if p.belowQuorum() {
			continue
		}
		for _, n := range p.nodes {
			if n.stopped {
				continue
			}
			h := sha256.Sum256(n.privValKey[32:])
			if !done[p.ID][h] {
				panic("block did not complete")
			}
		}
	}

	// Wait for execution to complete
	err = s.tasks.Flush()

	// Give any parallel processes a chance to run
	runtime.Gosched()

	return err
}

// belowQuorum reports whether fewer of the partition's validators are running
// than the consensus needs to complete a block: the simulator's consensus
// completes one when no more than a third of the validators have not voted.
func (p *Partition) belowQuorum() bool {
	var total, running int
	for _, n := range p.nodes {
		if !n.isValidatorOn(p.Type) {
			continue
		}
		total++
		if !n.stopped {
			running++
		}
	}
	return (total-running)*3 > total
}
