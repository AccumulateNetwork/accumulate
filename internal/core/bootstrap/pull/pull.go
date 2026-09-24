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
//     its own history agrees with a peer's below the peer's height.
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

	// bodyless says the peer served the account with no main state (#4397),
	// whose leaf the receipt cannot bind to the account's name.
	bodyless bool
}

// Root is the root the peer's receipt ends at: the peer's word, until the
// caller proves it and settles against it. Zero when there is no receipt.
func (p *Pending) Root() [32]byte {
	var r [32]byte
	if p.receipt != nil {
		copy(r[:], p.receipt.Receipt.Anchor)
	}
	return r
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
	receipt, bodyless, err := pullMain(ctx, src, sub, u, withReceipt)
	switch {
	case err == nil:
	case stderrors.Is(err, ErrNoLeaf):
		// Kept apart from every other failure so FetchFrom can tell "no
		// source holds it" from "a source did not answer" (#4397). It is a
		// stdlib wrap on purpose: the errors package's wrapping keeps a
		// status code, not a sentinel.
		return fail(fmt.Errorf("main %s: %w", u, err))
	default:
		return fail(errors.UnknownError.WithFormat("main %s: %w", u, err))
	}
	p.receipt = receipt
	p.bodyless = bodyless
	if receipt != nil {
		p.Block = receipt.LocalBlock

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

	// 4. Chains. At an anchored height there is one correct leaf per account:
	// what is taken is the peer's, and the root it hashes into decides
	// whether it is kept (Settle). The node's own heights are not consulted.
	switch opts.Mode {
	case ModeStateOnly:
		err = pullChainHeads(ctx, src, sub, u, pageSize)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chain heads %s: %w", u, err))
		}
	case ModeFullSpine:
		err = pullChainsFull(ctx, src, sub, u, pageSize)
		if err != nil {
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
	var bodyless []*Pending
	var leaves [][]byte
	var answers []string
	noLeaf := 0
	discard := func() {
		for _, p := range bodyless {
			p.Discard()
		}
	}
	for i, src := range srcs {
		p, err := Fetch(ctx, src, batch, u, opts, opts.Verify != nil)
		if ctx.Err() != nil {
			if p != nil {
				p.Discard()
			}
			discard()
			return nil, -1, errors.UnknownError.Wrap(ctx.Err())
		}
		switch {
		case err != nil:
			if stderrors.Is(err, ErrNoLeaf) {
				noLeaf++
				answers = append(answers, fmt.Sprintf("%s: no leaf", sourceName(i, src)))
			} else {
				answers = append(answers, fmt.Sprintf("%s: failed", sourceName(i, src)))
			}
			refusals = append(refusals, errors.UnknownError.WithFormat("%s: %w", sourceName(i, src), err))

		case !p.bodyless:
			if len(bodyless) == 0 {
				return p, i, nil
			}
			answers = append(answers, fmt.Sprintf("%s: a body", sourceName(i, src)))
			p.Discard()

		default:
			leaf, err := p.batch.Account(u).StateTreeReceipt()
			if err != nil {
				p.Discard()
				discard()
				return nil, -1, errors.UnknownError.WithFormat("%v: hash the pulled state: %w", u, err)
			}
			bodyless = append(bodyless, p)
			leaves = append(leaves, leaf.Anchor)
			answers = append(answers, fmt.Sprintf("%s: no body, leaf %x", sourceName(i, src), leaf.Anchor[:8]))
		}
	}

	// A leaf with no body is not bound to its account's name: the tree hashes
	// values, not keys, and every empty account's leaf is one hash, so one
	// peer can serve an empty account's receipt under a name the tree has no
	// leaf for and pass the leaf check (#4397 review F3). It is kept only when
	// EVERY source asked served the same body-less leaf. One dissent -- a
	// NotFound, a body, another leaf, or no answer -- and the name is neither
	// written nor dropped: it is asked again.
	if len(bodyless) > 0 {
		same := len(bodyless) == len(srcs)
		for _, l := range leaves {
			same = same && bytes.Equal(l, leaves[0])
		}
		if same {
			for _, p := range bodyless[1:] {
				p.Discard()
			}
			return bodyless[0], 0, nil
		}
		discard()
		return nil, -1, fmt.Errorf("%v: %w: %s", u, ErrDissent, strings.Join(answers, "; "))
	}

	if noLeaf == len(srcs) {
		return nil, -1, fmt.Errorf("%v: %w, by every source asked: %v", u, ErrNoLeaf, stderrors.Join(refusals...))
	}
	return nil, -1, errors.Conflict.WithFormat("%v: no source served it: %w", u, stderrors.Join(refusals...))
}

// ErrDissent says the sources asked for an account did not all serve the
// same body-less leaf for it, where one of them served one. Such a leaf is
// only kept on every source's word (#4397 review F3), so the name is asked
// again; the error names each source and what it answered.
var ErrDissent = stderrors.New("the sources do not agree on a leaf with no body")

// sourceName is how a source is named in an error: by the peer it reaches,
// when it says, and by its place in the list otherwise.
func sourceName(i int, src Source) string {
	if s, ok := src.(fmt.Stringer); ok {
		return fmt.Sprintf("source %d (%s)", i, s.String())
	}
	return fmt.Sprintf("source %d", i)
}

// ErrNoLeaf is a peer's answer that its state tree holds no leaf for an
// account: NotFound to a request for the account with a receipt, which a peer
// that holds a leaf never gives (executor.md, "Sync", §2; #4397). FetchFrom
// returns it only when every source answered so; one source that failed to
// answer makes it an ordinary refusal, to be asked again.
var ErrNoLeaf = stderrors.New("the peer holds no leaf for the account")

// pullMain stores the account body and returns the receipt the peer served
// with it, which binds the body to the peer's BPT root. wantReceipt asks for
// one; without it the peer does the work of building a proof nobody checks.
func pullMain(ctx context.Context, src Source, batch *database.Batch, u *url.URL, wantReceipt bool) (_ *api.Receipt, bodyless bool, _ error) {
	var query *api.DefaultQuery
	if wantReceipt {
		query = &api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}}
	}
	rec, err := src.QueryAccount(ctx, u, query)
	switch {
	case err == nil:
	case wantReceipt && errors.Is(err, errors.NotFound):
		// Asked for a receipt, a peer answers NotFound only when its tree
		// holds no leaf for the account (#4397). Only the account query's
		// answer means that: a NotFound from any later query is a peer that
		// could not serve part of what it holds.
		return nil, false, fmt.Errorf("%w: %v", ErrNoLeaf, err)
	default:
		return nil, false, errors.UnknownError.WithFormat("query account: %w", err)
	}

	// A peer that answers with an empty record has served nothing, and serving
	// nothing is a failure of that source, not an account with no state. It
	// used to return success with no receipt, and a fetch with no receipt
	// settles against any root at all, including one nobody anchored. It is
	// not NotFound: that is the peer's own answer that it holds no leaf, and
	// a name every source answers so is dropped (executor.md, "Sync", §2).
	if rec == nil {
		return nil, false, errors.Conflict.WithFormat("%v: the peer served no account", u)
	}
	if wantReceipt && rec.Receipt == nil {
		return nil, false, errors.Conflict.WithFormat("%v: the peer served no receipt", u)
	}

	// A leaf with no body (#4397): the peer's tree holds a leaf for an account
	// with no main state, and it served the receipt for that leaf. Nothing is
	// stored for the body, which hashes as the zero hash; the account is
	// marked dirty so its leaf is written when the pass is, and the rest of
	// it is pulled as for any account. Settle's leaf check is what decides:
	// a peer that serves no body for an account that has one serves a receipt
	// that does not pass through the leaf this state hashes to. The check
	// cannot say the leaf is this account's, so FetchFrom keeps it only on
	// every source's word.
	if rec.Account == nil {
		if !wantReceipt {
			return nil, false, errors.Conflict.WithFormat("%v: the peer served no account", u)
		}
		if err := batch.Account(u).MarkDirty(); err != nil {
			return nil, false, errors.UnknownError.WithFormat("mark dirty: %w", err)
		}
		return rec.Receipt, true, nil
	}
	if err := batch.Account(u).Main().Put(rec.Account); err != nil {
		return nil, false, errors.UnknownError.WithFormat("store main: %w", err)
	}
	if wantReceipt {
		// The answer that carries a receipt carries the rest of the leaf the
		// receipt proves; one without a receipt does not, and must not clear
		// what the node holds.
		if err := pullLeafBesideBody(ctx, src, batch, u, rec.Leaf); err != nil {
			return nil, false, errors.UnknownError.Wrap(err)
		}
	}
	return rec.Receipt, false, nil
}

