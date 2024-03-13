// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Receive implements [Message.Receive].
func (n *Node) Receive(messages ...Message) ([]Message, error) {
	var allOut []Message

	// Process each message
	for _, msg := range messages {
		// Ignore messages from ourself
		if msg, ok := msg.(nodeMessage); ok &&
			msg.senderID() == n.self.PubKeyHash {
			continue
		}

		// Ignore messages for other networks
		if msg, ok := msg.(networkMessage); ok &&
			msg.network() != n.network {
			continue
		}

		switch msg := msg.(type) {
		case *Submission:
			if n.submitState[msg.Envelope] != nil {
				continue
			}

			// Process submission
			s, out, err := n.processSubmission(msg)
			allOut = append(allOut, out...)
			if err != nil {
				return allOut, err
			}
			if s != nil {
				n.submitState[msg.Envelope] = s
			}
			continue

		case *StartBlock:
			if n.blockState != nil {
				return nil, errors.InternalError.With("attempted to start a block while a block is executing")
			}

			// Start a block
			s, out, err := n.proposeLeader()
			allOut = append(allOut, out...)
			if err != nil {
				return allOut, err
			}
			n.blockState = s
			continue

		case submitMessage:
			s := n.submitState[msg.envelope()]
			if s == nil {
				continue
			}

			s, out, err := executeState(n.context, s, msg)
			allOut = append(allOut, out...)
			if err != nil {
				return allOut, err
			}
			if s == nil {
				delete(n.submitState, msg.envelope())
			} else {
				n.submitState[msg.envelope()] = s
			}
		}

		if n.blockState != nil {
			s, out, err := executeState(n.context, n.blockState, msg)
			allOut = append(allOut, out...)
			if err != nil {
				return allOut, err
			}
			n.blockState = s
		}
	}

	return allOut, nil
}

// nodeMessages are passed between nodes.
type nodeMessage interface {
	Message
	senderID() [32]byte
}

type baseNodeMessage struct {
	sender *Node
}

var _ networkMessage = (*baseNodeMessage)(nil)

func (n *Node) newMsg() baseNodeMessage {
	return baseNodeMessage{n}
}

func (_ *baseNodeMessage) isMsg()             {}
func (m *baseNodeMessage) senderID() [32]byte { return m.sender.self.PubKeyHash }
func (m *baseNodeMessage) network() string    { return m.sender.network }

// Make lint shut up
func init() {
	if false {
		_, _, _ = (*didAcceptSubmission).execute(nil, nil)

		_, _, _ = (*didProposeLeader).execute(nil, nil)
		_, _, _ = (*didProposeBlock).execute(nil, nil)
		_, _, _ = (*didFinalizeBlock).execute(nil, nil)
		_, _, _ = (*didCommitBlock).execute(nil, nil)
	}
}
