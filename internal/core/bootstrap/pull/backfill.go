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

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Backfill brings the entries of an account the node took by its chain heads
// alone into its store: for every chain the node holds, the entries below the
// open mark set, the mark points that close their sets, and -- on a
// transaction chain -- the message behind every entry, the open set's
// included (executor spec, "Sync",
// "Two mismatches": the node ends holding every account's chains and
// entries, not only a root that matches).
//
// It does not touch a chain's head or its open mark set, so it may run while
// the node executes: what it writes lies below the head, where no block
// appends. The entries are the peer's, taken from the first source that
// serves them whole, and they are held to the node's own head -- replayed
// from the first, they and the node's open set must reproduce it -- so a
// source that serves another chain is refused and the next is asked. A chain
// with no entries is left as it is.
//
// The entries come a page at a time and each page is written to store and
// dropped before the next is asked for (#4446), so memory is bounded by the
// page and not by the chain. They are held to the head once the last is
// written: a source refused there has written entries the next source
// overwrites, position by position (docs/spec/DIFFERENCES.md).
func Backfill(ctx context.Context, srcs []Source, store Store, u *url.URL, pageSize uint64) error {
	if len(srcs) == 0 {
		return errors.BadRequest.With("pull.Backfill: at least one source required")
	}
	if pageSize == 0 {
		pageSize = 256
	}
	batch := store.Begin(false)
	defer batch.Discard()
	chains, err := batch.Account(u).Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("%v: load the chain index: %w", u, err)
	}
	for _, meta := range chains {
		c, err := batch.Account(u).ChainByName(meta.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("%v: chain %s: %w", u, meta.Name, err)
		}
		var refusals []error
		done := false
		for i, src := range srcs {
			err := backfillChain(ctx, src, store, u, meta.Name, meta.Type, c.Inner(), pageSize)
			if err == nil {
				done = true
				break
			}
			if ctx.Err() != nil {
				return errors.UnknownError.Wrap(ctx.Err())
			}
			refusals = append(refusals, fmt.Errorf("%s: %w", sourceName(i, src), err))
		}
		if !done {
			return errors.Conflict.WithFormat("%v: chain %s: no source served its entries: %w", u, meta.Name, stderrors.Join(refusals...))
		}
	}
	return nil
}

// backfillChain writes the entries of one chain below its head, a page at a
// time, and holds them to the head.
func backfillChain(ctx context.Context, src Source, store Store, u *url.URL, name string, typ merkle.ChainType, chain *database.MerkleManager, pageSize uint64) error {
	head, err := chain.Head().Get()
	if err != nil {
		return fmt.Errorf("load the head: %w", err)
	}
	if head.Count == 0 {
		return nil
	}
	boundary := int64(merkle.BoundaryFor(head.Count, chain.MarkFreq()))
	open, err := chain.OpenSet(head)
	if err != nil {
		return fmt.Errorf("load the open set: %w", err)
	}

	// Every entry the node's head counts, so the messages behind the open
	// set come too; the entries of the open set must be the node's own.
	// Replayed from the first, the entries must reproduce the node's head:
	// they are then the chain the node holds, whoever served them.
	st := new(merkle.State)
	bodies := carriesMessages(&api.ChainRecord{Name: name, Type: typ})
	err = streamEntries(ctx, src, store, u, name, 0, uint64(head.Count), pageSize, bodies, func(page *database.Batch, _ *messages, index uint64, h []byte) error {
		if i := int64(index) - boundary; i >= 0 && string(open[i]) != string(h) {
			return errors.Conflict.WithFormat("the entry served at %d is not the node's", index)
		}
		c, err := page.Account(u).ChainByName(name)
		if err != nil {
			return err
		}
		// The index of a hash names where it was last written; one the
		// chain already indexes is left to what the node wrote since.
		return c.Inner().PutBelow(st, h, true)
	})
	if err != nil {
		return err
	}
	if st.Count != head.Count || string(st.Anchor()) != string(head.Anchor()) {
		return errors.Conflict.WithFormat("the entries served below %d do not reproduce the node's head", boundary)
	}
	return nil
}
