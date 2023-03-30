// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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
	events.SubscribeSync(b, sub)
}
