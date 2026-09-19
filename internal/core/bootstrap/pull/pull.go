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
//     list, the Pending txid list) and its chains' heads with their open mark
//     sets — no history. The head reproduces the account's BPT leaf, because
//     the observer hashes a chain's head anchor and not its entries; the open
//     mark set is what makes the chain appendable, so the node can execute
//     the next block.
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
// What is pulled replaces what the node held for that account rather than
// joining with it, and a re-pull of an account is the same account: a restart
// re-pulls, and a pull that unions or appends leaves a node holding something
// no peer holds, which never hashes into an anchored root again.
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the pull
// writes through a nested batch and verifies before committing it; the
// bootstrap-v3 puller wrote straight through and verified nothing, because
// that design trusted whole-BPT root match and the peer's ACTIVE claim.
package pull

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

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
	// ModeStateOnly: head + secondary + chain heads with their open mark
	// sets, and no history. Used for the long tail.
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
	// Nil pulls without verifying, which is for tests only. The spine used
	// to be pulled this way, on the rationale that it is what the verifier
	// reads from; that rationale was false — a signature is verified against
	// a key the node already holds, not against a root — and it is what
	// #4301 closed.
	Verify Verifier

	// Partition is the partition whose blocks the account's state belongs to.
	// Required when Verify is set, and whenever a receipt is asked for: a
	// receipt proves the state as of a block, and block numbers collide
	// across partitions (#4205).
	Partition *url.URL

	// AtBlock is the block to ask the peer for, and it is why a restart can
	// converge at all (#4362).
	//
	// Zero asks for whatever block the peer is on. That is what the pull did
	// before, and it works for a COLD account -- one the peer has not touched
	// for a while, served at an old block the Directory anchored long ago,
	// which settles on the first try. It cannot work for a HOT one. The
	// partition's ledger, anchors and synthetic ledger change every block, so
	// the peer serves them at its CURRENT block, the Directory has not
	// anchored that block yet, and by the time it does the peer has moved on.
	// A restarted node holds every cold account already and differs from its
	// peers only in the hot ones, so the pull it needs is exactly the pull
	// that could never settle: pulled=0, forever.
	//
	// Non-zero asks for the state as of that block, which the caller takes
	// from a spine anchor it has verified (anchorsrc.Source.LatestAnchor), so
	// the answer is anchored before it is requested and settles on the round
	// it was fetched. The peer must answer at that block or not at all; a
	// peer that answers at a different one is refused rather than settled
	// against a root nobody asked about.
	//
	// The peer must be retaining that block's BPT state to answer -- a node's
	// default is 1024 blocks (cmd/accumulated/run/dagbft.go), and a peer
	// configured with none refuses, which is a refusal and not a wait.
	AtBlock uint64
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

	// Partition is the partition whose block Block is. Block numbers collide
	// across partitions — with a one-second cadence the Directory and a BVN
	// are at the same number at the same second — so a block number without
	// its partition names nothing (#4205).
	Partition *url.URL

	// Block is the block the peer served the state at. It is the block of
	// Partition whose anchored root settles it.
	Block uint64

	receipt *api.Receipt
	batch   *database.Batch
	done    bool

	// past says the node was found to be past this peer for this account, so
	// nothing was pulled: the batch was discarded where it was decided and
	// there is nothing to write and nothing to verify. See Fetch.
	past bool
}

// Past reports that the node is past the peer for this account — at or beyond
// it on every chain the peer serves and strictly beyond on at least one — so
// everything the peer could give for it, the node already has. Nothing was
// pulled; Settle and Keep are no-ops and there is nothing to wait for an
// anchor for.
func (p *Pending) Past() bool { return p.past }

// MaxHeld bounds how many fetched-but-unsettled accounts may be outstanding at
// once, across the process. Each one holds an open child batch, so the state it
// pulled is held in memory until it settles or is discarded, and an unbounded
// pull is an unbounded heap. A caller that needs more than this settles a round
// of accounts before fetching the next.
const MaxHeld = 1024

var held atomic.Int64

// Held reports how many fetched accounts are outstanding, for diagnostics.
func Held() int64 { return held.Load() }

// release marks the pending state finished and gives back its place under
// MaxHeld. It is called exactly once per Pending.
func (p *Pending) release() {
	p.done = true
	held.Add(-1)
}

