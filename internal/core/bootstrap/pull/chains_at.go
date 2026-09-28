// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	stderrors "errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// TakeChainsAt takes the entries of a served account's chains, with the
// messages behind them, up to the heads the served state names and never past
// them: whole, or by the window of the block the state was read at, as
// FetchFrom's ModeFullSpine and ModeWindow take them from the peer's head now
// (#4442). It writes no head: KeepState writes the state's own.
//
// A join executes from the block its state is complete at, V, and the executor
// reads the recent entries of the trust chain and the windowed accounts, and
// the messages behind them. Taken from the peer's head now they reach past V,
// and an entry held past a head is one a later append is deduplicated
// against (merkle.Chain.AddEntry): the node would then execute V + 1 without
// appending what its peers appended. Entries are immutable, so those below V's
// head are the same whichever block the peer is at.
//
// held says the node took this account's chains earlier in the same pull, to
// an earlier answer's heads: a windowed chain is then brought on from its own
// head, as a whole one is, without searching the peer's index chain for the
// window again -- a search of several calls a chain, which on every answer
// made the records of a loaded partition slower than the partition (#4442).
// A chain that does not continue from its head is taken by its window.
func TakeChainsAt(ctx context.Context, srcs []Source, store Store, st *api.AccountState, block uint64, whole, held bool, window uint64) (int, error) {
	if len(srcs) == 0 {
		return -1, errors.BadRequest.With("pull.TakeChainsAt: at least one source required")
	}
	if st == nil || st.Url == nil || st.Leaf == nil {
		return -1, nil
	}
	var records []*api.ChainRecord
	byName := map[string]*api.ChainRecord{}
	for _, c := range st.Leaf.Chains {
		if c == nil || c.Name == "" {
			continue
		}
		r := &api.ChainRecord{Name: c.Name, Type: c.Type, Count: c.Count, State: c.State}
		records = append(records, r)
		byName[r.Name] = r
	}
	var refusals []error
	for i, src := range srcs {
		err := takeChainsAt(ctx, src, store, st, records, byName, block, whole, held, window)
		if err == nil {
			return i, nil
		}
		if ctx.Err() != nil {
			return -1, errors.UnknownError.Wrap(ctx.Err())
		}
		refusals = append(refusals, fmt.Errorf("%s: %w", sourceName(i, src), err))
	}
	return -1, errors.Conflict.WithFormat("%v: no source served its chains to the heads of block %d: %w", st.Url, block, stderrors.Join(refusals...))
}

func takeChainsAt(ctx context.Context, src Source, store Store, st *api.AccountState, records []*api.ChainRecord, byName map[string]*api.ChainRecord, block uint64, whole, held bool, window uint64) error {
	const pageSize = 256
	u := st.Url
	found := map[string]indexFound{}
	for _, c := range mainFirst(records) {
		var err error
		if whole {
			_, err = streamChain(ctx, src, store, u, c, pageSize, false, false)
			if stderrors.Is(err, errNotThePeers) || stderrors.Is(err, errLongerThanThePeers) {
				_, err = streamChain(ctx, src, store, u, c, pageSize, false, true)
			}
		} else {
			if held {
				_, err = streamChain(ctx, src, store, u, c, pageSize, false, false)
				if err == nil {
					continue
				}
				if !stderrors.Is(err, errNotThePeers) && !stderrors.Is(err, errLongerThanThePeers) {
					return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
				}
			}
			_, err = streamChainWindow(ctx, src, store, u, c, byName, found, pageSize, block, window, false, true)
		}
		if err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
	}
	return nil
}
