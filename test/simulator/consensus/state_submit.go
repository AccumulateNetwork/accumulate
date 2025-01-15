// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

type submitState state[submitState]

type submitMessage interface {
	Message
	envelope() *messaging.Envelope
}

func (n *Node) processSubmission(sub *SubmitEnvelope) (submitState, []Message, error) {
	result, err := n.check(context.Background(), sub.Envelope)
	if err != nil {
		return nil, nil, err
	}

	if sub.Pretend {
		return nil, []Message{
			&EnvelopeSubmitted{
				Results: result,
			},
		}, nil
	}

	m := new(didAcceptSubmission)
	m.Node = n
	m.s.baseNodeMessage = n.newMsg()
	m.s.env = sub.Envelope
	m.s.result.Results = result
	m.votes = n.newVotes()

	s, out, err := executeState[submitState](n.context, m, nil)
	out = append(out, &m.s)
	return s, out, err
}

type didAcceptSubmission struct {
	*Node
	s     acceptedSubmission
	votes votes
}

func (m *acceptedSubmission) envelope() *messaging.Envelope { return m.env }

func (n *didAcceptSubmission) execute(msg Message) (submitState, []Message, error) {
	switch msg := msg.(type) {
	case *acceptedSubmission:
		// Verify the result matches
		if !msg.result.Equal(&n.s.result) {
			return n, nil, &ConsensusError[EnvelopeSubmitted]{
				Message: "conflicting leader proposal",
				Mine:    n.s.result,
				Theirs:  msg.result,
			}
		}

		// Add the vote
		n.votes.add(msg.SenderID())
	}

	if !n.votes.reachedThreshold() {
		return n, nil, nil
	}

	n.mempool.Add(n.s.env)
	return nil, []Message{&n.s.result}, nil
}
