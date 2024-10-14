// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
)

// A Module is a component of a consensus network.
type Module interface {
	// Receive processes messages, potentially mutating the state of the module
	// and returning messages to broadcast.
	Receive(...Message) ([]Message, error)
}

// A Hub distributes messages to modules.
type Hub interface {
	Register(Module)
	Unregister(Module)
	Send(...Message) error
	With(...Module) Hub
}

type Recorder interface {
	DidInit(snapshot ioutil.SectionReader) error
	DidReceiveMessages([]Message) error
	DidCommitBlock(state execute.BlockState) error
}

// A ConsensusError is produced when nodes produce conflicting results.
type ConsensusError[V any] struct {
	Message      string
	Mine, Theirs V
}

func (e *ConsensusError[V]) Error() string {
	return e.Message
}
