package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type Globals struct {
	Active, Pending core.GlobalValues
}

func (x *Executor) loadGlobals(view func(func(batch *database.Batch) error) error) error {
	x.globals = new(Globals)
	err := x.globals.Active.Load(&x.Describe, func(account *url.URL, target interface{}) error {
		return view(func(batch *database.Batch) error {
			return batch.Account(account).GetStateAs(target)
		})
	})
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load globals: %w", err)
	}

	x.globals.Pending = *x.globals.Active.Copy()
	return nil
}
