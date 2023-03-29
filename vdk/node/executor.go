package node

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
)

//This package will expose some internal capability and is subject to change, use with caution

//expose the bus and events used by a node

type Bus = events.Bus

type Event = events.Event

type DidCommitBlock = events.DidCommitBlock

type DidSaveSnapshot = events.DidSaveSnapshot

type FatalError = events.FatalError

// SubscribeSync will expose the internal subscribe sync
func SubscribeSync[T Event](b *Bus, sub func(T) error) {
	events.SubscribeSync((*events.Bus)(b), sub)
}

func SubscribeSyncDidCommitBlock(b *Bus, sub func(DidCommitBlock) error) {
	events.SubscribeSync[events.DidCommitBlock]((*events.Bus)(b), func(block events.DidCommitBlock) error {
		return sub(DidCommitBlock(block))
	})
}

func SubscribeSyncDidSaveSnapshot(b *Bus, sub func(DidSaveSnapshot) error) {
	events.SubscribeSync[events.DidSaveSnapshot]((*events.Bus)(b), func(snapshot events.DidSaveSnapshot) error {
		return sub(DidSaveSnapshot(snapshot))
	})
}

func SubscribeSyncFatalError(b *Bus, sub func(FatalError) error) {
	events.SubscribeSync[events.FatalError]((*events.Bus)(b), func(fatal events.FatalError) error {
		return sub(FatalError(fatal))
	})
}
