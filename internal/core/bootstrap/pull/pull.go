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
//     spine, and NOT for verification — verifying an anchor needs only the
//     validator set the node already holds (anchorsrc.Authority), never a key
//     page of the time. The chains are taken because these are the accounts
//     every block touches: a node holding only their heads cannot show that
//     its own history agrees with a peer's below the peer's height, and the
//     pull refuses what it cannot compare (see meeting.past).
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
}

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
// THERE IS NO EXEMPTION. Every pull either verifies against the anchored root
// or is refused. The "past" case -- the node is beyond this peer on the
// account, so nothing was taken and nothing was checked -- existed because
// accounts were compared against a peer's CURRENT state, which is a moving
// target no anchor covers. At a fixed anchored block there is one correct leaf
// per account and no ordering question: the account hashes into that block's
// root or it is asked of somebody else (#4348 x2, #4350).
func (p *Pending) Settle(anchoredRoot [32]byte) error {
	if p.done {
		return errors.NotAllowed.WithFormat("%v: already settled", p.Account)
	}
	p.release()
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

// Keep writes the state into the caller's batch without verifying it.
//
// **Nothing in production calls it.** It was for the spine, on the rationale
// that the spine is what the verifier reads from; that rationale was false —
// the verifier reads its keys from the node's own store — and it is what
// #4301 closed. What is left is the pull-library tests, which have no anchors
// to verify against.
func (p *Pending) Keep() error {
	if p.done {
		return errors.NotAllowed.WithFormat("%v: already settled", p.Account)
	}
	p.release()
	defer p.batch.Discard()
	return errors.UnknownError.Wrap(p.batch.Commit())
}

// Discard throws the pulled state away.
func (p *Pending) Discard() {
	if p.done {
		return
	}
	p.release()
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
		// THE BLOCK THIS STATE IS SETTLED AGAINST IS THE BLOCK THIS NODE
		// ASKED AT, and nothing in the answer decides it (#4362, the #4361
		// threat review's F1).
		//
		// The answer carries two block numbers and both are the peer's word:
		// ForHeight, the block the peer says it resolved the request to, and
		// LocalBlock, the peer's own present. A consumer that keys the
		// anchored-root lookup on either of them lets the peer choose which
		// anchored root it is checked against: it answers at an older
		// anchored B', every account verifies because the receipt genuinely
		// ends at a root the joiner holds, the joiner's root becomes
		// root(B'), and it then executes a block it never collected.
		//
		// It costs nothing to refuse them. If the peer resolves backwards to
		// B < AtBlock, either root(B) == root(AtBlock) -- nothing changed in
		// between, so the state at B IS the state at AtBlock -- or the
		// receipt does not end at the root the caller holds for AtBlock and
		// the account is refused. Either way the puller needs no field from
		// the peer to know it.
		//
		// Zero AtBlock is the caller with no verified anchor to ask at
		// (tests). There the peer's current state is all there is, so the
		// answer's own numbers are used, as they were before.
		switch {
		case opts.AtBlock != 0:
			p.Block = opts.AtBlock
			p.Partition = opts.Partition
		default:
			p.Block = receipt.LocalBlock
			if receipt.ForHeight != 0 {
				p.Block = receipt.ForHeight
			}
			// The block is the SERVING partition's, not the puller's. A
			// receipt proves the state as of a block of the partition that
			// built it, and block numbers collide across partitions (#4308).
			if receipt.Partition != "" {
				p.Partition = protocol.PartitionUrl(receipt.Partition)
			}
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

	// 4. The chains, as of the same block the body is. They are part of the
	// account's leaf, so a body from block B beside chains from the peer's
	// present is a value no node ever held (see queryAccountChainsAt).
	switch opts.Mode {
	case ModeStateOnly:
		err = pullChainHeads(ctx, src, sub, u, pageSize, opts)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chain heads %s: %w", u, err))
		}
	case ModeFullSpine:
		err = pullChainsFull(ctx, src, sub, u, pageSize, opts)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chains full %s: %w", u, err))
		}
	default:
		return fail(errors.BadRequest.WithFormat("unknown pull mode %d", opts.Mode))
	}

	return p, nil
}

// The account-level meeting point is gone.
//
// It asked "am I ahead of this peer, level with it, or behind it" for an
// account as a whole, and every answer it could give was wrong at least
// sometimes: refusing the ahead case stranded every restart (#4348), taking
// the peer's body in the level case re-stamped an anchor sequence number the
// network had already seen, because an account's body moves with all of its
// chains standing still (#4350), and the whole question only existed because
// accounts were compared against a peer's CURRENT state, which is a moving
// target that no anchor covers. At an anchored block there is one correct leaf
// per account. The node hashes to it or it does not, and it asks somebody else
// (executor spec, "Sync", §2).
func removedMeetingPoint() {}

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
		// No production caller reaches this: the join passes Verify on every
		// path, spine included (#4301). It is kept for the pull-library
		// tests, which build a peer and a store and have no anchors to
		// verify against.
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