// Settle verifies the state against the root the Directory anchored for
// Pending.Block and, if it holds, writes it into the caller's batch. Either
// way the pending state is released.
//
// It cannot succeed on anything it did not verify: a fetch that carries no
// receipt, or a root nobody anchored, is a failure, not an empty success.
//
// The one thing it does not verify is a pull that took nothing. The past case
// — the node is beyond this peer on the account — is structurally incompatible
// with verification: what the node holds is not what the peer serves, so it
// cannot hash to the leaf the peer's receipt proves, and a past pull that
// carried the node's own state in its batch would be refused by its own
// verifier. That is why the past case discards its batch at the point it is
// decided (Fetch) rather than filling it with the node's own state: there is
// nothing to write, so there is nothing to verify, and the invariant is held
// by construction instead of by a flag that says "trust this one".
func (p *Pending) Settle(anchoredRoot [32]byte) error {
	if p.done {
		return errors.NotAllowed.WithFormat("%v: already settled", p.Account)
	}
	p.release()
	if p.past {
		return nil
	}
	defer p.batch.Discard()

	if p.receipt == nil {
		return errors.BadRequest.WithFormat(
			"%v: fetched without a receipt, so there is nothing to settle it against", p.Account)
	}
	if anchoredRoot == ([32]byte{}) {
		return errors.BadRequest.WithFormat(
			"%v: the directory anchored no root for %v block %d", p.Account, p.Partition, p.Block)
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
	p.release()
	if p.past {
		return nil // Nothing was pulled; see Settle.
	}
	defer p.batch.Discard()
	return errors.UnknownError.Wrap(p.batch.Commit())
}

// Discard throws the pulled state away.
func (p *Pending) Discard() {
	if p.done {
		return
	}
	p.release()
	if p.past {
		return // Nothing was pulled; see Settle.
	}
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
	if withReceipt && opts.Partition == nil {
		// A receipt proves the state as of a block, and a block number without
		// its partition names nothing.
		return nil, errors.BadRequest.With("pull.Fetch: partition required when asking for a receipt")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	if n := held.Add(1); n > MaxHeld {
		held.Add(-1)
		return nil, errors.NotReady.WithFormat(
			"%v: %d accounts are already fetched and unsettled, the limit is %d", u, n-1, MaxHeld)
	}

	sub := batch.Begin(true)
	p := &Pending{Account: u, Partition: opts.Partition, batch: sub}

	fail := func(err error) (*Pending, error) {
		p.release()
		sub.Discard()
		return nil, err
	}

	// 1. Main account state, with the receipt that binds it to the peer's root.
	receipt, err := pullMain(ctx, src, sub, u, withReceipt, opts.AtBlock)
	if err != nil {
		return fail(errors.UnknownError.WithFormat("main %s: %w", u, err))
	}
	p.receipt = receipt
	if receipt != nil {
		// The block this state is FOR. On a historical answer that is
		// ForHeight -- the block the proof was built at -- and NOT
		// LocalBlock, which is the serving node's present and moves every
		// block whatever was asked for. Settling against LocalBlock is what
		// made a hot account unsettleable: the root asked of the Directory
		// was always the peer's newest, which it has not anchored yet (#4362).
		p.Block = receipt.LocalBlock
		if receipt.ForHeight != 0 {
			p.Block = receipt.ForHeight
		}

		// The block is the SERVING partition's, not the puller's. A receipt
		// proves the state as of a block of the partition that built it
		// (api.Receipt.Partition, internal/api/v3/querier.go), and block
		// numbers collide across partitions -- so settling a foreign
		// account's block against this node's partition asks the Directory
		// for a root it never anchored for that block (#4308). Latent while
		// every account a join pulls is its own partition's; wrong the moment
		// one is not.
		if receipt.Partition != "" {
			p.Partition = protocol.PartitionUrl(receipt.Partition)
		}
	}

	// 2. Directory entries (the secondary-state list of contained URLs).
	if err := pullDirectory(ctx, src, sub, u, pageSize); err != nil {
		return fail(errors.UnknownError.WithFormat("directory %s: %w", u, err))
	}

	// 3. Pending txids.
	if err := pullPending(ctx, src, sub, u, pageSize); err != nil {
		return fail(errors.UnknownError.WithFormat("pending %s: %w", u, err))
	}

	// 4. Chains, and with them the account-level meeting point. Both modes
	// answer it: the long tail is pulled in ModeStateOnly, and the long tail
	// is where a restarted node spends its life. When the node is ahead,
	// ChangedAccounts names <partition>/ledger and <partition>/synthetic
	// unconditionally and enumerate.Stale names every account whose leaf
	// differs from the peer's IN EITHER DIRECTION — so the ahead case is not
	// an edge of the long tail, it is what the long tail runs every round
	// (#4348).
	var m *meeting
	switch opts.Mode {
	case ModeStateOnly:
		m, err = pullChainHeads(ctx, src, sub, u, pageSize)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chain heads %s: %w", u, err))
		}
	case ModeFullSpine:
		m, err = pullChainsFull(ctx, src, sub, u, pageSize)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chains full %s: %w", u, err))
		}
	default:
		return fail(errors.BadRequest.WithFormat("unknown pull mode %d", opts.Mode))
	}

	past, err := m.past(u)
	if err != nil {
		return fail(errors.UnknownError.Wrap(err))
	}
	if past {
		// The meeting point is the ACCOUNT's, not one chain's, and the
		// account is its body, its directory list, its pending list and its
		// chains together — that is what its leaf is hashed from
		// (observer_prod.go, hashState). A node past this peer on the account
		// is past it on all four: taking any one of them from the peer builds
		// a leaf that is neither side's, which is the one thing a pull must
		// never leave behind, because a node holding something no peer holds
		// never hashes into an anchored root again.
		//
		// So nothing is taken. The batch is thrown away where the meeting
		// point is decided rather than being refilled with the node's own
		// state: refilling it writes the node's state back over itself, which
		// is at best a no-op and at worst a leaf assembled out of two heights
		// — and it leaves a batch that cannot verify, since what is in it is
		// not what the peer's receipt proves (see Settle).
		//
		// This is not a refusal. Everything this peer can give for this
		// account, the node has; the pull is complete and it is empty.
		p.past = true
		sub.Discard()
		p.batch = nil
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

	root, err := opts.Verify.AnchoredRoot(ctx, p.Partition, p.Block)
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
//
// A source that serves the account at a block the Directory has not anchored
// is one of those refusals, not the end of the pull. The block compared is the
// one the peer put in its own receipt, so a peer claiming a block that will
// never be anchored would otherwise stop the whole pull for everyone. Only
// when no source could serve an anchored state is the failure reported as
// ErrNotAnchored — which is the wait it names, and the caller asks again.
func AccountFrom(ctx context.Context, srcs []Source, batch *database.Batch, u *url.URL, opts Options) (int, error) {
	if len(srcs) == 0 {
		return -1, errors.BadRequest.With("pull.AccountFrom: at least one source required")
	}
	var refusals []error
	var early int
	for i, src := range srcs {
		err := Account(ctx, src, batch, u, opts)
		if err == nil {
			return i, nil
		}
		if ctx.Err() != nil {
			return -1, errors.UnknownError.Wrap(err)
		}
		if errors.Is(err, ErrNotAnchored) {
			early++
		}
		refusals = append(refusals, errors.UnknownError.WithFormat("source %d: %w", i, err))
	}
	if early == len(srcs) {
		return -1, errors.NotReady.WithFormat(
			"%v: no source served a state at a block the directory has anchored: %w", u, ErrNotAnchored)
	}
	return -1, errors.Conflict.WithFormat("%v: no source served state that verifies: %w", u, stderrors.Join(refusals...))
}

// FetchFrom fetches u from the first source that serves it and hands the state
// back held, unverified and unwritten, with the index of the source that
// answered. A source that cannot serve the account is refused and the next is
// asked; when none answer, every refusal is reported.
//
// It is AccountFrom's first half, for a caller that settles later. That caller
// is the join: Account discards on ErrNotAnchored, so a caller built on it
// re-fetches next round, at a newer block the Directory has not anchored
// either -- a treadmill that never settles anything. Holding the fetch and
// retrying Settle against THE SAME BLOCK is what the Pending/Settle split
// exists for.
//
// The returned Pending holds an open child of batch. It must be settled or
// discarded before batch is committed or discarded, and it counts against
// MaxHeld until it is.
func FetchFrom(ctx context.Context, srcs []Source, batch *database.Batch, u *url.URL, opts Options) (*Pending, int, error) {
	if len(srcs) == 0 {
		return nil, -1, errors.BadRequest.With("pull.FetchFrom: at least one source required")
	}
	var refusals []error
	for i, src := range srcs {
		p, err := Fetch(ctx, src, batch, u, opts, opts.Verify != nil)
		if err == nil {
			return p, i, nil
		}
		if ctx.Err() != nil {
			return nil, -1, errors.UnknownError.Wrap(err)
		}
		refusals = append(refusals, errors.UnknownError.WithFormat("source %d: %w", i, err))
	}
	return nil, -1, errors.Conflict.WithFormat("%v: no source served it: %w", u, stderrors.Join(refusals...))
}

// pullMain stores the account body and returns the receipt the peer served
// with it, which binds the body to the peer's BPT root. wantReceipt asks for
// one; without it the peer does the work of building a proof nobody checks.
func pullMain(ctx context.Context, src Source, batch *database.Batch, u *url.URL, wantReceipt bool, atBlock uint64) (*api.Receipt, error) {
	var query *api.DefaultQuery
	if wantReceipt {
		// ForHeight when the caller named a block, ForAny otherwise. See
		// Options.AtBlock: ForAny is the pull that a hot account can never
		// settle, and it is kept only for callers with no verified anchor to
		// ask at (tests, and the spine's own first read).
		ro := &api.ReceiptOptions{ForAny: true}
		if atBlock != 0 {
			ro = &api.ReceiptOptions{ForHeight: atBlock}
		}
		query = &api.DefaultQuery{IncludeReceipt: ro}
	}
	rec, err := src.QueryAccount(ctx, u, query)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query account: %w", err)
	}

	// A peer that answers with an empty record has served nothing, and serving
	// nothing is a failure of that source, not an account with no state. It
	// used to return success with no receipt, and a fetch with no receipt
	// settles against any root at all, including one nobody anchored.
	if rec == nil || rec.Account == nil {
		return nil, errors.NotFound.WithFormat("%v: the peer served no account", u)
	}
	if wantReceipt && rec.Receipt == nil {
		return nil, errors.Conflict.WithFormat("%v: the peer served no receipt", u)
	}

	// Asked as of a block, answered as of the node's present. ForHeight is
	// the block the answer is FOR -- the last block at or before the one
	// asked for in which this account changed, which is exact rather than
	// approximate, because a block that changed nothing carries its
	// predecessor's root. LocalBlock is a different thing and always has
	// been: the latest block the SERVING NODE has indexed, which is its
	// present (internal/api/v3/querier.go, historicalStateReceipt).
	//
	// So a zero ForHeight on a request that named a block means the peer
	// served its current state and not the state asked for, and settling
	// that would verify against a root the caller never chose. A ForHeight
	// past the block asked for is the same failure in the other direction.
	// Both are refusals, and the caller asks another source.
	if atBlock != 0 && rec.Receipt != nil {
		switch {
		case rec.Receipt.ForHeight == 0:
			return nil, errors.Conflict.WithFormat(
				"%v: asked for the state as of block %d and the peer served its current state",
				u, atBlock)
		case rec.Receipt.ForHeight > atBlock:
			return nil, errors.Conflict.WithFormat(
				"%v: asked for the state as of block %d and the peer answered for block %d",
				u, atBlock, rec.Receipt.ForHeight)
		}
	}
	if err := batch.Account(u).Main().Put(rec.Account); err != nil {
		return nil, errors.UnknownError.WithFormat("store main: %w", err)
	}
	return rec.Receipt, nil
}

// pullDirectory replaces the account's directory list with the peer's. It
// replaces rather than adds: a node re-pulling an account it already holds —
// which is the restart case, and the point of the sync — would otherwise keep
// the entries the peer has dropped, and an account holding one entry more than
// the peer's does not hash into the anchored root, against that peer or any
// other, for as long as the node runs.
func pullDirectory(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	var all []*url.URL
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
			break
		}
		for _, r := range page.Records {
			if r == nil || r.Value == nil {
				continue
			}
			all = append(all, r.Value)
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return errors.UnknownError.Wrap(batch.Account(u).Directory().Put(all))
}

// pullPending replaces the account's pending list with the peer's, for the
// reason pullDirectory replaces its directory.
func pullPending(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) error {
	var all []*url.TxID
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
			break
		}
		for _, r := range page.Records {
			if r == nil || r.Value == nil {
				continue
			}
			all = append(all, r.Value)
			// TODO #3999: also pull each pending tx's sig-material
			// (ValidatorSignatures, Payments, Votes, Signatures) so
			// hashPendingV2 can compute the same per-account hash as
			// the source. Without that, accounts with non-empty
			// Pending diverge and the launcher never promotes.
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return errors.UnknownError.Wrap(batch.Account(u).Pending().Put(all))
}

// meeting is where the node's chains stand against one peer's, for one
// account. It is the account-level meeting point, collected chain by chain:
// the node may hold a chain the peer cannot serve to its end (beyond), and the
// peer may hold entries of a chain the node did not have (filled).
//
// One chain of each is a contradiction, not an account state — see past.
type meeting struct {
	beyondChain string // the first chain the node holds past this peer's end
	filledChain string // the first chain the peer had entries the node lacked
}

// beyond records that the node holds a chain past the end of the peer's.
func (m *meeting) beyond(name string) {
	if m.beyondChain == "" {
		m.beyondChain = name
	}
}

// filled records that the peer held entries of a chain the node did not have.
func (m *meeting) filled(name string) {
	if m.filledChain == "" {
		m.filledChain = name
	}
}

// past reports whether the node is past this peer for the account as a whole:
// at or beyond it on every chain, and strictly beyond on at least one. That is
// the account-level meeting point, and it is what says the state the node
// holds is the later of the two.
//
// Beyond on one chain and behind on another is neither side's account, and it
// is refused rather than written. The prior reading was that a coherent node
// cannot be in that state — every chain of an account grows with the blocks
// that touch it, so a node that executed further is at or beyond the peer on
// all of them — and that reading is right about a node. It is not right about
// a PEER: ModeFullSpine is pulled with no Verify and settled with Keep, so one
// dishonest source that is a single entry ahead on any one chain and behind on
// another used to choose the fall-through, and the fall-through wrote its body
// over the node's own executed height (#4344). A source that cannot be
// reconciled is refused, and AccountFrom asks the next one.
func (m *meeting) past(u *url.URL) (bool, error) {
	if m.beyondChain != "" && m.filledChain != "" {
		return false, errors.Conflict.WithFormat(
			"%v: the node is past this peer on %s and behind it on %s; the peer's state and the node's cannot both be this account's, and a mixture of the two is neither",
			u, m.beyondChain, m.filledChain)
	}
	return m.beyondChain != "", nil
}

// localChains is the height of every chain the node already holds for the
// account, keyed by lower-case name — ChainByName lower-cases, so the peer's
// spelling and the node's index must be compared that way.
func localChains(batch *database.Batch, u *url.URL) (map[string]int64, error) {
	index, err := batch.Account(u).Chains().Get()
	if err != nil {
		return nil, fmt.Errorf("read the local chain index: %w", err)
	}
	out := make(map[string]int64, len(index))
	for _, cm := range index {
		if cm == nil || cm.Name == "" {
			continue
		}
		c, err := batch.Account(u).ChainByName(cm.Name)
		if err != nil {
			return nil, fmt.Errorf("local chain %s: %w", cm.Name, err)
		}
		head, err := c.Inner().Head().Get()
		if err != nil {
			return nil, fmt.Errorf("local chain %s: load the head: %w", cm.Name, err)
		}
		out[strings.ToLower(cm.Name)] = head.Count
	}
	return out, nil
}

// unserved records, in the meeting, every chain the node holds entries of that
// the peer did not serve at all. A peer that cannot name a chain the node has
// is a peer the node is past on that chain — including the peer that serves no
// chains whatever, which would otherwise have its body installed over an
// account the node holds in full.
//
// An account with no chains at all is not that: acc://dn.acme/ledger/1 and
// every account created without a transaction of its own has an empty chain
// list, legitimately, and a node bootstrapping one must take the peer's body.
// The distinction is what the NODE holds, not what the peer serves.
func (m *meeting) unserved(mine map[string]int64) {
	names := make([]string, 0, len(mine))
	for name, height := range mine {
		if height > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names) // A refusal must name the same chain every time
	for _, name := range names {
		m.beyond(name)
	}
}

// agreesAt is the meeting point for one chain, for a node at or past the
// peer's height: the local state after the peer's Count entries against the
// head the peer served. Below that height the two must agree; above it the
// peer has no opinion.
//
// Agreement must be SHOWN. A node that cannot compute its own state at the
// peer's height — a chain held only from a mark point on, which is what a
// chain restored head-first is — refuses, because being ahead is only safe
// when the prefix can be checked. StateAt says so with an error for some of
// those and with a state of the wrong height for others, so both are checked.
//
// A Merkle state at height N commits to every entry below N, so a chain that
// agrees there and forks below it is a hash collision.
func agreesAt(dst *database.MerkleManager, head, want *merkle.State) error {
	mine := head
	if head.Count > want.Count {
		var err error
		mine, err = dst.StateAt(want.Count - 1)
		if err == nil && mine.Count != want.Count {
			err = errors.NotFound.WithFormat(
				"the local chain holds no state at %d, only at %d", want.Count, mine.Count)
		}
		if err != nil {
			// Without the local state at the peer's height there is no
			// telling a node that is simply ahead from one holding
			// different history — and that difference is what this check
			// exists for. Refuse rather than assume.
			return errors.Conflict.WithFormat(
				"the local chain is at %d and the peer served %d, and the local state at %d cannot be read, so the two cannot be compared: %w",
				head.Count, want.Count, want.Count, err)
		}
		// StateAt does not always say when it cannot answer. Below the first
		// mark point it holds, it replays from an empty state and no hashes,
		// and hands back a state of the right HEIGHT built out of nothing —
		// which anchors to something neither side has, and would be reported
		// as a disagreement the node has not established. The entry itself is
		// the check that the node holds data down there at all.
		if _, err := dst.Entry(want.Count - 1); err != nil {
			return errors.Conflict.WithFormat(
				"the local chain is at %d and the peer served %d, and the local entry %d cannot be read, so the two cannot be compared: %w",
				head.Count, want.Count, want.Count-1, err)
		}
	}
	if !bytes.Equal(mine.Anchor(), want.Anchor()) {
		return errors.Conflict.WithFormat(
			"the local chain of %d entries and the peer's of %d disagree at %d: the local chain anchors to %x there and the peer's head to %x",
			head.Count, want.Count, want.Count, mine.Anchor(), want.Anchor())
	}
	return nil
}

// pullChainHeads sets each of the account's chains from ChainRecord.{Count,
// State} plus the entries of its open mark set. It skips the entries below the
// last mark point: the BPT-leaf hash is over a chain's head anchor, which is
// computed from Pending alone (internal/database/observer_prod.go, hashChains),
// so the leaf is reproduced without them.
//
// The open mark set is not optional. A chain given a head and no elements
// cannot be appended to — an append rebuilds its Tail chunk from the elements
// of the open set — so a node joined with such a chain could not execute block
// Q+1 (merkle.Chain.RestoreHead).
//
// It honours the meeting point, chain by chain, as the spine fill does. It did
// not: it restored every head the peer served unconditionally, so a node that
// had executed further had the chain SHORTENED to the peer's height — and the
// shortening cannot be caught downstream, because the rewound account hashes
// to exactly the leaf the peer's receipt proves (#4348). A chain the node is
// at or past the peer on is left alone; the head the peer served is one this
// chain has already passed through, and a head that it has not is a
// disagreement and is refused.
func pullChainHeads(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) (*meeting, error) {
	// Empty ChainQuery requests "list all chains for this account".
	// Setting Range here triggers the v3 validator's "name is required
	// when querying by index, entry, or range" rejection — Range is
	// for entries within a named chain, not for the chain list.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	mine, err := localChains(batch, u)
	if err != nil {
		return nil, err
	}
	m := new(meeting)
	if chains != nil {
		for _, c := range chains.Records {
			if c == nil || c.Name == "" {
				continue
			}
			delete(mine, strings.ToLower(c.Name))
			want := &merkle.State{
				Count:   int64(c.Count),
				Pending: c.State,
			}
			dstChain, err := batch.Account(u).ChainByName(c.Name)
			if err != nil {
				return nil, fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			inner := dstChain.Inner()
			head, err := inner.Head().Get()
			if err != nil {
				return nil, fmt.Errorf("chain %s/%s: load the local head: %w", u, c.Name, err)
			}

			if head.Count >= want.Count {
				// The meeting point, reached at or before the peer's
				// height. Nothing is fetched and nothing is written.
				if err := agreesAt(inner, head, want); err != nil {
					return nil, fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
				}
				if head.Count > want.Count {
					m.beyond(c.Name)
				}
				if err := addChainToIndex(batch, u, c); err != nil {
					return nil, err
				}
				continue
			}

			m.filled(c.Name)
			lastMark := want.Count &^ inner.MarkMask()
			open, err := chainEntries(ctx, src, u, c.Name, uint64(lastMark), uint64(want.Count), pageSize)
			if err != nil {
				return nil, fmt.Errorf("chain %s/%s: open mark set: %w", u, c.Name, err)
			}
			if err := inner.RestoreHead(want, open); err != nil {
				return nil, fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			if err := addChainToIndex(batch, u, c); err != nil {
				return nil, err
			}
		}
	}
	m.unserved(mine)
	return m, nil
}

// chainEntries reads the entries [start, end) of one of the peer's chains.
func chainEntries(ctx context.Context, src Source, u *url.URL, chainName string, start, end, pageSize uint64) ([][]byte, error) {
	var out [][]byte
	for start < end {
		count := pageSize
		if count > end-start {
			count = end - start
		}
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
			return nil, fmt.Errorf("query entries from %d: %w", start, err)
		}
		if page == nil || len(page.Records) == 0 {
			return nil, fmt.Errorf("the peer served no entry at %d of %d", start, end)
		}
		for _, e := range page.Records {
			if e == nil {
				return nil, fmt.Errorf("the peer served a nil entry at %d", start)
			}
			if e.Index != start {
				return nil, fmt.Errorf("the peer served entry %d where %d was asked for", e.Index, start)
			}
			entry := e.Entry
			out = append(out, entry[:])
			start++
			if start >= end {
				break
			}
		}
	}
	return out, nil
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

// pullChainsFull replays every entry of every chain the node does not already
// hold. It starts from the local height, not from zero, so a second pull of an
// account is a no-op rather than a chain of twice the height — a restarting
// node re-pulls the spine, and a pull that is not idempotent doubles it.
//
// The result is held to the peer's word for the chain's head: after the
// replay the local anchor must equal the anchor of the head the peer served.
// A local chain that holds a different prefix is refused rather than extended
// into a chain neither side has. A local chain that is merely ahead of the
// peer's is the meeting point reached early — see pullChainEntries.
//
// It collects the account-level meeting point as it goes: which chains the
// node holds past this peer's end, and which the peer had entries for that the
// node did not. See meeting.past.
func pullChainsFull(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64) (*meeting, error) {
	// Empty ChainQuery: list-all-chains. See pullChainHeads.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	mine, err := localChains(batch, u)
	if err != nil {
		return nil, err
	}
	m := new(meeting)
	if chains != nil {
		for _, c := range chains.Records {
			if c == nil || c.Name == "" {
				continue
			}
			delete(mine, strings.ToLower(c.Name))
			dstChain, err := batch.Account(u).ChainByName(c.Name)
			if err != nil {
				return nil, fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			// came in at is the node's height for this chain BEFORE the fill.
			cameInAt, err := pullChainEntries(ctx, src, dstChain.Inner(), u, c, pageSize)
			if err != nil {
				return nil, fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			switch {
			case cameInAt > int64(c.Count):
				m.beyond(c.Name)
			case cameInAt < int64(c.Count):
				m.filled(c.Name)
			}
			// Equal is neither: a chain that did not move between the peer's
			// block and the node's says nothing about which of them is later.
			if err := addChainToIndex(batch, u, c); err != nil {
				return nil, err
			}
		}
	}
	m.unserved(mine)
	return m, nil
}

// pullChainEntries brings one chain up to the height the peer served, from
// whatever the node already holds, and checks the result against the peer's
// head.
//
// Syncing and bootstrapping are one walk at two depths: the node fills entries
// back from the head until it meets data it already has. A bootstrapping node
// never meets any, so it collects everything; a restarted node meets its own
// at once. The meeting point is therefore the only part of the walk a restart
// exercises, and the only part a bootstrap does not — which is why bootstrap
// passed while a twelve-node restart could not pull a spine account at all.
//
// Two different things can be true of a chain the node already holds, and they
// get different answers:
//
//   - The local chain is AHEAD of the peer's — the peer serves c.Count
//     entries and the node holds those same entries and more. That is the
//     meeting point reached early. It is not an error: everything this peer
//     can give for this chain, the node has. Nothing is fetched and nothing is
//     appended.
//
//   - The local chain DISAGREES with the peer's at a position they both hold.
//     That is two nodes holding different history, and it is refused however
//     long either chain is. Refusing is the whole point of the check, so a
//     case where agreement cannot be established is refused too.
//
// Which of the two it is, is decided at the PEER'S height, not the node's: the
// local state after c.Count entries against the head the peer served. Below
// that height the two must agree; above it the peer has no opinion.
//
// What this cannot decide is WHY the node is ahead — whether it executed
// further before it stopped, or an earlier pull wrote a peer's entries into
// its chain. Both leave a chain that agrees with this peer everywhere this
// peer can speak, and nothing in a chain records which of the two put an entry
// there. The check here is the one that can be made: agreement wherever the
// peer has an opinion.
//
// It returns the height the node came in at, which the caller compares with
// the peer's to decide the account-level meeting point.
func pullChainEntries(ctx context.Context, src Source, dst *database.MerkleManager, u *url.URL, c *api.ChainRecord, pageSize uint64) (int64, error) {
	head, err := dst.Head().Get()
	if err != nil {
		return 0, fmt.Errorf("load the local head: %w", err)
	}
	// The height the node comes in at, taken as a number before anything is
	// appended: Head().Get() hands back the manager's own state, and every
	// AddEntry below advances it, so head.Count is not the starting height
	// once the fill has run.
	from := head.Count
	want := &merkle.State{Count: int64(c.Count), Pending: c.State}

	// At or past the peer's height: the meeting point, discharged where the
	// peer has something to say. The same discharge the head-only pull makes
	// (pullChainHeads) — one rule, one implementation.
	if from >= int64(c.Count) {
		return from, errors.UnknownError.Wrap(agreesAt(dst, head, want))
	}

	entries, err := chainEntries(ctx, src, u, c.Name, uint64(from), c.Count, pageSize)
	if err != nil {
		return from, err
	}
	for i, e := range entries {
		if err := dst.AddEntry(e, false); err != nil {
			return from, fmt.Errorf("add entry %d: %w", from+int64(i), err)
		}
	}

	// The peer's head is what the account's leaf is hashed from, so a replay
	// that does not reproduce it has built a different chain. The fill only
	// appended what the peer served, so the disagreement is below the height
	// the node came in at — the two hold different history there.
	got, err := dst.Head().Get()
	if err != nil {
		return from, fmt.Errorf("load the rebuilt head: %w", err)
	}
	if !bytes.Equal(got.Anchor(), want.Anchor()) {
		return from, errors.Conflict.WithFormat(
			"the local chain of %d entries is not a prefix of the peer's: after replaying the peer's entries [%d, %d) the chain anchors to %x and the peer's head to %x, so they disagree below %d",
			from, from, c.Count, got.Anchor(), want.Anchor(), from)
	}
	return from, nil
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

		// The network definition and the globals, because they are what says
		// who may sign an anchor and how many of them are needed. A node
		// holds its own from genesis or from its own execution, and the only
		// way that copy ever moves is this one: pulled with a receipt that
		// ends at a root a quorum signed and passes through the leaf the
		// pulled body hashes to, then handed to anchorsrc.Authority. Past
		// Vandenberg a change to them never travels in an anchor
		// (block_end.go:791-793), so this is the whole of how a joining node
		// crosses one (#4301).
		partitionURL.JoinPath(protocol.Network),
		partitionURL.JoinPath(protocol.Globals),
	}
}

// DnSpineAccounts is a backward-compatible helper returning
// SpineAccounts(protocol.DnUrl()).
func DnSpineAccounts() []*url.URL { return SpineAccounts(protocol.DnUrl()) }
