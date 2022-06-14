package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type Globals struct {
	Active, Pending core.GlobalValues
}

func (x *Executor) loadGlobals(view func(func(batch *database.Batch) error) error) error {
	// Load from the database
	x.globals = new(Globals)
	err := x.globals.Active.Load(&x.Describe, func(account *url.URL, target interface{}) error {
		return view(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	})
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load globals: %w", err)
	}

	// Publish an update
	err = x.EventBus.Publish(events.DidChangeGlobals{
		Values: &x.globals.Active,
	})
	if err != nil {
		return errors.Format(errors.StatusUnknown, "publish globals update: %w", err)
	}

	// Make a copy for pending
	x.globals.Pending = *x.globals.Active.Copy()
	return nil
}
