// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pull is the bootstrap-v3 launcher's account-state puller.
//
// Two modes:
//
//   - ModeStateOnly: head (protocol.Account body) + secondary state
//     (Directory list, Pending txid list) + chain *heads* (Count +
//     Pending). No chain entries. Sufficient to recompute the BPT
//     leaf hash for the account because the production observer
//     hashes only the chain head's Anchor (computed from Pending),
//     not entries.
//
//   - ModeFullSpine: head + secondary state + every chain entry
//     replayed via merkle.AddEntry. Used for DN-side spine accounts
//     (anchor pool, ledger, operators, operators/1) where the
//     launcher needs the full chain history for ongoing operation.
//
// Trust model: per the v3 doc, BPT-match-against-trusted-anchor is
// the consistency check. The puller doesn't verify anything itself
// — it just populates the local DB. The launcher's tracker (v3-4)
// is what flips ACTIVE on root match.
package pull

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Mode determines how chain data is pulled.
type Mode int

const (
	// ModeStateOnly: head + secondary + chain heads (no entries).
	// Used for the long tail and for accounts touched on demand by
	// gossip.
	ModeStateOnly Mode = iota

	// ModeFullSpine: head + secondary + every chain entry replayed
	// locally. Used for the four DN-side spine accounts.
	ModeFullSpine
)

// Source is the read-only surface Pull needs from the network.
type Source interface {
	QueryAccount(ctx context.Context, scope *url.URL, query *api.DefaultQuery) (*api.AccountRecord, error)
	QueryDirectoryUrls(ctx context.Context, scope *url.URL, query *api.DirectoryQuery) (*api.RecordRange[*api.UrlRecord], error)
	QueryPendingIds(ctx context.Context, scope *url.URL, query *api.PendingQuery) (*api.RecordRange[*api.TxIDRecord], error)
	QueryAccountChains(ctx context.Context, scope *url.URL, query *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error)
	QueryChainEntries(ctx context.Context, scope *url.URL, query *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error)
}

// Options configures Account.
type Options struct {
	Mode Mode

	// PageSize for paginated list pulls. Default 256.
	PageSize uint64
}

// Account pulls u from src into batch per opts.Mode. After Account
// returns successfully, calling batch.Account(u).Hash() will produce
// the same BPT leaf hash as the source DB (modulo the source's BPT
// having advanced — eventual consistency under live traffic).
func Account(ctx context.Context, src Source, batch *database.Batch, u *url.URL, opts Options) error {
	if src == nil {
		return fmt.Errorf("pull.Account: src required")
	}
	if batch == nil {
		return fmt.Errorf("pull.Account: batch required")
	}
	if u == nil {
		return fmt.Errorf("pull.Account: url required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	// 1. Main account state.
	if err := pullMain(ctx, src, batch, u); err != nil {
		return fmt.Errorf("main %s: %w", u, err)
	}

	// 2. Directory entries (the secondary-state list of contained URLs).
	if err := pullDirectory(ctx, src, batch, u, pageSize); err != nil {
		return fmt.Errorf("directory %s: %w", u, err)
	}

	// 3. Pending txids.
	if err := pullPending(ctx, src, batch, u, pageSize); err != nil {
		return fmt.Errorf("pending %s: %w", u, err)
	}

	// 4. Chains.
	switch opts.Mode {
	case ModeStateOnly:
		if err := pullChainHeads(ctx, src, batch, u, pageSize); err != nil {
			return fmt.Errorf("chain heads %s: %w", u, err)
		}
	case ModeFullSpine:
		if err := pullChainsFull(ctx, src, batch, u, pageSize); err != nil {
			return fmt.Errorf("chains full %s: %w", u, err)
		}
	default:
		return fmt.Errorf("unknown pull mode %d", opts.Mode)
	}
	return nil
}

func pullMain(ctx context.Context, src Source, batch *database.Batch, u *url.URL) error {
	rec, err := src.QueryAccount(ctx, u, nil)
	if err != nil {
		return fmt.Errorf("query account: %w", err)
	}
	if rec == nil || rec.Account == nil {
		return nil // pre-genesis placeholder; nothing to store
	}
	if err := batch.Account(u).Main().Put(rec.Account); err != nil {
		return fmt.Errorf("store main: %w", err)
	}
	return nil
}

func pullDirectory(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	var start uint64
	for {
		count := pageSize
		page, err := src.QueryDirectoryUrls(ctx, u, &api.DirectoryQuery{
			Range: &api.RangeOptions{Start: start, Count: &count},
		})
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}
		if page == nil || len(page.Records) == 0 {
			return nil
		}
		for _, r := range page.Records {
			if r == nil || r.Value == nil {
				continue
			}
			if err := batch.Account(u).Directory().Add(r.Value); err != nil {
				return fmt.Errorf("add %s: %w", r.Value, err)
			}
		}
		if uint64(len(page.Records)) < count {
			return nil
		}
		start += uint64(len(page.Records))
	}
}

func pullPending(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	var start uint64
	for {
		count := pageSize
		page, err := src.QueryPendingIds(ctx, u, &api.PendingQuery{
			Range: &api.RangeOptions{Start: start, Count: &count},
		})
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}
		if page == nil || len(page.Records) == 0 {
			return nil
		}
		for _, r := range page.Records {
			if r == nil || r.Value == nil {
				continue
			}
			if err := batch.Account(u).Pending().Add(r.Value); err != nil {
				return fmt.Errorf("add %s: %w", r.Value, err)
			}
		}
		if uint64(len(page.Records)) < count {
			return nil
		}
		start += uint64(len(page.Records))
	}
}

