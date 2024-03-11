// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Module is a component of a consensus network.
type Module interface {
	// Receive processes messages, potentially mutating the state of the module
	// and returning messages to broadcast.
	Receive(...Message) ([]Message, error)
}

// A Message is a message sent between modules.
type Message interface {
	isMsg()
}

// A networkMessage is a message that is specific to a network.
type networkMessage interface {
	Message
	network() string
}

// blockMessages are passed between nodes while executing a block.
type blockMessage interface {
	Message
	isBlkMsg()
	senderID() [32]byte
}

// A Hub distributes messages to modules.
type Hub interface {
	Send(...Message) error
	With(modules ...Module) Hub
}

type Response interface {
	Message
	isResponse()
}

type Submission struct {
	Network  string
	Envelope *messaging.Envelope
	Pretend  bool
}

var _ networkMessage = (*Submission)(nil)

func (_ *Submission) isMsg()          {}
func (s *Submission) network() string { return s.Network }

type SubmissionResponse struct {
	Results []*protocol.TransactionStatus
}

var _ Response = (*SubmissionResponse)(nil)

func (*SubmissionResponse) isMsg()      {}
func (*SubmissionResponse) isResponse() {}

func (s *SubmissionResponse) Equal(r *SubmissionResponse) bool {
	if len(s.Results) != len(r.Results) {
		return false
	}

	for i := range s.Results {
		if !s.Results[i].Equal(r.Results[i]) {
			return false
		}
	}

	return true
}

// StartBlock starts a block.
type StartBlock struct{}

var _ Message = (*StartBlock)(nil)

func (*StartBlock) isMsg() {}

type ExecutedBlock struct {
	Network string
	Node    [32]byte
}

var _ Response = (*ExecutedBlock)(nil)

func (*ExecutedBlock) isMsg()      {}
func (*ExecutedBlock) isResponse() {}

type LeaderProposal struct {
	Leader [32]byte
}

type BlockProposal struct {
	LeaderProposal
	Index     uint64
	Time      time.Time
	Envelopes []*messaging.Envelope
}

func (b *BlockProposal) Equal(c *BlockProposal) bool {
	if b.Index != c.Index ||
		!b.Time.Equal(c.Time) ||
		len(b.Envelopes) != len(c.Envelopes) {
		return false
	}

	for i := range b.Envelopes {
		if !b.Envelopes[i].Equal(c.Envelopes[i]) {
			return false
		}
	}
	return true
}

type BlockResults struct {
	MessageResults   []*protocol.TransactionStatus
	ValidatorUpdates []*execute.ValidatorUpdate
}

func (b *BlockResults) Equal(c *BlockResults) bool {
	if len(b.MessageResults) != len(c.MessageResults) ||
		len(b.ValidatorUpdates) != len(c.ValidatorUpdates) {
		return false
	}

	for i := range b.MessageResults {
		if !b.MessageResults[i].Equal(c.MessageResults[i]) {
			return false
		}
	}

	for i := range b.ValidatorUpdates {
		if !b.ValidatorUpdates[i].Equal(c.ValidatorUpdates[i]) {
			return false
		}
	}

	return true
}

type CommitResult struct {
	Hash [32]byte
}

// A ConsensusError is produced when nodes produce conflicting results.
type ConsensusError[V any] struct {
	Message      string
	Mine, Theirs V
}

func (e *ConsensusError[V]) Error() string {
	return e.Message
}