// pullLeafBesideBody writes the part of a system account's leaf that only the
// account answer carries (#4399): the synthetic ledger's delivery queues and
// the partition ledger's scheduled events. It REPLACES what the node held --
// an absent or empty set served clears the node's -- for the reason
// pullDirectory replaces: a restarted node's own queue under the peer's body
// hashes into nothing anyone anchored.
//
// Each queued local delivery is executed at the next block from its stored
// message (block.drainDeliveryQueues), so the message is fetched too, and kept
// only if it hashes to the ID the verified queue names.
func pullLeafBesideBody(ctx context.Context, src Source, batch *database.Batch, u *url.URL, leaf *api.AccountLeaf) error {
	if _, ok := protocol.ParsePartitionUrl(u); !ok {
		return nil
	}
	if leaf == nil {
		leaf = new(api.AccountLeaf)
	}
	account := batch.Account(u)
	switch {
	case u.PathEqual(protocol.Synthetic):
		if err := account.LocalDeliveryQueue().Put(leaf.LocalDeliveryQueue); err != nil {
			return errors.UnknownError.WithFormat("store local delivery queue: %w", err)
		}
		if err := account.CascadeDeliveryQueue().Put(leaf.CascadeDeliveryQueue); err != nil {
			return errors.UnknownError.WithFormat("store cascade delivery queue: %w", err)
		}
		for _, id := range leaf.LocalDeliveryQueue {
			if err := pullMessage(ctx, src, batch, id); err != nil {
				return errors.UnknownError.WithFormat("queued local delivery %v: %w", id, err)
			}
		}

	case u.PathEqual(protocol.Ledger):
		if err := replaceEvents(account.Events(), leaf.Events); err != nil {
			return errors.UnknownError.WithFormat("store scheduled events: %w", err)
		}
	}
	return nil
}

