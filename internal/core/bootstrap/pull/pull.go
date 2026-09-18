// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pull fetches account state from a peer, for a node that is pulling
// the state (executor.md, "Sync", step 3).
//
// Two modes:
//
//   - ModeStateOnly: the account body, its secondary state (the Directory
//     list, the Pending txid list) and its chain *heads* — no chain entries.
//     That is enough to reproduce the account's BPT leaf, because the
//     observer hashes a chain's head anchor, not its entries.
//
//   - ModeFullSpine: the same, plus every chain entry replayed. Used for the
//     spine — anchors, ledger, operators, operators/1 — where the node needs
//     the chain history itself: without the operators' key pages of the time
//     it cannot verify the signatures on the anchors it verifies against.
//
// Verification. An account is only as good as the root it hashes into, and a
// node that is still pulling has no root of its own: it verifies against the
// root the Directory anchored for the block the peer served the account at
// (Verify, in verify.go). A peer whose state does not verify is refused and
// another is asked — AccountFrom. Nothing is written into the caller's batch
// until it verifies.
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the pull
// writes through a nested batch and verifies before committing it; the
// bootstrap-v3 puller wrote straight through and verified nothing, because
// that design trusted whole-BPT root match and the peer's ACTIVE claim.
package pull

import (
	"context"
	stderrors "errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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

	// QueryMessage is needed to pull the sig-material for pending
	// transactions (validator signatures, payments, votes,
	// signatures). Without this the per-account hash diverges for
	// any account with non-empty Pending — see #3999.
	QueryMessage(ctx context.Context, txid *url.TxID, query *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error)
}

// The v3 client is a Source as it stands.
var _ Source = api.Querier2{}

// Options configures Account.
type Options struct {
	Mode Mode

	// PageSize for paginated list pulls. Default 256.
	PageSize uint64

	// Verify says what root the Directory anchored for the block a peer served
	// an account at. When it is set, Account refuses state that does not hash
	// into that root.
	//
	// Nil pulls without verifying. That is for the Directory spine — the
	// accounts the verifier itself reads from, which cannot be verified before
	// they exist — and for tests.
	Verify Verifier

	// Partition is the partition whose blocks the account's state belongs to.
	// Required when Verify is set.
	Partition *url.URL
}

// Pending is state pulled from a peer and not yet kept. It sits in a batch of
// its own; nothing reaches the caller's batch until Settle says the Directory
// anchored the root it hashes into.
//
// It exists because the pull runs ahead of the anchors. A peer serves its
// current block, and the Directory anchors that block a few blocks later, so
// an account fetched now is verified in a moment — not refused for arriving
// before its proof (executor.md, "Sync": the pull follows the network).
type Pending struct {
	// Account is the account that was pulled.
	Account *url.URL

	// Block is the block the peer served the state at. It is the block whose
	// anchored root settles it.
	Block uint64

	receipt *api.Receipt
	batch   *database.Batch
	done    bool
}

// Settle verifies the state against the root the Directory anchored for
// Pending.Block and, if it holds, writes it into the caller's batch. Either
// way the pending state is released.
func (p *Pending) Settle(anchoredRoot [32]byte) error {
	if p.done {
		return errors.NotAllowed.WithFormat("%v: already settled", p.Account)
	}
	p.done = true
	defer p.batch.Discard()

	if p.receipt == nil {
		// The peer has no such account, so there is nothing to verify and
		// nothing was written.
		return nil
	}
	if err := Verify(p.batch, p.Account, p.receipt, anchoredRoot); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return errors.UnknownError.Wrap(p.batch.Commit())
}

// Keep writes the state into the caller's batch without verifying it. It is
// for the Directory spine, which is what the verifier itself is read from, and
// for tests.
func (p *Pending) Keep() error {
	if p.done {
		return errors.NotAllowed.WithFormat("%v: already settled", p.Account)
	}
	p.done = true
	defer p.batch.Discard()
	return errors.UnknownError.Wrap(p.batch.Commit())
}

// Discard throws the pulled state away.
func (p *Pending) Discard() {
	p.done = true
	p.batch.Discard()
}

