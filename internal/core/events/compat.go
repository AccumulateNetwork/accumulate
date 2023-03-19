// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package events

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/events"
)

type Bus = events.Bus
type Event = events.Event
type DidCommitBlock = events.DidCommitBlock
type DidSaveSnapshot = events.DidSaveSnapshot
type WillChangeGlobals = events.WillChangeGlobals
type FatalError = events.FatalError

func NewBus(logger log.Logger) *Bus                    { return events.NewBus(logger) }
func SubscribeSync[T Event](b *Bus, sub func(T) error) { events.SubscribeSync(b, sub) }
func SubscribeAsync[T Event](b *Bus, sub func(T))      { events.SubscribeAsync(b, sub) }