// pullMessage fetches a stored message and keeps it only if it is the message
// the ID names. The hash is the whole check: a message is its hash.
func pullMessage(ctx context.Context, src Source, batch *database.Batch, id *url.TxID) error {
	rec, err := src.QueryMessage(ctx, id, nil)
	if err != nil {
		return errors.UnknownError.WithFormat("query message: %w", err)
	}
	if rec == nil || rec.Message == nil {
		return errors.Conflict.With("the peer served no message")
	}
	h := rec.Message.Hash()
	if h != id.Hash() {
		return errors.Conflict.WithFormat("the peer served a message that hashes to %x", h[:8])
	}
	return errors.UnknownError.Wrap(batch.Message(h).Main().Put(rec.Message))
}

// replaceEvents replaces a partition ledger's scheduled events with the ones
// served, through the event sets, which keep the events BPT in step: the leaf
// hashes that tree's root, and a root cannot be written, only rebuilt.
//
// The block lists the executor finds the events by are DERIVED from the
// served sets, never taken from the answer. They are an index outside the
// events BPT, so nothing the leaf check proves covers them: a peer that
// served a held vote and left its block off the list would pass the check,
// and the joined node would never release that vote at the anchor its peers
// release it at -- a different block (#4399 review, F1).
func replaceEvents(events *database.AccountEvents, ev *api.LedgerEvents) error {
	if ev == nil {
		ev = new(api.LedgerEvents)
	}

	blocks, err := events.Minor().Blocks().Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, b := range blocks {
		if err := events.Minor().Votes(b).Put(nil); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	blocks, err = events.Major().Blocks().Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, b := range blocks {
		if err := events.Major().Pending(b).Put(nil); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	if err := events.Minor().Blocks().Put(nil); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if err := events.Major().Blocks().Put(nil); err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Writing a non-empty set adds its block to the list (blockEventSet).
	for _, v := range ev.MinorVotes {
		if v == nil || len(v.Votes) == 0 {
			continue
		}
		if err := events.Minor().Votes(v.Block).Put(v.Votes); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	for _, p := range ev.MajorPending {
		if p == nil || len(p.Pending) == 0 {
			continue
		}
		if err := events.Major().Pending(p.Block).Put(p.Pending); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return errors.UnknownError.Wrap(events.Backlog().Expired().Put(ev.Expired))
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
// The head restored is the peer's whatever the node held: the node's own
// height is not compared with it. The account's leaf is hashed from these
// heads, and whether the leaf is the one the anchored root proves is what
// Settle decides.
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
		want := &merkle.State{
			Count:   int64(c.Count),
			Pending: c.State,
		}
		dstChain, err := batch.Account(u).ChainByName(c.Name)
		if err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
		inner := dstChain.Inner()
		lastMark := want.Count &^ inner.MarkMask()
		open, err := chainEntries(ctx, src, u, c.Name, uint64(lastMark), uint64(want.Count), pageSize)
		if err != nil {
			return fmt.Errorf("chain %s/%s: open mark set: %w", u, c.Name, err)
		}
		if err := inner.RestoreHead(want, open); err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
		if err := addChainToIndex(batch, u, c); err != nil {
			return err
		}
	}
	return nil
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
// A local chain that holds a different prefix, or more entries than the peer
// served, cannot be made the peer's by appending and is refused; the pull
// asks another peer, or this one again once it has moved on.
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
		if err := pullChainEntries(ctx, src, dstChain.Inner(), u, c, pageSize); err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
		if err := addChainToIndex(batch, u, c); err != nil {
			return err
		}
	}
	return nil
}

// pullChainEntries brings one chain up to the height the peer served, from
// whatever the node already holds, and checks the result against the peer's
// head.
func pullChainEntries(ctx context.Context, src Source, dst *database.MerkleManager, u *url.URL, c *api.ChainRecord, pageSize uint64) error {
	head, err := dst.Head().Get()
	if err != nil {
		return fmt.Errorf("load the local head: %w", err)
	}
	// Taken as a number before anything is appended: Head().Get() hands back
	// the manager's own state, and every AddEntry below advances it.
	from := head.Count
	if from > int64(c.Count) {
		return errors.Conflict.WithFormat(
			"the local chain is at %d and the peer served %d; it cannot be re-pulled", from, c.Count)
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
	// that does not reproduce it has built a different chain.
	got, err := dst.Head().Get()
	if err != nil {
		return fmt.Errorf("load the rebuilt head: %w", err)
	}
	want := &merkle.State{Count: int64(c.Count), Pending: c.State}
	if !bytes.Equal(got.Anchor(), want.Anchor()) {
		return errors.Conflict.WithFormat(
			"the local chain of %d entries is not a prefix of the peer's: after replaying the peer's entries [%d, %d) the chain anchors to %x and the peer's head to %x",
			from, from, c.Count, got.Anchor(), want.Anchor())
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
