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

// reloadGlobals makes the executor's globals the ones the store holds, and
// reports which parts differed. A node that joins pulls the state it
// executes from, the network definition, oracle, globals, routing and
// executor versions among it, and the executor loaded its globals from what
// the store held when it started -- genesis, for a fresh node -- and reloads
// them only when a block it executes writes them. Executing the first block
// after the handoff with the old ones executes it differently from the peers
// (#4442, run 20260928T051402Z).
func (x *Executor) reloadGlobals(batch *database.Batch) ([]string, error) {
	fresh := new(core.GlobalValues)
	err := fresh.Load(x.Describe.NodeUrl(), func(account *url.URL, target interface{}) error {
		return batch.Account(account).Main().GetAs(target)
	})
	switch {
	case errors.Is(err, errors.NotFound):
		// A store with no network accounts has no globals to reload.
		return nil, nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load globals: %w", err)
	}
	cur := x.globals()
	var differ []string
	if cur != nil {
		differ = globalsDiffer(&cur.Active, fresh)
		if len(differ) == 0 && cur.Active.Equal(&cur.Pending) {
			return nil, nil
		}
	}
	old := new(core.GlobalValues)
	if cur != nil {
		old = cur.Active.Copy()
	}
	x.globalsPtr.Store(&Globals{Active: *fresh.Copy(), Pending: *fresh.Copy()})
	err = x.EventBus.Publish(events.WillChangeGlobals{New: fresh.Copy(), Old: old})
	if err != nil {
		return differ, errors.UnknownError.WithFormat("publish globals update: %w", err)
	}
	return differ, nil
}

// globalsDiffer names the parts of two sets of globals that differ.
func globalsDiffer(a, b *core.GlobalValues) []string {
	var out []string
	part := func(name string, x, y interface{ MarshalBinary() ([]byte, error) }, nx, ny bool) {
		if nx || ny {
			if nx != ny {
				out = append(out, name)
			}
			return
		}
		bx, _ := x.MarshalBinary()
		by, _ := y.MarshalBinary()
		if string(bx) != string(by) {
			out = append(out, name)
		}
	}
	part("oracle", a.Oracle, b.Oracle, a.Oracle == nil, b.Oracle == nil)
	part("globals", a.Globals, b.Globals, a.Globals == nil, b.Globals == nil)
	part("network", a.Network, b.Network, a.Network == nil, b.Network == nil)
	part("routing", a.Routing, b.Routing, a.Routing == nil, b.Routing == nil)
	if a.ExecutorVersion != b.ExecutorVersion {
		out = append(out, "executor-version")
	}
	if len(out) == 0 && !a.Equal(b) {
		out = append(out, "bvn-executor-versions")
	}
	return out
}