// pullChainHeads sets each of the account's chains' Head() directly
// from ChainRecord.{Count, State}. Skips chain entries entirely. The
// resulting BPT-leaf hash matches the source's because hashChains
// uses CurrentState().Anchor() which is computed from Pending only
// (see internal/core/execute/v2/internal/bpt_prod.go).
func pullChainHeads(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{
		Range: &api.RangeOptions{Count: &pageSize},
	})
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}
	if chains == nil {
		return nil
	}
	for _, c := range chains.Records {
		if c == nil || c.Name == "" {
			continue
		}
		state := &merkle.State{
			Count:   int64(c.Count),
			Pending: c.State,
			// HashList not exposed via api.ChainRecord; left nil.
			// Anchor() uses Pending only, so BPT-leaf hash is
			// correct. Forward AddEntry calls (gossip extension)
			// rebuild HashList naturally from the markpoint cycle.
		}
		dstChain, err := batch.Account(u).ChainByName(c.Name)
		if err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
		if err := dstChain.Head().Put(state); err != nil {
			return fmt.Errorf("set head %s/%s: %w", u, c.Name, err)
		}
	}
	return nil
}

// pullChainsFull replays every entry on every chain via
// merkle.AddEntry. The Head is rebuilt naturally as entries are
// added; we don't write Head() directly.
func pullChainsFull(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{
		Range: &api.RangeOptions{Count: &pageSize},
	})
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}
	if chains == nil {
		return nil
	}
	for _, c := range chains.Records {
		if c == nil || c.Name == "" {
			continue
		}
		dstChain, err := batch.Account(u).ChainByName(c.Name)
		if err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
		if err := pullChainEntries(ctx, src, dstChain.Inner(), u, c.Name, pageSize); err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
	}
	return nil
}

// pullChainEntries paginates one chain's entries and AddEntry-s
// them into the local chain.
func pullChainEntries(ctx context.Context, src Source, dst dstChainAdder, u *url.URL, chainName string, pageSize uint64) error {
	var start uint64
	for {
		count := pageSize
		expand := false
		page, err := src.QueryChainEntries(ctx, u, &api.ChainQuery{
			Name: chainName,
			Range: &api.RangeOptions{
				Start:  start,
				Count:  &count,
				Expand: &expand,
			},
		})
		if err != nil {
			return fmt.Errorf("query entries: %w", err)
		}
		if page == nil || len(page.Records) == 0 {
			return nil
		}
		for _, e := range page.Records {
			if e == nil {
				continue
			}
			if err := dst.AddEntry(e.Entry[:], false); err != nil {
				return fmt.Errorf("add entry %d: %w", e.Index, err)
			}
		}
		if uint64(len(page.Records)) < count {
			return nil
		}
		start += uint64(len(page.Records))
	}
}

// dstChainAdder narrows a database chain to just the AddEntry method
// pullChainEntries needs. Lets us share the loop across the merkle
// chain and the index chain types if needed later.
type dstChainAdder interface {
	AddEntry(hash []byte, unique bool) error
}

// dnSpineAccounts is the canonical DN-side spine list. Exported as a
// helper for the orchestrator (v3-5).
func dnSpineAccounts() []*url.URL {
	dn := protocol.DnUrl()
	return []*url.URL{
		dn.JoinPath(protocol.AnchorPool),
		dn.JoinPath(protocol.Ledger),
		dn.JoinPath(protocol.Operators),
		dn.JoinPath(protocol.Operators, "1"),
	}
}

// DnSpineAccounts returns the four DN-side spine accounts the
// launcher pulls in ModeFullSpine.
func DnSpineAccounts() []*url.URL { return dnSpineAccounts() }
