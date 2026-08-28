// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Globals struct {
	Active, Pending core.GlobalValues
}

func (x *Executor) loadGlobals(view func(func(batch *database.Batch) error) error) error {
	// Load from the database
	g := new(Globals)
	x.globalsPtr.Store(g)
	err := g.Active.Load(x.Describe.NodeUrl(), func(account *url.URL, target interface{}) error {
		return view(func(batch *database.Batch) error {
			return batch.Account(account).Main().GetAs(target)
		})
	})
	if err != nil {
		return errors.UnknownError.WithFormat("load globals: %w", err)
	}

	// Publish an update
	// A snapshot, not a pointer into the executor's state — see the note at
	// the block-end publish (#4170).
	err = x.EventBus.Publish(events.WillChangeGlobals{
		New: x.globals().Active.Copy(),
	})
	if err != nil {
		return errors.UnknownError.WithFormat("publish globals update: %w", err)
	}

	// Make a copy for pending. Safe to write through the pointer here: this
	// runs during load, before the executor is serving anything.
	g.Pending = *g.Active.Copy()
	return nil
}
