// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package keybookat resolves a keybook to its state at a given block time
// by walking the keybook's pages' main chains. Central back-walk primitive
// for the minimum-data node bootstrap (issue #3957, #3953).
//
// Initial implementation handles the common case where no main-chain entries
// post-date the requested block time — i.e., the requested time is at or
// after every recorded mutation. In that case the resolver returns the pages'
// current state. Forward replay for older block times is not yet implemented;
// callers receive ErrNotYetImplemented when the requested time precedes
// the most recent main-chain entry.
package keybookat

import (
	"errors"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Resolved is a keybook resolved to its state at a given block time.
type Resolved struct {
	Url       *url.URL
	BlockTime time.Time
	Pages     []*protocol.KeyPage
}

// ErrNotYetImplemented indicates the requested block time precedes
// recorded mutations and the forward-replay path isn't built yet.
var ErrNotYetImplemented = errors.New("keybookat: forward replay for historical times not yet implemented")

// Resolve returns the state of the keybook at kbUrl as of block time t.
// Pages are returned in page-index order (page 1 first).
func Resolve(batch *database.Batch, kbUrl *url.URL, t time.Time) (*Resolved, error) {
	if kbUrl == nil {
		return nil, fmt.Errorf("nil keybook url")
	}

	var book protocol.KeyBook
	err := batch.Account(kbUrl).Main().GetAs(&book)
	if err != nil {
		return nil, fmt.Errorf("load keybook %s: %w", kbUrl, err)
	}
	if book.PageCount == 0 {
		return &Resolved{Url: kbUrl, BlockTime: t}, nil
	}

	res := &Resolved{Url: kbUrl, BlockTime: t, Pages: make([]*protocol.KeyPage, 0, book.PageCount)}
	for i := uint64(1); i <= book.PageCount; i++ {
		pageUrl := protocol.FormatKeyPageUrl(kbUrl, i-1)

		page, err := resolvePage(batch, pageUrl, t)
		if err != nil {
			return nil, fmt.Errorf("resolve page %d: %w", i, err)
		}
		res.Pages = append(res.Pages, page)
	}
	return res, nil
}

// resolvePage returns one page's state at time t. Initial implementation
// only handles the case where no main-chain entries post-date t.
func resolvePage(batch *database.Batch, pageUrl *url.URL, t time.Time) (*protocol.KeyPage, error) {
	acct := batch.Account(pageUrl)

	var page protocol.KeyPage
	err := acct.Main().GetAs(&page)
	if err != nil {
		return nil, fmt.Errorf("load page %s: %w", pageUrl, err)
	}

	// Check if any main-chain entries post-date t. If so, the current state
	// already reflects later mutations; we'd need to roll them back.
	chain, err := acct.MainChain().Get()
	if err != nil {
		return nil, fmt.Errorf("get main chain: %w", err)
	}
	count := chain.Height()
	if count == 0 {
		return &page, nil
	}

	// We don't have direct per-entry block-time on the main chain itself
	// (it lives on the index chain alongside each entry). For the initial
	// implementation we approximate: if the chain's last block time
	// (returned by indexing) is at or before t, the current state is correct.
	// Otherwise return ErrNotYetImplemented.
	lastBlockTime, ok, err := lastEntryBlockTime(acct)
	if err != nil {
		return nil, fmt.Errorf("read last block time: %w", err)
	}
	if !ok || !lastBlockTime.After(t) {
		return &page, nil
	}
	return nil, ErrNotYetImplemented
}

// lastEntryBlockTime returns the block time of the most recent main-chain
// entry on the page, if available. It reads the page's main-chain index
// chain (`main-index`) and inspects the last index entry. Returns ok=false
// if no index entries exist.
func lastEntryBlockTime(acct *database.Account) (time.Time, bool, error) {
	idx, err := acct.MainChain().Index().Get()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("get main-index chain: %w", err)
	}
	count := idx.Height()
	if count == 0 {
		return time.Time{}, false, nil
	}
	// Index entries are 32-byte hashes referring to IndexEntry records;
	// the entries themselves carry BlockTime. We read the last entry value.
	last, err := idx.Entry(count - 1)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read last index entry: %w", err)
	}
	// The IndexEntry is unmarshaled from the entry value.
	var ie protocol.IndexEntry
	if err := ie.UnmarshalBinary(last); err != nil {
		return time.Time{}, false, fmt.Errorf("decode index entry: %w", err)
	}
	if ie.BlockTime == nil {
		return time.Time{}, false, nil
	}
	return *ie.BlockTime, true, nil
}
