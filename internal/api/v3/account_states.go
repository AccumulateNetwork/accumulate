// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"encoding/binary"
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// accountStatesBudget is how many bytes of marshalled account states one page
// of ChangedAccountStates or AccountStates carries. An account larger than it
// is answered in parts. A variable so a test can page a small store.
var accountStatesBudget = 1 << 20

// maxRecordSpan is how many blocks of records one ChangedAccountStates answer
// covers: T is at most After plus this. Every page of the answer reads the
// records of the span again -- each page is a view of its own, and none is
// held across pages (#4446) -- so the span bounds what one page costs.
const maxRecordSpan = 1024

// maxAccountStatesAsked is how many accounts one AccountStates query may name:
// a walk page's worth and more (executor.md, "Sync", §3).
const maxAccountStatesAsked = 4096

// queryChangedAccountStates answers the sync state of every account the block
// ledger's records of the blocks after After name, through T, from this one
// view of the store (executor.md, "Sync", §3; #4453). T is this node's block B
// on a first page, capped at maxRecordSpan blocks after After, and the cursor
// carries it so every page of the answer covers the same records: an account
// the records name many times is in the union once, and in the answer once.
func (s *Querier) queryChangedAccountStates(batch *database.Batch, scope *url.URL, query *api.ChangedAccountStatesQuery) (*api.AccountStatesRecord, error) {
	if scope == nil || !s.partition.URL.Equal(scope) {
		return nil, errors.BadRequest.WithFormat(
			"the accounts a partition's records name are the partition's: this node serves %v, and the query is scoped to %v",
			s.partition.URL, scope)
	}
	block, err := s.viewBlock(batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if block < query.After {
		// Behind the asker: its records are records this node has not
		// written yet. Another peer is asked.
		return nil, errors.NotReady.WithFormat("this node stands at block %d, before block %d", block, query.After)
	}

	cur, err := decodeStatesCursor(query.Cursor)
	if err != nil {
		return nil, errors.BadRequest.Wrap(err)
	}
	through := cur.through
	if len(query.Cursor) == 0 {
		through = min(block, query.After+maxRecordSpan)
	}
	if through < query.After || through > block {
		return nil, errors.BadRequest.WithFormat("the cursor names block %d, not between %d and this node's block %d", through, query.After, block)
	}

	named, err := s.namedBetween(batch, query.After, through)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r, err := s.statesPage(batch, named, cur)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r.Through, r.Block = through, block
	if len(r.Cursor) > 0 {
		c, _ := decodeStatesCursor(r.Cursor)
		c.through = through
		r.Cursor = c.encode()
	}
	return r, nil
}

// queryAccountStates answers the sync state of the named accounts, from this
// one view of the store, stamped with its block (executor.md, "Sync", §3).
func (s *Querier) queryAccountStates(batch *database.Batch, scope *url.URL, query *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	if scope == nil || !s.partition.URL.Equal(scope) {
		return nil, errors.BadRequest.WithFormat(
			"account states are asked of the partition: this node serves %v, and the query is scoped to %v",
			s.partition.URL, scope)
	}
	if len(query.Accounts) > maxAccountStatesAsked {
		return nil, errors.BadRequest.WithFormat("%d accounts asked for, at most %d", len(query.Accounts), maxAccountStatesAsked)
	}
	for _, u := range query.Accounts {
		if u == nil {
			return nil, errors.BadRequest.With("an account asked for is empty")
		}
	}
	block, err := s.viewBlock(batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	cur, err := decodeStatesCursor(query.Cursor)
	if err != nil {
		return nil, errors.BadRequest.Wrap(err)
	}
	r, err := s.statesPage(batch, query.Accounts, cur)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r.Block = block
	return r, nil
}

// viewBlock is the block this view of the store is at: the partition ledger's
// index, written in the same commit as everything the block changed. A store
// with no ledger has executed nothing, and is at block 0.
func (s *Querier) viewBlock(batch *database.Batch) (uint64, error) {
	var ledger *protocol.SystemLedger
	err := batch.Account(s.partition.Ledger()).Main().GetAs(&ledger)
	switch {
	case err == nil:
		return ledger.Index, nil
	case errors.Is(err, errors.NotFound):
		return 0, nil
	default:
		return 0, errors.UnknownError.WithFormat("load the ledger: %w", err)
	}
}

// namedBetween is the union of the accounts the block-ledger records of the
// blocks after after through through name, once each, in a fixed order, so
// every page of one answer walks the same list. A block with no record changed
// nothing. The placeholder acc://unknown names no account and is left out.
func (s *Querier) namedBetween(batch *database.Batch, after, through uint64) ([]*url.URL, error) {
	ledger := batch.Account(s.partition.Ledger())
	unknown := protocol.UnknownUrl()
	seen := map[string]*url.URL{}
	for n := after + 1; n <= through; n++ {
		_, entries, err := indexing.LoadBlockLedger(ledger, n)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load the record of block %d: %w", n, err)
		}
		for _, e := range entries {
			if e == nil || e.Account == nil || e.Account.Authority == "" ||
				e.Account.Equal(unknown) || e.Account.Identity().Equal(unknown) {
				continue
			}
			k := strings.ToLower(e.Account.String())
			if _, ok := seen[k]; !ok {
				seen[k] = e.Account
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*url.URL, len(keys))
	for i, k := range keys {
		out[i] = seen[k]
	}
	return out, nil
}

// statesPage answers the accounts from cur on, as many whole as fit the
// budget. An account that does not fit a page of its own is answered in
// parts, one part a page, and a page that carries a part carries nothing else.
func (s *Querier) statesPage(batch *database.Batch, accounts []*url.URL, cur statesCursor) (*api.AccountStatesRecord, error) {
	r := new(api.AccountStatesRecord)
	budget := accountStatesBudget
	used := 0
	for i := cur.index; i < uint64(len(accounts)); i++ {
		st, err := s.accountState(batch, accounts[i])
		if err != nil {
			return nil, errors.UnknownError.WithFormat("%v: %w", accounts[i], err)
		}
		data, err := st.MarshalBinary()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("marshal the state of %v: %w", accounts[i], err)
		}

		if i == cur.index && (cur.offset > 0 || len(data) > budget) {
			// In parts. The part is of this view's state; the asker checks
			// the whole against its leaf.
			if cur.offset >= uint64(len(data)) {
				return nil, errors.BadRequest.WithFormat("the cursor names byte %d of a state of %d bytes", cur.offset, len(data))
			}
			end := min(cur.offset+uint64(budget), uint64(len(data)))
			r.Part = &api.AccountStatePart{Url: accounts[i], Offset: cur.offset, Total: uint64(len(data)), Data: data[cur.offset:end]}
			next := statesCursor{index: i, offset: end}
			if end == uint64(len(data)) {
				next = statesCursor{index: i + 1}
			}
			if next.index < uint64(len(accounts)) {
				r.Cursor = next.encode()
			}
			return r, nil
		}
		if used+len(data) > budget {
			r.Cursor = statesCursor{index: i}.encode()
			return r, nil
		}
		used += len(data)
		r.States = append(r.States, st)
	}
	return r, nil
}

// accountState is one account's sync state as this view holds it: what the
// account's leaf hashes (database.Account.LeafState, which reads the store as
// hashState does), the entries of each chain's open mark set, and the leaf
// itself (executor.md, "Sync", §3).
func (s *Querier) accountState(batch *database.Batch, u *url.URL) (*api.AccountState, error) {
	st := &api.AccountState{Url: u}
	record := batch.Account(u)
	leafHash, err := batch.BPT().Get(record.Key())
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		st.NoLeaf = true
		return st, nil
	default:
		return nil, errors.UnknownError.WithFormat("load the leaf: %w", err)
	}
	if len(leafHash) != 32 {
		return nil, errors.InternalError.WithFormat("the leaf is %d bytes", len(leafHash))
	}
	st.LeafHash = [32]byte(leafHash)

	st.Main, err = record.Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load the main state: %w", err)
	}
	leaf, err := record.LeafState()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	st.Directory = leaf.Directory

	l := new(api.AccountLeaf)
	l.EventsRoot = leaf.EventsRoot
	l.LocalDeliveryQueue = leaf.LocalDeliveryQueue
	l.CascadeDeliveryQueue = leaf.CascadeDeliveryQueue
	if u.Equal(s.partition.Ledger()) {
		l.Events, err = ledgerEvents(record.Events())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load scheduled events: %w", err)
		}
	}
	for _, c := range leaf.Chains {
		chain, err := record.ChainByName(c.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get chain %s: %w", c.Name, err)
		}
		l.Chains = append(l.Chains, &api.ChainRecord{Name: c.Name, Type: chain.Type(), Count: c.Count, State: c.Pending})

		inner := chain.Inner()
		head, err := inner.Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load the head of chain %s: %w", c.Name, err)
		}
		open, err := inner.OpenSet(head)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load the open mark set of chain %s: %w", c.Name, err)
		}
		st.OpenEntries = append(st.OpenEntries, &api.ChainOpenEntries{Name: c.Name, Entries: open})
	}
	for _, p := range leaf.Pending {
		v, err := pendingSets(p)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		l.Pending = append(l.Pending, v)
	}
	for _, p := range leaf.BookPending {
		v, err := pendingSets(p)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		l.BookPending = append(l.BookPending, v)
	}
	st.Leaf = l
	return st, nil
}

// statesCursor is where the next page of an answer starts: the records' end
// the answer is fixed to, the account, and the byte of its state a part
// continues from.
type statesCursor struct {
	through, index, offset uint64
}

func (c statesCursor) encode() []byte {
	b := make([]byte, 0, 3*binary.MaxVarintLen64)
	b = binary.AppendUvarint(b, c.through)
	b = binary.AppendUvarint(b, c.index)
	b = binary.AppendUvarint(b, c.offset)
	return b
}

func decodeStatesCursor(b []byte) (statesCursor, error) {
	var c statesCursor
	if len(b) == 0 {
		return c, nil
	}
	for _, v := range []*uint64{&c.through, &c.index, &c.offset} {
		x, n := binary.Uvarint(b)
		if n <= 0 {
			return c, errors.BadRequest.With("malformed cursor")
		}
		*v, b = x, b[n:]
	}
	if len(b) != 0 {
		return c, errors.BadRequest.With("malformed cursor")
	}
	return c, nil
}
