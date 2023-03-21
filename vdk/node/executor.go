package node

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
)

//This package will expose some internal capability and is subject to change, use with caution

type Executor interface {
	execute.Executor
}

type BlockParams struct {
	execute.BlockParams
}

type ValidatorUpdate struct {
	execute.ValidatorUpdate
}

type Block interface {
	execute.Block
}

//expose the bus and events used by a node

type Bus struct {
	Bus events.Bus
}

type Event interface {
	events.Event
}

type DidCommitBlock struct {
	DidCommit events.DidCommitBlock
}

type DidSaveSnapshot struct {
	DidSave events.DidSaveSnapshot
}

type FatalError struct {
	Fatal events.FatalError
}

// SubscribeSync will expose the internal subscribe sync
func SubscribeSync[T Event](b *Bus, sub func(T) error) {
	events.SubscribeSync(&b.Bus, sub)
}
