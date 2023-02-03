// Copyright 2023 The Accumulate Authors
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
	x.globals = new(Globals)
	err := x.globals.Active.Load(x.Describe.PartitionUrl(), func(account *url.URL, target interface{}) error {
		return view(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	})
	if err != nil {
		return errors.UnknownError.WithFormat("load globals: %w", err)
	}

	// Publish an update
	err = x.EventBus.Publish(events.WillChangeGlobals{
		New: &x.globals.Active,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("publish globals update: %w", err)
	}

	// Make a copy for pending
	x.globals.Pending = *x.globals.Active.Copy()
	return nil
}