// agreesAt checks that a local chain already at the served length holds the
// same history: the local head against the head the peer served for that
// length.
//
// A Merkle state at height N commits to every entry below N, so a chain that
// agrees there and forks below it is a hash collision.
func agreesAt(head, want *merkle.State) error {
	if !bytes.Equal(head.Anchor(), want.Anchor()) {
		return errors.Conflict.WithFormat(
			"the local chain of %d entries and the peer's of %d disagree at %d: the local chain anchors to %x there and the peer's head to %x",
			head.Count, want.Count, want.Count, head.Anchor(), want.Anchor())
	}
	return nil
}

// pullChainHeads sets each of the account's chains from ChainRecord.{Count,
// State} plus the entries of its open mark set, AT THE BLOCK THE PULL ASKED
// FOR. It skips the entries below the last mark point: the BPT-leaf hash is
// over a chain's head anchor, which is computed from Pending alone
// (internal/database/observer_prod.go, hashChains), so the leaf is reproduced
// without them.
//
// The open mark set is not optional. A chain given a head and no elements
// cannot be appended to -- an append rebuilds its Tail chunk from the elements
// of the open set -- so a node joined with such a chain could not execute
// block Q+1 (merkle.Chain.RestoreHead).
//
// THREE CASES AND NO ORDERING QUESTION. At the block that was asked for, a
// chain had one length and one head, and the node's chain is shorter than it,
// equal to it, or longer than it:
//
//   - Shorter: the entries between are fetched and the head restored. That is
//     the ordinary case, and for a restarted node it is a handful of entries.
//   - Equal: the two must anchor to the same value, or the node and the peer
//     hold different history and the account is refused.
//   - Longer: the node holds entries the block did not. A joining node has
//     executed nothing since it stopped, and it stopped at or before the
//     block, so this is a peer serving a length that is not the block's, or a
//     node holding something no node held. It is refused, never shortened:
//     rewinding a chain the node built hashes to exactly the leaf the peer's
//     receipt proves, so nothing downstream would catch it (#4348).
//
// A chain the peer serves with no entries did not exist at the block. It is
// skipped rather than indexed, because an account's chain index is what
// hashChains walks: indexing a chain that was not there puts an empty chain's
// hash into a leaf that never had one.
func pullChainHeads(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64, opts Options) error {
	// Empty ChainQuery requests "list all chains for this account".
	// Setting Range here triggers the v3 validator's "name is required
	// when querying by index, entry, or range" rejection -- Range is
	// for entries within a named chain, not for the chain list.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{ForHeight: opts.AtBlock})
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}
	mine, err := localChains(batch, u)
	if err != nil {
		return err
	}
	if chains != nil {
		for _, c := range chains.Records {
			if c == nil || c.Name == "" {
				continue
			}
			held := mine[strings.ToLower(c.Name)]
			delete(mine, strings.ToLower(c.Name))
			if c.Count == 0 {
				if held > 0 {
					return errors.Conflict.WithFormat(
						"chain %s/%s: the local chain holds %d entries and the peer serves none for the block asked for",
						u, c.Name, held)
				}
				continue
			}
			want := &merkle.State{Count: int64(c.Count), Pending: c.State}
			dstChain, err := batch.Account(u).ChainByName(c.Name)
			if err != nil {
				return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			inner := dstChain.Inner()
			head, err := inner.Head().Get()
			if err != nil {
				return fmt.Errorf("chain %s/%s: load the local head: %w", u, c.Name, err)
			}

			switch {
			case head.Count > want.Count:
				return errors.Conflict.WithFormat(
					"chain %s/%s: the local chain holds %d entries and the block asked for held %d; a joining node cannot be past the block it is pulling",
					u, c.Name, head.Count, want.Count)

			case head.Count == want.Count:
				if err := agreesAt(head, want); err != nil {
					return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
				}

			default:
				lastMark := want.Count &^ inner.MarkMask()
				open, err := chainEntries(ctx, src, u, c.Name, uint64(lastMark), uint64(want.Count), pageSize)
				if err != nil {
					return fmt.Errorf("chain %s/%s: open mark set: %w", u, c.Name, err)
				}
				if err := inner.RestoreHead(want, open); err != nil {
					return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
				}
			}
			if err := addChainToIndex(batch, u, c); err != nil {
				return err
			}
		}
	}

	// A chain the node holds that the peer did not name at all. An account's
	// chain index only grows, so at the block the peer served there is no such
	// thing: this is two nodes holding different accounts.
	return unservedChains(u, mine)
}