// Fetch pulls u from src per opts.Mode and holds it, unverified and unwritten,
// until the caller settles it. withReceipt asks the peer for the proof that
// binds the state to its root; without one the state can only be kept, not
// verified.
func Fetch(ctx context.Context, src Source, batch *database.Batch, u *url.URL, opts Options, withReceipt bool) (*Pending, error) {
	if src == nil {
		return nil, errors.BadRequest.With("pull.Fetch: src required")
	}
	if batch == nil {
		return nil, errors.BadRequest.With("pull.Fetch: batch required")
	}
	if u == nil {
		return nil, errors.BadRequest.With("pull.Fetch: url required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	sub := batch.Begin(true)
	p := &Pending{Account: u, batch: sub}

	fail := func(err error) (*Pending, error) {
		sub.Discard()
		return nil, err
	}

	// 1. Main account state, with the receipt that binds it to the peer's root.
	receipt, err := pullMain(ctx, src, sub, u, withReceipt)
	if err != nil {
		return fail(errors.UnknownError.WithFormat("main %s: %w", u, err))
	}
	p.receipt = receipt
	if receipt != nil {
		p.Block = receipt.LocalBlock
	}

	// 2. Directory entries (the secondary-state list of contained URLs).
	if err := pullDirectory(ctx, src, sub, u, pageSize); err != nil {
		return fail(errors.UnknownError.WithFormat("directory %s: %w", u, err))
	}

	// 3. Pending txids.
	if err := pullPending(ctx, src, sub, u, pageSize); err != nil {
		return fail(errors.UnknownError.WithFormat("pending %s: %w", u, err))
	}

	// 4. Chains.
	switch opts.Mode {
	case ModeStateOnly:
		if err := pullChainHeads(ctx, src, sub, u, pageSize); err != nil {
			return fail(errors.UnknownError.WithFormat("chain heads %s: %w", u, err))
		}
	case ModeFullSpine:
		if err := pullChainsFull(ctx, src, sub, u, pageSize); err != nil {
			return fail(errors.UnknownError.WithFormat("chains full %s: %w", u, err))
		}
	default:
		return fail(errors.BadRequest.WithFormat("unknown pull mode %d", opts.Mode))
	}

	return p, nil
}

// Account pulls u from src into batch per opts.Mode and, when opts.Verify is
// set, refuses it unless it hashes into the root the Directory anchored for
// the block the peer served it at. It is Fetch and Settle in one call, for a
// caller that can wait on the anchor; a caller that cannot uses the two.
//
// Nothing is written until it verifies, so a refused account leaves nothing
// behind. Note that the peer's state moves while the pull runs: the four
// queries can straddle a block, in which case the assembled state hashes to
// nothing the Directory anchored and the account is refused. That is the pull
// racing the network, and the answer to it is to ask again — see AccountFrom.
func Account(ctx context.Context, src Source, batch *database.Batch, u *url.URL, opts Options) error {
	if opts.Verify != nil && opts.Partition == nil {
		return errors.BadRequest.With("pull.Account: partition required when verifying")
	}

	p, err := Fetch(ctx, src, batch, u, opts, opts.Verify != nil)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if opts.Verify == nil {
		return errors.UnknownError.Wrap(p.Keep())
	}

	root, err := opts.Verify.AnchoredRoot(ctx, opts.Partition, p.Block)
	if err != nil {
		p.Discard()
		return errors.UnknownError.Wrap(err)
	}
	return errors.UnknownError.Wrap(p.Settle(root))
}

// AccountFrom pulls u from the first source whose state verifies, and reports
// which one answered. A source that serves state that does not hash into the
// anchored root is refused and the next is asked; when none answer, every
// refusal is reported.
func AccountFrom(ctx context.Context, srcs []Source, batch *database.Batch, u *url.URL, opts Options) (int, error) {
	if len(srcs) == 0 {
		return -1, errors.BadRequest.With("pull.AccountFrom: at least one source required")
	}
	var refusals []error
	for i, src := range srcs {
		err := Account(ctx, src, batch, u, opts)
		if err == nil {
			return i, nil
		}
		if errors.Is(err, ErrNotAnchored) || ctx.Err() != nil {
			// Not the peer's fault, and asking another will not help.
			return -1, errors.UnknownError.Wrap(err)
		}
		refusals = append(refusals, errors.UnknownError.WithFormat("source %d: %w", i, err))
	}
	return -1, errors.Conflict.WithFormat("%v: no source served state that verifies: %w", u, stderrors.Join(refusals...))
}

// pullMain stores the account body and returns the receipt the peer served
// with it, which binds the body to the peer's BPT root. wantReceipt asks for
// one; without it the peer does the work of building a proof nobody checks.
func pullMain(ctx context.Context, src Source, batch *database.Batch, u *url.URL, wantReceipt bool) (*api.Receipt, error) {
	var query *api.DefaultQuery
	if wantReceipt {
		query = &api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}}
	}
	rec, err := src.QueryAccount(ctx, u, query)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query account: %w", err)
	}
	if rec == nil || rec.Account == nil {
		return nil, nil // Nothing to store
	}
	if err := batch.Account(u).Main().Put(rec.Account); err != nil {
		return nil, errors.UnknownError.WithFormat("store main: %w", err)
	}
	if wantReceipt && rec.Receipt == nil {
		return nil, errors.Conflict.WithFormat("%v: the peer served no receipt", u)
	}
	return rec.Receipt, nil
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
			// TODO #3999: also pull each pending tx's sig-material
			// (ValidatorSignatures, Payments, Votes, Signatures) so
			// hashPendingV2 can compute the same per-account hash as
			// the source. Without that, accounts with non-empty
			// Pending diverge and the launcher never promotes.
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
	// Empty ChainQuery requests "list all chains for this account".
	// Setting Range here triggers the v3 validator's "name is required
	// when querying by index, entry, or range" rejection — Range is
	// for entries within a named chain, not for the chain list.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
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
		if err := addChainToIndex(batch, u, c); err != nil {
			return err
		}
	}
	return nil
}

