// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.
package consensus

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package consensus --out enums_gen.go enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package consensus messages.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package consensus --language go-union --out unions_gen.go messages.yml

type messageType int

// A Message is a message sent between modules.
type Message interface {
	encoding.UnionValue
	isMsg()
	Type() messageType
}

// A NetworkMessage is a message that is specific to a network.
type NetworkMessage interface {
	Message
	PartitionID() string
}

// nodeMessages are passed between nodes.
type NodeMessage interface {
	NetworkMessage
	SenderID() [32]byte
}

func (*baseNodeMessage) isMsg()   {}
func (*SubmitEnvelope) isMsg()    {}
func (*EnvelopeSubmitted) isMsg() {}
func (*StartBlock) isMsg()        {}
func (*ExecutedBlock) isMsg()     {}

func (m *baseNodeMessage) PartitionID() string { return m.Network }
func (s *SubmitEnvelope) PartitionID() string  { return s.Network }

func (m *baseNodeMessage) SenderID() [32]byte { return m.PubKeyHash }

var _ NodeMessage = (*proposeLeader)(nil)
var _ NodeMessage = (*proposeBlock)(nil)
var _ NodeMessage = (*acceptBlockProposal)(nil)
var _ NodeMessage = (*finalizedBlock)(nil)
var _ NodeMessage = (*committedBlock)(nil)
var _ NetworkMessage = (*SubmitEnvelope)(nil)