// unservedChains refuses an account whose chains the peer could not name.
func unservedChains(u *url.URL, mine map[string]int64) error {
	names := make([]string, 0, len(mine))
	for name, height := range mine {
		if height > 0 {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names) // A refusal must name the same chain every time
	return errors.Conflict.WithFormat(
		"%v: the local account holds chains the peer serves none of for the block asked for: %v", u, names)
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
// hold, up to the length the chain held AT THE BLOCK THE PULL ASKED FOR. It
// starts from the local height, not from zero, so a second pull of an account
// is a no-op rather than a chain of twice the height -- a restarting node
// re-pulls the spine, and a pull that is not idempotent doubles it.
//
// The result is held to the head the peer served for that block: after the
// replay the local anchor must equal it. A local chain that holds a different
// prefix is refused rather than extended into a chain neither side has, and
// one longer than the block's is refused rather than shortened -- the three
// cases are pullChainHeads's.
func pullChainsFull(ctx context.Context, src Source, batch *database.Batch, u *url.URL, pageSize uint64, opts Options) error {
	// List-all-chains, as of the block that was asked for. See pullChainHeads.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{ForHeight: opts.AtBlock})
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}
	mine, err := localChains(batch, u)
	if err != nil {
		return err
	}
	if chains != nil {
		for _, c := range chains.Records {
			if c == nil || c.Name == "" {
				continue
			}
			held := mine[strings.ToLower(c.Name)]
			delete(mine, strings.ToLower(c.Name))
			if c.Count == 0 {
				if held > 0 {
					return errors.Conflict.WithFormat(
						"chain %s/%s: the local chain holds %d entries and the peer serves none for the block asked for",
						u, c.Name, held)
				}
				continue
			}
			dstChain, err := batch.Account(u).ChainByName(c.Name)
			if err != nil {
				return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			err = pullChainEntries(ctx, src, dstChain.Inner(), u, c, pageSize)
			if err != nil {
				return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
			}
			if err := addChainToIndex(batch, u, c); err != nil {
				return err
			}
		}
	}
	return unservedChains(u, mine)
}

// pullChainEntries brings one chain up to the length it held at the block the
// pull asked for, from whatever the node already holds, and checks the result
// against the head the peer served for that block.
//
// Syncing and bootstrapping are one walk at two depths: the node fills entries
// back from the head until it meets data it already has. A bootstrapping node
// never meets any, so it collects everything; a restarted node meets its own
// at once. The meeting point is therefore the only part of the walk a restart
// exercises, and the only part a bootstrap does not -- which is why bootstrap
// passed while a twelve-node restart could not pull a spine account at all.
func pullChainEntries(ctx context.Context, src Source, dst *database.MerkleManager, u *url.URL, c *api.ChainRecord, pageSize uint64) error {
	head, err := dst.Head().Get()
	if err != nil {
		return fmt.Errorf("load the local head: %w", err)
	}
	// The height the node comes in at, taken as a number before anything is
	// appended: Head().Get() hands back the manager's own state, and every
	// AddEntry below advances it, so head.Count is not the starting height
	// once the fill has run.
	from := head.Count
	want := &merkle.State{Count: int64(c.Count), Pending: c.State}

	switch {
	case from > want.Count:
		return errors.Conflict.WithFormat(
			"the local chain holds %d entries and the block asked for held %d; a joining node cannot be past the block it is pulling",
			from, want.Count)
	case from == want.Count:
		return agreesAt(head, want)
	}

	entries, err := chainEntries(ctx, src, u, c.Name, uint64(from), c.Count, pageSize)
	if err != nil {
		return err
	}
	for i, e := range entries {
		if err := dst.AddEntry(e, false); err != nil {
			return fmt.Errorf("add entry %d: %w", from+int64(i), err)
		}
	}

	// The peer's head is what the account's leaf is hashed from, so a replay
	// that does not reproduce it has built a different chain. The fill only
	// appended what the peer served, so the disagreement is below the height
	// the node came in at -- the two hold different history there.
	got, err := dst.Head().Get()
	if err != nil {
		return fmt.Errorf("load the rebuilt head: %w", err)
	}
	if !bytes.Equal(got.Anchor(), want.Anchor()) {
		return errors.Conflict.WithFormat(
			"the local chain of %d entries is not a prefix of the peer's: after replaying the peer's entries [%d, %d) the chain anchors to %x and the peer's head to %x, so they disagree below %d",
			from, from, c.Count, got.Anchor(), want.Anchor(), from)
	}
	return nil
}

// SpineAccounts is what a join takes first, in ModeFullSpine: the accounts
// every block touches, with their chains, so the node can compare its own
// history against a peer's and can append to them when it executes again.
//
// They are verified like every other account (#4301). The chains are not
// taken in order to verify anything — an anchor is checked against the
// validator set the node already holds — they are taken because a head
// without its entries cannot be reconciled with a peer's.
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
