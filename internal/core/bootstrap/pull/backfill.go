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
// "One rule for every node": the node ends holding every account's chains and
// entries, not only a root that matches).
//
// It does not touch a chain's head or its open mark set, so it may run while
// the node executes: what it writes lies below the head, where no block
// appends. The entries are the peer's, taken from the first source that
// serves them whole, and they are held to the node's own head -- replayed
// from the first, they and the node's open set must reproduce it -- so a
// source that serves another chain is refused and the next is asked. A chain
// with no entries is left as it is.
func Backfill(ctx context.Context, srcs []Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	if len(srcs) == 0 {
		return errors.BadRequest.With("pull.Backfill: at least one source required")
	}
	if pageSize == 0 {
		pageSize = 256
	}
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
			err := backfillChain(ctx, src, batch, u, meta.Name, meta.Type, c.Inner(), pageSize)
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

// backfillChain writes the entries of one chain below its open mark set.
func backfillChain(ctx context.Context, src Source, batch *database.Batch, u *url.URL, name string, typ merkle.ChainType, chain *database.MerkleManager, pageSize uint64) error {
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

	sub := batch.Begin(true)
	defer sub.Discard()
	var bodies *messages
	if carriesMessages(&api.ChainRecord{Name: name, Type: typ}) {
		bodies = newMessages(ctx, src, sub)
	}
	// Every entry the node's head counts, so the messages behind the open
	// set come too; the entries of the open set must be the node's own.
	all, msgs, err := chainEntriesWith(ctx, src, u, name, 0, uint64(head.Count), pageSize, bodies)
	if err != nil {
		return err
	}
	entries := all[:boundary]
	for i, h := range open {
		if string(all[boundary+int64(i)]) != string(h) {
			return errors.Conflict.WithFormat("the entry served at %d is not the node's", boundary+int64(i))
		}
	}

	// Replayed from the first, the entries and the node's own open set must
	// reproduce its head: they are then the chain the node holds, whoever
	// served them.
	st := new(merkle.State)
	markMask, markFreq := chain.MarkMask(), chain.MarkFreq()
	type mark struct {
		index int64
		state *merkle.State
	}
	var marks []mark
	add := func(h []byte) {
		st.AddEntry(h)
		if st.Count&markMask == 0 {
			m := st.Copy()
			marks = append(marks, mark{st.Count - 1, m})
			st.HashList = st.HashList[:0]
		}
	}
	for _, h := range entries {
		add(h)
	}
	below := st.Copy()
	below.HashList = nil
	if !merkle.VerifyAgainstHead(below, head, open) {
		return errors.Conflict.WithFormat("the entries served below %d do not reproduce the node's head", boundary)
	}
	if int64(len(open)) == markFreq && head.Count&markMask == 0 {
		// The open set closes a mark point at the head itself.
		for _, h := range open {
			add(h)
		}
	}

	inner := sub.Account(u)
	c, err := inner.ChainByName(name)
	if err != nil {
		return err
	}
	dst := c.Inner()
	for i, h := range all {
		if bodies != nil {
			if err := bodies.took(u, name, uint64(i), h, msgs[i]); err != nil {
				return err
			}
		}
		if int64(i) >= boundary {
			continue
		}
		if err := dst.Element(uint64(i)).Put(h); err != nil {
			return fmt.Errorf("put element %d: %w", i, err)
		}
		// The index of a hash names where it was last written; one the chain
		// already indexes is left to what the node wrote since.
		if _, err := dst.ElementIndex(h).Get(); errors.Is(err, errors.NotFound) {
			if err := dst.ElementIndex(h).Put(uint64(i)); err != nil {
				return fmt.Errorf("put element index %d: %w", i, err)
			}
		}
	}
	for _, m := range marks {
		if err := dst.States(uint64(m.index)).Put(m.state); err != nil {
			return fmt.Errorf("put mark point %d: %w", m.index, err)
		}
	}
	if err := sub.Commit(); err != nil {
		return err
	}
	return bodies.store(batch)
}