// addChainToIndex records the chain in the account's chain index. The account
// hash is taken over the chains that index names, and a batch only builds it
// when it commits — the pull verifies before it commits, so it writes the
// index itself.
func addChainToIndex(batch *database.Batch, u *url.URL, c *api.ChainRecord) error {
	meta := &protocol.ChainMetadata{Name: c.Name, Type: c.Type}
	_, err := batch.Account(u).Chains().Index(meta)
	switch {
	case err == nil:
		return nil // Already listed
	case errors.Is(err, errors.NotFound):
		return errors.UnknownError.Wrap(batch.Account(u).Chains().Add(meta))
	default:
		return errors.UnknownError.WithFormat("chain index %s: %w", u, err)
	}
}

// pullChainsFull replays every entry on every chain via
// merkle.AddEntry. The Head is rebuilt naturally as entries are
// added; we don't write Head() directly.
func pullChainsFull(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	// Empty ChainQuery: list-all-chains. See pullChainHeads.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
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
		if err := addChainToIndex(batch, u, c); err != nil {
			return err
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

// SpineAccounts returns the four spine accounts for a given
// partition. The launcher pulls these in ModeFullSpine because the
// orchestrator's tracker (#3988) needs full chain history for the
// validator keypage to verify signed major-block anchors locally.
//
// For the DN: dn.acme/{anchors, ledger, operators, operators/1}.
// For a BVN: <bvn>.acme/{anchors, ledger, operators, operators/1}.
func SpineAccounts(partitionURL *url.URL) []*url.URL {
	return []*url.URL{
		partitionURL.JoinPath(protocol.AnchorPool),
		partitionURL.JoinPath(protocol.Ledger),
		partitionURL.JoinPath(protocol.Operators),
		partitionURL.JoinPath(protocol.Operators, "1"),
	}
}

// DnSpineAccounts is a backward-compatible helper returning
// SpineAccounts(protocol.DnUrl()).
func DnSpineAccounts() []*url.URL { return SpineAccounts(protocol.DnUrl()) }
