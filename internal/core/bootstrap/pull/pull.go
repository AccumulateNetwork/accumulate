// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Account pulls the complete state surface for u from src and writes
// it into batch. After Account returns, batch.Account(u).Hash() must
// match the source's leaf hash for u.
//
// Order of operations is significant. Main goes first because some
// downstream paths (chain initialization, secondary state) read it.
// Chains follow Main. Directory and Pending populate independently.
func Account(ctx context.Context, src Source, batch *database.Batch, u *url.URL) error {
	// 1. Main state.
	main, err := src.Main(ctx, u)
	if err != nil {
		return fmt.Errorf("pull main %s: %w", u, err)
	}
	if main != nil {
		if err := batch.Account(u).Main().Put(main); err != nil {
			return fmt.Errorf("store main %s: %w", u, err)
		}
	}

	// 2. Chains. Enumerate, then for each, replay entries oldest-first.
	chainNames, err := src.ChainNames(ctx, u)
	if err != nil {
		return fmt.Errorf("list chains %s: %w", u, err)
	}
	for _, name := range chainNames {
		entries, err := src.ChainEntries(ctx, u, name)
		if err != nil {
			return fmt.Errorf("pull chain %s/%s: %w", u, name, err)
		}
		c, err := batch.Account(u).ChainByName(name)
		if err != nil {
			return fmt.Errorf("local chain %s/%s: %w", u, name, err)
		}
		for i, e := range entries {
			if err := c.Inner().AddEntry(e, false); err != nil {
				return fmt.Errorf("add entry %d to %s/%s: %w", i, u, name, err)
			}
		}
	}

	// 3. Directory.
	dirs, err := src.DirectoryUrls(ctx, u)
	if err != nil {
		return fmt.Errorf("pull directory %s: %w", u, err)
	}
	for _, d := range dirs {
		if err := batch.Account(u).Directory().Add(d); err != nil {
			return fmt.Errorf("add directory entry %s -> %s: %w", u, d, err)
		}
	}

	// 4. Pending.
	pending, err := src.PendingIDs(ctx, u)
	if err != nil {
		return fmt.Errorf("pull pending %s: %w", u, err)
	}
	for _, txid := range pending {
		if err := batch.Account(u).Pending().Add(txid); err != nil {
			return fmt.Errorf("add pending %s -> %s: %w", u, txid, err)
		}
	}

	return nil
}
