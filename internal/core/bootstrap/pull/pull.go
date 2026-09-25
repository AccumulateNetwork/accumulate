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
// Verification. The join's one proof is the whole local root matching a
// signed anchor's (executor spec, "Sync", "The algorithm", step 3): what Fetch
// pulls is written with Keep, unverified account by account, and nothing
// before that whole-root match is proven or needs to be.
//
// What is pulled replaces what the node held for that account rather than
// joining with it, and a re-pull of an account is the same account: a restart
// re-pulls, and a pull that unions or appends leaves a node holding something
// no peer holds, which never hashes into an anchored root again.
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the pull
// writes through a nested batch, discarded on anything Fetch itself refuses
// (a malformed body, a mismatched name, a missing message) rather than
// straight through.
package pull

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
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

	// RetakeLonger, in ModeFullSpine, takes a chain the node holds more
	// entries of than the peer served again whole, from its first entry,
	// instead of refusing the peer, and replaces the account's chain index
	// with the peer's, so a chain only the node holds is dropped. A joining node that executed blocks and
	// is repairing what it executed from the block ledger sets it (executor
	// spec, "Sync", "Two mismatches"): its chains may
	// carry entries of its own that no peer has, and refusing every peer
	// would leave the account wrong for good.
	RetakeLonger bool

	// WithReceipt asks each source for the receipt that binds the account to
	// its BPT root. The answer that carries a receipt is the one that carries
	// the rest of the leaf beside the body (#4399), and the one whose
	// NotFound means the peer holds no leaf (ErrNoLeaf, #4397). The join asks
	// for it and verifies nothing against a root: its one proof is the whole
	// local root matching a signed anchor's (executor spec, "Sync", "The
	// algorithm", step 3).
	WithReceipt bool

	// Partition is the partition whose blocks the account's state belongs to.
	// Required whenever a receipt is asked for: a receipt proves the state as
	// of a block, and block numbers collide across partitions (#4205).
	Partition *url.URL

	// CheckHeld, in ModeFullSpine, also checks the newest HeldCheckDepth
	// entries the node already holds on each transaction chain, and fetches
	// the message behind every one that has none. A full pull resumes from
	// the local head, so without it an entry a node took without its message
	// stays that way for good: a node that joined before #4421 holds its
	// pool's entries past its first pass so, and its seed fails at every
	// process start. It is a read of the node's own store per entry, so the
	// join asks for it once per process, in the pass that carries the spine.
	CheckHeld bool

	// Store is where the entries of the chains taken in ModeFullSpine are
	// written, a page at a time, each page committed before the next is asked
	// for (#4446): the node's database, for a pull whose memory must not grow
	// with the history it takes. Nil writes them into the caller's batch,
	// which holds them until it commits.
	Store Store
}

// Store begins the batches a pull writes chain entries into, a page at a
// time. A *database.Database writes each page to disk; a *database.Batch
// writes it into itself, and holds it.
type Store interface {
	Begin(writable bool) *database.Batch
}

// HeldCheckDepth is how many of a chain's newest held entries CheckHeld
// checks for their messages. It reaches past what the first block a process
// opens reads of the anchor pool: the seed walks the pool newest-first down
// to at most synthcache.DefaultHorizon (600) of the partition's own blocks,
// and a Directory's pool takes about one anchor a block from each partition,
// so 600 blocks of a network of up to five partitions is under 4096 entries.
const HeldCheckDepth = 4096

// Pending is state pulled from a peer and not yet kept. It sits in a batch of
// its own; nothing of it reaches the caller's batch until Keep writes it or
// Discard throws it away. What lies under a chain's head is not in it: the
// entries of a chain taken whole, the messages behind them and what executing
// them wrote are written to Options.Store a page at a time as they arrive
// (#4446), and stay written whatever becomes of the Pending. They are placed
// under a head the node does not hold until Keep writes it.
//
// It exists because the pull runs ahead of the join's own proof. The join
// does not settle any one account: its one proof is the whole local root
// matching a signed anchor's, checked once the spine is whole (executor
// spec, "Sync", "The algorithm", step 3).
type Pending struct {
	// Account is the account that was pulled.
	Account *url.URL

	// Partition is the partition whose block Block is. Block numbers collide
	// across partitions — with a one-second cadence the Directory and a BVN
	// are at the same number at the same second — so a block number without
	// its partition names nothing (#4205).
	Partition *url.URL

	// Block is the block the peer claims to have served the state at.
	Block uint64

	receipt *api.Receipt
	batch   *database.Batch
	done    bool
}

// Root is the root the peer's receipt ends at: the peer's word, unverified.
// Zero when there is no receipt.
func (p *Pending) Root() [32]byte {
	var r [32]byte
	if p.receipt != nil {
		copy(r[:], p.receipt.Receipt.Anchor)
	}
	return r
}

// MaxHeld bounds how many fetched-but-unkept accounts may be outstanding at
// once, across the process. Each one holds an open child batch, so the state it
// pulled is held in memory until it is kept or discarded, and an unbounded
// pull is an unbounded heap. A caller that needs more than this keeps a round
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

// Keep writes the state into the caller's batch without verifying it against a
// root.
//
// It is what the join does with every account it pulls (#4438): the join's
// one proof is the whole local root equal to a signed anchor's StateTreeAnchor
// (executor spec, "Sync", "The algorithm", step 3), and nothing before that
// match is proven or needs to be. What Fetch refuses as malformed -- a body
// under another name, an answer with no body, an entry with no message behind
// it -- never reaches it.
func (p *Pending) Keep() error {
	if p.done {
		return errors.NotAllowed.WithFormat("%v: already kept or discarded", p.Account)
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

// Fetch pulls u from src per opts.Mode and holds it, unwritten, until the
// caller keeps or discards it. withReceipt asks the peer for the proof that
// binds the state to its root, for what the receipt-bearing answer carries
// beside the body (#4399); Keep does not check it against a root.
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
			"%v: %d accounts are already fetched and unkept, the limit is %d", u, n-1, MaxHeld)
	}

	sub := batch.Begin(true)
	p := &Pending{Account: u, Partition: opts.Partition, batch: sub}

	fail := func(err error) (*Pending, error) {
		p.release()
		sub.Discard()
		return nil, err
	}

	// 1. Main account state, with the receipt that binds it to the peer's root.
	receipt, err := pullMain(ctx, src, sub, u, withReceipt)
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
	if receipt != nil {
		p.Block = receipt.LocalBlock

		// The block is the SERVING partition's, not the puller's. A receipt
		// proves the state as of a block of the partition that built it
		// (api.Receipt.Partition, internal/api/v3/querier.go), and block
		// numbers collide across partitions -- so a foreign account's block
		// attributed to this node's partition names the wrong block entirely
		// (#4308). Latent while
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

	// 4. Chains. What is taken is the peer's; the node's own heights are not
	// consulted.
	switch opts.Mode {
	case ModeStateOnly:
		err = pullChainHeads(ctx, src, sub, u, pageSize)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chain heads %s: %w", u, err))
		}
	case ModeFullSpine:
		// The entries go to the store, a page at a time; without one, into the
		// caller's batch -- not the account's own: a message is not the
		// account's, and the same key written by two pending batches of one
		// pass conflicts when the second is kept.
		var pages Store = batch
		if opts.Store != nil {
			pages = opts.Store
		}
		err = pullChainsFull(ctx, src, sub, pages, u, pageSize, opts.CheckHeld, opts.RetakeLonger)
		if err != nil {
			return fail(errors.UnknownError.WithFormat("chains full %s: %w", u, err))
		}
	default:
		return fail(errors.BadRequest.WithFormat("unknown pull mode %d", opts.Mode))
	}

	return p, nil
}

// Account pulls u from src into batch per opts.Mode and keeps it. It is Fetch
// and Keep in one call, for a caller that does not need to hold the fetch
// before deciding what to do with it; a caller that does uses the two
// (FetchFrom).
func Account(ctx context.Context, src Source, batch *database.Batch, u *url.URL, opts Options) error {
	p, err := Fetch(ctx, src, batch, u, opts, opts.WithReceipt)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return errors.UnknownError.Wrap(p.Keep())
}

// AccountFrom pulls u from the first source that can serve it, and reports
// which one answered. A source that cannot serve the account is refused and
// the next is asked; when none answer, every refusal is reported.
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
		if ctx.Err() != nil {
			return -1, errors.UnknownError.Wrap(err)
		}
		refusals = append(refusals, errors.UnknownError.WithFormat("source %d: %w", i, err))
	}
	return -1, errors.Conflict.WithFormat("%v: no source served it: %w", u, stderrors.Join(refusals...))
}

// FetchFrom fetches u from the first source that serves it and hands the state
// back held, unwritten, with the index of the source that answered. A source
// that cannot serve the account is refused and the next is asked; when none
// answer, every refusal is reported.
//
// It is what the join uses: hold the fetch, and Keep it once the state is
// assembled, or Discard it.
//
// The returned Pending holds an open child of batch. It must be kept or
// discarded before batch is committed or discarded, and it counts against
// MaxHeld until it is.
func FetchFrom(ctx context.Context, srcs []Source, batch *database.Batch, u *url.URL, opts Options) (*Pending, int, error) {
	if len(srcs) == 0 {
		return nil, -1, errors.BadRequest.With("pull.FetchFrom: at least one source required")
	}
	var refusals []error
	noLeaf := 0
	for i, src := range srcs {
		p, err := Fetch(ctx, src, batch, u, opts, opts.WithReceipt)
		if ctx.Err() != nil {
			if p != nil {
				p.Discard()
			}
			return nil, -1, errors.UnknownError.Wrap(ctx.Err())
		}
		if err == nil {
			return p, i, nil
		}
		if stderrors.Is(err, ErrNoLeaf) {
			noLeaf++
		}
		refusals = append(refusals, errors.UnknownError.WithFormat("%s: %w", sourceName(i, src), err))
	}

	if noLeaf == len(srcs) {
		return nil, -1, fmt.Errorf("%v: %w, by every source asked: %v", u, ErrNoLeaf, stderrors.Join(refusals...))
	}
	return nil, -1, errors.Conflict.WithFormat("%v: no source served it: %w", u, stderrors.Join(refusals...))
}

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
func pullMain(ctx context.Context, src Source, batch *database.Batch, u *url.URL, wantReceipt bool) (*api.Receipt, error) {
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
		return nil, fmt.Errorf("%w: %v", ErrNoLeaf, err)
	default:
		return nil, errors.UnknownError.WithFormat("query account: %w", err)
	}

	// A peer that answers with an empty record has served nothing, and serving
	// nothing is a failure of that source, not an account with no state. It
	// used to return success with no receipt, and a fetch with no receipt was
	// kept for any block at all, anchored or not. It is not NotFound: that is
	// the peer's own answer that it holds no leaf, and a name every source
	// answers so is dropped (executor.md, "Sync", §2).
	if rec == nil {
		return nil, errors.Conflict.WithFormat("%v: the peer served no account", u)
	}
	if wantReceipt && rec.Receipt == nil {
		return nil, errors.Conflict.WithFormat("%v: the peer served no receipt", u)
	}

	// A leaf with no body does not exist: the state tree holds a leaf only
	// for an account with main state (executor spec, invariant 13; #4437).
	// A peer that serves a receipt with no body is serving a leaf no honest
	// peer has, and it is refused as a failure of that source. Before #4437
	// such leaves did exist, one hash for every one of them, and taking them
	// on peers' word was the phantom-leaf hole (#4406).
	if rec.Account == nil {
		return nil, errors.Conflict.WithFormat("%v: the peer served no account", u)
	}
	// A body names its own account, and that is what binds a body's leaf to
	// the name asked for. A body served under another name hashes to that
	// other account's true leaf and passes the leaf check; the store then
	// refuses it only at commit, where a mismatched URL is a panic, and one
	// such answer took the joining node down (#4408, review R4). It is a
	// refusal of this source here, and the next is asked.
	if got := rec.Account.GetUrl(); got == nil || !got.Equal(u) {
		return nil, errors.Conflict.WithFormat("%v: the peer served the body of %v", u, got)
	}
	if err := batch.Account(u).Main().Put(rec.Account); err != nil {
		return nil, errors.UnknownError.WithFormat("store main: %w", err)
	}
	if wantReceipt {
		// The answer that carries a receipt carries the rest of the leaf the
		// receipt proves; one without a receipt does not, and must not clear
		// what the node holds.
		if err := pullLeafBesideBody(ctx, src, batch, u, rec.Leaf); err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	return rec.Receipt, nil
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

	// Each set is written once per block, with each entry once. The events
	// BPT is keyed by entry, so an entry served twice leaves the root -- and
	// the leaf check -- unchanged, while the set itself would hold it twice
	// and the executor would process it twice (review R2). A block served
	// twice is one block. Writing a non-empty set adds its block to the list
	// (blockEventSet).
	votes := map[uint64][]*protocol.AuthoritySignature{}
	var voteBlocks []uint64
	for _, v := range ev.MinorVotes {
		if v == nil {
			continue
		}
		if _, ok := votes[v.Block]; !ok {
			voteBlocks = append(voteBlocks, v.Block)
		}
		votes[v.Block] = append(votes[v.Block], v.Votes...)
	}
	for _, b := range voteBlocks {
		set := uniqueBy(votes[b], func(v *protocol.AuthoritySignature) [32]byte {
			return record.NewKey(v.Authority, v.TxID.Hash()).Hash()
		})
		if len(set) == 0 {
			continue
		}
		if err := events.Minor().Votes(b).Put(set); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	pending := map[uint64][]*url.TxID{}
	var pendingBlocks []uint64
	for _, p := range ev.MajorPending {
		if p == nil {
			continue
		}
		if _, ok := pending[p.Block]; !ok {
			pendingBlocks = append(pendingBlocks, p.Block)
		}
		pending[p.Block] = append(pending[p.Block], p.Pending...)
	}
	for _, b := range pendingBlocks {
		set := uniqueBy(pending[b], (*url.TxID).Hash)
		if len(set) == 0 {
			continue
		}
		if err := events.Major().Pending(b).Put(set); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return errors.UnknownError.Wrap(events.Backlog().Expired().Put(uniqueBy(ev.Expired, (*url.TxID).Hash)))
}

// uniqueBy keeps the first of each entry with the same key, and drops nil
// entries, in the order served.
func uniqueBy[T comparable](in []T, key func(T) [32]byte) []T {
	seen := map[[32]byte]bool{}
	var out []T
	var zero T
	for _, v := range in {
		if v == zero {
			continue
		}
		k := key(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
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
// heads.
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

// chainEntries reads the entries [start, end) of one of the peer's chains,
// held in memory: for a range no longer than a mark set.
func chainEntries(ctx context.Context, src Source, u *url.URL, chainName string, start, end, pageSize uint64) ([][]byte, error) {
	var out [][]byte
	err := streamEntries(ctx, src, nil, u, chainName, start, end, pageSize, false, func(_ *database.Batch, _ *messages, index uint64, entry []byte) error {
		out = append(out, entry)
		return nil
	})
	return out, err
}

// messages proves and holds the messages a peer serves behind the entries of
// an account's transaction chains, for one page of one chain: it is written
// with the page and dropped with it (#4446), so what it holds is bounded by
// the page size and not by the chain's length.
//
// An entry of a transaction chain is the hash of a message, and a node that
// holds the hash without the message holds half of what the executor reads:
// the first block a new process opens walks the anchor pool's chains and loads
// the message behind each entry (executor.md, "Sync" §3). The entry is under
// the root the pass is proven against, so a message is proven by its own hash
// and nothing about the peer is trusted. What the peer stores under an entry
// is not always the message as it arrived: a wrapper -- an anchor, a
// sequenced or synthetic message -- whose transaction is stored under its own
// hash is stored referring to it (the executor's storedForm, #4236). Such a
// message is proven by putting the transaction back, itself proven by its
// hash, and hashing the result; it is kept in the stored form, with the
// transaction under its own hash beside it, as the peer keeps it.
//
// A peer that serves an entry with no message behind it -- a peer that itself
// joined holds the blocks it did not execute that way -- or a message that
// does not hash to the entry, has not served the chain; the fetch fails and
// the caller asks the next peer. A hash is never kept without its message.
type messages struct {
	ctx   context.Context
	src   Source
	local *database.Batch

	// txns are the transactions proven so far in this page, by hash: the
	// ones a stored form refers to, and the entries that are transactions.
	// One proven by an earlier page is read back from local, where that page
	// wrote it.
	txns map[[32]byte]*protocol.Transaction

	// kept is what the fetch keeps, by the key it is stored under: each
	// entry's message, and the transactions stored forms referred to.
	kept map[[32]byte]messaging.Message

	// signatures are the anchor signatures on the signature chains taken,
	// indexed under their transactions when the page is written (#4416).
	signatures []signature

	// executed are the transactions this page took as main chain entries:
	// the anchors that executed. One an earlier page took is found on the
	// main chain the store holds (isExecuted).
	executed map[[32]byte]bool

	// account is the account whose chains the messages are behind.
	account *url.URL
}

// newMessages proves messages served by src; local is the node's own store,
// consulted first for a transaction a stored form refers to.
func newMessages(ctx context.Context, src Source, local *database.Batch, account *url.URL) *messages {
	return &messages{ctx: ctx, src: src, local: local, account: account, txns: map[[32]byte]*protocol.Transaction{}, kept: map[[32]byte]messaging.Message{}, executed: map[[32]byte]bool{}}
}

// behind is the message the peer served behind e, if it is e's.
func (m *messages) behind(e *api.ChainEntryRecord[api.Record]) (messaging.Message, error) {
	var msg messaging.Message
	switch v := e.Value.(type) {
	case *api.MessageRecord[messaging.Message]:
		msg = v.Message
	case *api.ErrorRecord:
		return nil, fmt.Errorf("the peer does not hold the message: %v", v.Value)
	case nil:
	default:
		return nil, fmt.Errorf("the peer served a %v record, not a message", v.RecordType())
	}
	if msg == nil {
		return nil, fmt.Errorf("the peer served no message")
	}

	// A transaction entry is its transaction, whole: a remote transaction's
	// hash is whatever the stub says it is.
	if tm, ok := msg.(*messaging.TransactionMessage); ok {
		if err := wholeTransaction(tm.Transaction, e.Entry); err != nil {
			return nil, err
		}
		m.txns[e.Entry] = tm.Transaction
		return msg, nil
	}

	full, err := m.expand(msg)
	if err != nil {
		return nil, err
	}
	if h := full.Hash(); h != e.Entry {
		return nil, errors.Conflict.WithFormat("the peer served a %v that hashes to %x", msg.Type(), h[:4])
	}
	if ba, ok := full.(*messaging.BlockAnchor); ok {
		if err := checkAnchorSignature(ba); err != nil {
			return nil, err
		}
	}
	return storedForm(msg, full), nil
}

// storedForm is the form of full the node keeps, built here and not taken
// from the peer: where the peer's stored form refers to a transaction by
// hash, full's transaction is referred to by hash under the principal it
// names, as the executor's storedForm refers to it. A reference is replaced
// whole when the message is proven (expand), so its header is covered by no
// hash, and keeping the served one would keep the peer's word for it
// (#4416 review F3).
func storedForm(served, full messaging.Message) messaging.Message {
	switch f := full.(type) {
	case *messaging.TransactionMessage:
		s, ok := served.(*messaging.TransactionMessage)
		if !ok || s.Transaction == nil || s.Transaction.Body == nil || s.Transaction.Body.Type() != protocol.TransactionTypeRemote {
			return full
		}
		ref := new(protocol.Transaction)
		ref.Header.Principal = f.Transaction.Header.Principal
		ref.Body = &protocol.RemoteTransaction{Hash: *(*[32]byte)(f.Transaction.GetHash())}
		return &messaging.TransactionMessage{Transaction: ref}

	case *messaging.SequencedMessage:
		s, ok := served.(*messaging.SequencedMessage)
		if !ok {
			return full
		}
		c := *f
		c.Message = storedForm(s.Message, f.Message)
		return &c

	case *messaging.SyntheticMessage:
		s, ok := served.(*messaging.SyntheticMessage)
		if !ok {
			return full
		}
		c := *f
		c.Message = storedForm(s.Message, f.Message)
		return &c

	case *messaging.BadSyntheticMessage:
		s, ok := served.(*messaging.BadSyntheticMessage)
		if !ok {
			return full
		}
		c := *f
		c.Message = storedForm(s.Message, f.Message)
		return &c

	case *messaging.BlockAnchor:
		s, ok := served.(*messaging.BlockAnchor)
		if !ok {
			return full
		}
		c := *f
		c.Anchor = storedForm(s.Anchor, f.Anchor)
		return &c

	default:
		return full
	}
}

// expand is msg with every transaction it refers to by hash put back.
func (m *messages) expand(msg messaging.Message) (messaging.Message, error) {
	switch w := msg.(type) {
	case *messaging.TransactionMessage:
		if w.Transaction == nil || w.Transaction.Body == nil {
			return nil, fmt.Errorf("a transaction with no body")
		}
		remote, ok := w.Transaction.Body.(*protocol.RemoteTransaction)
		if !ok {
			return msg, nil
		}
		txn, err := m.transaction(remote.Hash)
		if err != nil {
			return nil, err
		}
		return &messaging.TransactionMessage{Transaction: txn}, nil

	case *messaging.SequencedMessage:
		inner, err := m.expand(w.Message)
		if err != nil {
			return nil, err
		}
		c := *w
		c.Message = inner
		return &c, nil

	case *messaging.SyntheticMessage:
		inner, err := m.expand(w.Message)
		if err != nil {
			return nil, err
		}
		c := *w
		c.Message = inner
		return &c, nil

	case *messaging.BadSyntheticMessage:
		inner, err := m.expand(w.Message)
		if err != nil {
			return nil, err
		}
		c := *w
		c.Message = inner
		return &c, nil

	case *messaging.BlockAnchor:
		inner, err := m.expand(w.Anchor)
		if err != nil {
			return nil, err
		}
		c := *w
		c.Anchor = inner
		return &c, nil

	case nil:
		return nil, fmt.Errorf("an empty message")

	default:
		return msg, nil
	}
}

// transaction is the transaction whose hash is h, proven by it: from this
// fetch if it has been seen, else from the node's own store -- an anchor the
// node executed before its gap has its transaction there -- else asked of the
// same peer. The node's own copy is held to the same check as a peer's: a
// stored form there is not a body.
func (m *messages) transaction(h [32]byte) (*protocol.Transaction, error) {
	if txn, ok := m.txns[h]; ok {
		return txn, nil
	}
	if m.local != nil {
		var own *messaging.TransactionMessage
		if m.local.Message(h).Main().GetAs(&own) == nil && wholeTransaction(own.Transaction, h) == nil {
			m.txns[h] = own.Transaction
			return own.Transaction, nil
		}
	}
	rec, err := m.src.QueryMessage(m.ctx, protocol.UnknownUrl().WithTxID(h), nil)
	if err != nil {
		return nil, fmt.Errorf("the peer does not serve the transaction %x a message refers to: %w", h[:4], err)
	}
	if rec == nil {
		return nil, fmt.Errorf("the peer served no transaction %x", h[:4])
	}
	tm, ok := rec.Message.(*messaging.TransactionMessage)
	if !ok {
		return nil, fmt.Errorf("the peer served a %T for the transaction %x", rec.Message, h[:4])
	}
	if err := wholeTransaction(tm.Transaction, h); err != nil {
		return nil, err
	}
	m.txns[h] = tm.Transaction
	m.kept[h] = tm
	return tm.Transaction, nil
}

// wholeTransaction says whether txn is the transaction h names, with its body.
func wholeTransaction(txn *protocol.Transaction, h [32]byte) error {
	if txn == nil || txn.Body == nil {
		return fmt.Errorf("the peer served a transaction with no body")
	}
	if _, remote := txn.Body.(*protocol.RemoteTransaction); remote {
		return fmt.Errorf("the peer served a transaction without its body")
	}
	if got := *(*[32]byte)(txn.GetHash()); got != h {
		return errors.Conflict.WithFormat("the peer served transaction %x for %x", got[:4], h[:4])
	}
	return nil
}

// store writes what the page kept into batch, the page's own.
func (m *messages) store(batch *database.Batch) error {
	if m == nil {
		return nil
	}
	for h, msg := range m.kept {
		if err := batch.Message(h).Main().Put(msg); err != nil {
			return fmt.Errorf("store message %x: %w", h[:4], err)
		}
	}
	return storeSignatures(batch, m.signatures, func(txn [32]byte) bool { return m.isExecuted(batch, txn) })
}

// isExecuted is whether txn is an entry of the account's main chain: taken
// in this page, or written by an earlier one. The main chain is taken before
// the signature chain (pullChainsFull), so an anchor that executed is found
// whichever page took it.
func (m *messages) isExecuted(batch *database.Batch, txn [32]byte) bool {
	if m.executed[txn] {
		return true
	}
	if m.account == nil {
		return false
	}
	main := batch.Account(m.account).MainChain().Inner()
	i, err := main.ElementIndex(txn[:]).Get()
	if err != nil {
		return false
	}
	h, err := main.Element(i).Get()
	return err == nil && bytes.Equal(h, txn[:])
}

// carriesMessages is whether a chain's entries are the hashes of messages the
// node stores. The data model labels the synthetic sequence chains as
// transaction chains when their entries are index entries (the querier makes
// the same exception, internal/api/v3/querier.go queryChainEntry).
func carriesMessages(c *api.ChainRecord) bool {
	return c.Type == merkle.ChainTypeTransaction && !strings.HasPrefix(c.Name, "synthetic-sequence(")
}

// addChainToIndex records the chain in the account's chain index directly: a
// batch only builds the index from the chains it holds when it commits, and
// this writes it into the pull's own batch ahead of that.
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

// pullChainsFull takes every entry of every chain the node does not already
// hold. It starts from the local height, not from zero, so a second pull of an
// account is a no-op rather than a chain of twice the height — a restarting
// node re-pulls the spine, and a pull that is not idempotent doubles it.
//
// A chain's head is the peer's (c.State, the count and pending hashes the
// account's leaf is hashed from) and it does not move while the entries under
// it come in: they are streamed to pages, a page at a time, each written to
// the store and dropped before the next is asked for (#4446), and they are
// held to the head once the last is written -- the running state they build,
// from the node's own below the local height, must reach the head's count and
// anchor. Only then is the head restored, into batch, the account's own.
// A chain the node holds more of than the peer served is refused; the pull
// asks another peer, or this one again once it has moved on -- unless the
// caller repairs what it executed (Options.RetakeLonger), when it is taken
// again whole. A chain that is not the peer's once the peer's entries are
// appended to it — a node that
// executed from a wrong state appended entries of its own — is taken again,
// whole, from its first entry (#4421): it cannot be brought to the peer's by
// appending, and refusing it left such a node unable ever to sync again. The
// node's history is not compared with the peer's to find where they part: at
// an anchored height there is one correct chain, and it is the peer's, taken
// on Fetch's word and proven only when the join's whole local root matches a
// signed anchor (executor spec, "Sync", "The algorithm", step 3).
func pullChainsFull(ctx context.Context, src Source, batch *database.Batch, pages Store, u *url.URL, pageSize uint64, checkHeld, retakeLonger bool) error {
	// Empty ChainQuery: list-all-chains. See pullChainHeads.
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}
	if chains == nil {
		return nil
	}
	// The main chain first: an anchor signature's page writes what executing
	// the anchor wrote only for an anchor on the main chain, and it looks
	// there for it (messages.isExecuted).
	ordered := make([]*api.ChainRecord, 0, len(chains.Records))
	for _, c := range chains.Records {
		if c != nil && c.Name == "main" {
			ordered = append(ordered, c)
		}
	}
	for _, c := range chains.Records {
		if c != nil && c.Name != "main" {
			ordered = append(ordered, c)
		}
	}
	for _, c := range ordered {
		if c.Name == "" {
			continue
		}
		err := pullChain(ctx, src, batch, pages, u, c, pageSize, checkHeld, false)
		if stderrors.Is(err, errNotThePeers) || retakeLonger && stderrors.Is(err, errLongerThanThePeers) {
			err = pullChain(ctx, src, batch, pages, u, c, pageSize, false, true)
		}
		if err != nil {
			return fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
	}
	// Indexed once every chain is taken: a page lists its chain in the index
	// where it is written (Account.Commit), and batch touching the index
	// before the last page is written would hold a version of it the pages
	// have moved past.
	for _, c := range ordered {
		if c.Name == "" {
			continue
		}
		if err := addChainToIndex(batch, u, c); err != nil {
			return err
		}
	}
	if retakeLonger {
		// The account is taken whole: a chain the node created that the
		// peer's account does not have leaves the index, or the account's
		// hash still counts it (executor spec, "Sync", "Two
		// mismatches": "the repair takes accounts whole").
		var list []*protocol.ChainMetadata
		for _, c := range chains.Records {
			if c == nil || c.Name == "" {
				continue
			}
			list = append(list, &protocol.ChainMetadata{Name: c.Name, Type: c.Type})
		}
		if err := batch.Account(u).Chains().Put(list); err != nil {
			return fmt.Errorf("replace the chain index of %s: %w", u, err)
		}
	}
	return nil
}

// errNotThePeers is a chain that is not the peer's after the peer's entries
// are appended to what the node holds.
var errNotThePeers = stderrors.New("the local chain is not a prefix of the peer's")

// errLongerThanThePeers is a chain the node holds more entries of than the
// peer served.
var errLongerThanThePeers = stderrors.New("the local chain is longer than the peer's")

// pullChain brings one chain up to the head the peer served, from whatever the
// node already holds -- or, whole, from its first entry, over whatever the
// node held. A transaction chain's entries come with the messages they name,
// each checked against its entry (#4400) and written with its page. checkHeld
// also fetches what the held entries lack (Options.CheckHeld).
//
// The entries and messages are written to pages as they arrive and are not
// taken back when the chain is refused: a message is proven by its hash, and
// an element above the node's head is overwritten by the next pull. What a
// refused chain leaves behind is an index entry naming a position the next
// pull writes something else at (docs/spec/DIFFERENCES.md, #4446).
func pullChain(ctx context.Context, src Source, batch *database.Batch, pages Store, u *url.URL, c *api.ChainRecord, pageSize uint64, checkHeld, whole bool) error {
	dstChain, err := batch.Account(u).ChainByName(c.Name)
	if err != nil {
		return err
	}
	dst := dstChain.Inner()

	// What the node holds is read beside batch, not through it: batch
	// touches none of the chain's records until the pages under it are
	// written, or it would hold versions of them the pages have moved past.
	local := pages.Begin(false)
	defer local.Discard()
	localChain, err := local.Account(u).ChainByName(c.Name)
	if err != nil {
		return err
	}
	held := localChain.Inner()

	// The running state the entries build, from the node's own head below
	// the local height.
	st := new(merkle.State)
	if !whole {
		head, err := held.Head().Get()
		if err != nil {
			return fmt.Errorf("load the local head: %w", err)
		}
		if head.Count > int64(c.Count) {
			return fmt.Errorf("the local chain is at %d and the peer served %d; it cannot be re-pulled: %w", head.Count, c.Count, errLongerThanThePeers)
		}
		st = head.Copy()
		st.HashList = nil
		if st.Count&held.MarkMask() != 0 {
			open, err := held.OpenSet(head)
			if err != nil {
				return fmt.Errorf("load the local open mark set: %w", err)
			}
			st.HashList = open
		}
	}
	from := st.Count

	bodies := carriesMessages(c)
	if checkHeld && bodies {
		if err := fetchHeldMessages(ctx, src, pages, local, held, u, c, from, pageSize); err != nil {
			return err
		}
	}
	err = streamEntries(ctx, src, pages, u, c.Name, uint64(from), c.Count, pageSize, bodies, func(page *database.Batch, _ *messages, index uint64, entry []byte) error {
		pc, err := page.Account(u).ChainByName(c.Name)
		if err != nil {
			return err
		}
		return pc.Inner().PutBelow(st, entry, false)
	})
	if err != nil {
		return err
	}

	// The peer's head is what the account's leaf is hashed from, so entries
	// that do not reproduce it are a different chain.
	want := &merkle.State{Count: int64(c.Count), Pending: c.State}
	if st.Count != want.Count || !bytes.Equal(st.Anchor(), want.Anchor()) {
		return fmt.Errorf("%w: the peer's entries [%d, %d) on the %d the node holds anchor to %x and the peer's head to %x",
			errNotThePeers, from, c.Count, from, st.Anchor(), want.Anchor())
	}
	if !whole && from == want.Count {
		return nil // Nothing taken; the head is the node's own
	}
	return dst.RestoreHead(want, st.HashList)
}

// streamEntries takes the entries [start, end) of one of the peer's chains a
// page at a time and, when bodies is set, the message behind each, proven by
// its hash (see messages). Each page is written into a batch of its own begun
// from pages and committed before the next page is asked for, with the
// messages it proved; put writes an entry into it. Nothing of a page is held
// once it is committed (#4446). With pages nil nothing is written and put
// only sees the entries.
func streamEntries(ctx context.Context, src Source, pages Store, u *url.URL, chainName string, start, end, pageSize uint64, bodies bool, put func(page *database.Batch, m *messages, index uint64, entry []byte) error) error {
	for start < end {
		count := pageSize
		if count > end-start {
			count = end - start
		}
		var page *database.Batch
		if pages != nil {
			page = pages.Begin(true)
		}
		next, err := streamPage(ctx, src, page, u, chainName, start, count, bodies, put)
		if err == nil && page != nil {
			err = page.Commit()
		}
		if err != nil {
			if page != nil {
				page.Discard()
			}
			return err
		}
		start = next
	}
	return nil
}

// streamPage takes up to count entries from start into page, and reports
// where the next page starts.
func streamPage(ctx context.Context, src Source, page *database.Batch, u *url.URL, chainName string, start, count uint64, bodies bool, put func(page *database.Batch, m *messages, index uint64, entry []byte) error) (uint64, error) {
	resp, err := src.QueryChainEntries(ctx, u, &api.ChainQuery{
		Name: chainName,
		Range: &api.RangeOptions{
			Start:  start,
			Count:  &count,
			Expand: &bodies,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("query entries from %d: %w", start, err)
	}
	if resp == nil || len(resp.Records) == 0 {
		return 0, fmt.Errorf("the peer served no entry at %d", start)
	}
	var m *messages
	if bodies {
		m = newMessages(ctx, src, page, u)
	}
	end := start + count
	for _, e := range resp.Records {
		if e == nil {
			return 0, fmt.Errorf("the peer served a nil entry at %d", start)
		}
		if e.Index != start {
			return 0, fmt.Errorf("the peer served entry %d where %d was asked for", e.Index, start)
		}
		entry := e.Entry[:]
		if err := put(page, m, start, entry); err != nil {
			return 0, err
		}
		if m != nil {
			msg, err := m.behind(e)
			if err != nil {
				return 0, errors.NotFound.WithFormat("entry %d (%x): %w", start, entry[:4], err)
			}
			if err := m.took(u, chainName, start, entry, msg); err != nil {
				return 0, err
			}
		}
		start++
		if start >= end {
			break
		}
	}
	if page != nil {
		if err := m.store(page); err != nil {
			return 0, err
		}
	}
	return start, nil
}

// took keeps what the page proved behind entry index of u's chain, and what
// executing it wrote beside the entry (#4416).
func (m *messages) took(u *url.URL, chain string, index uint64, entry []byte, msg messaging.Message) error {
	// Under the entry, not under the message's own hash: a stored form refers
	// to its transaction and hashes to something else.
	m.kept[*(*[32]byte)(entry)] = msg
	switch chain {
	case "main":
		m.executed[*(*[32]byte)(entry)] = true
	case "signature":
		// Proven by behind; expanding again reads what it cached.
		full, err := m.expand(msg)
		if err != nil {
			return fmt.Errorf("entry %d: %w", index, err)
		}
		if sig, ok := signatureOf(u, index, msg, full); ok {
			m.signatures = append(m.signatures, sig)
		}
	default:
		if err := m.companion(chain, index, msg); err != nil {
			return err
		}
	}
	return nil
}

// companion takes the transaction a synthetic names beside it: a signature
// request, a signature or a credit payment a partition sends another is an
// entry of the synthetic ledger's chain to it, and the transaction it is for
// is not on the chain. The seed loads it beside the entry (synth_cache_seed.go
// rebuildCacheBlock) and dispatch sends it with the entry, so a synthetic
// ledger taken with its entries' messages and not their companions left the
// seed failing on a block the node did not execute (#4434). It is proven by
// its hash, like every transaction a stored form refers to.
func (m *messages) companion(chain string, index uint64, msg messaging.Message) error {
	if !strings.HasPrefix(chain, "synthetic(") {
		return nil
	}
	seq, ok := msg.(*messaging.SequencedMessage)
	if !ok {
		return nil
	}
	inner, ok := seq.Message.(messaging.MessageForTransaction)
	if !ok || seq.Message.Type() == messaging.MessageTypeBlockAnchor {
		return nil
	}
	if _, err := m.transaction(inner.GetTxID().Hash()); err != nil {
		return fmt.Errorf("entry %d: the transaction a synthetic names: %w", index, err)
	}
	return nil
}

// fetchHeldMessages fetches the message behind each of the newest
// HeldCheckDepth entries below height the node holds on a transaction chain
// that has none (Options.CheckHeld), and what executing it wrote beside the
// entry. Each is asked of the peer by position and proven like any other: its
// message must hash to its entry. A peer that does not serve it has not served
// the chain, and the fetch fails so the next peer is asked; a hash is never
// kept without its message (executor.md, "Sync" §3). A peer whose entry at
// that position is not the one held is not the chain the node holds, and the
// chain is taken whole.
func fetchHeldMessages(ctx context.Context, src Source, pages Store, local *database.Batch, dst *database.MerkleManager, u *url.URL, c *api.ChainRecord, height int64, pageSize uint64) error {
	from := height - HeldCheckDepth
	if from < 0 {
		from = 0
	}

	// The runs of held entries with no message behind them.
	type run struct {
		start  int64
		hashes [][]byte
	}
	var runs []*run
	var last *run
	for i := from; i < height; i++ {
		h, err := dst.Entry(i)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			// A position the node does not hold is not data it already has:
			// the lead's state-only re-pull restored a head and its open mark
			// set, and a chain that crossed a mark point between passes kept
			// the positions between not at all (#4421 review F1).
			return fmt.Errorf("%w: the node does not hold entry %d: %v", errNotThePeers, i, err)
		default:
			return fmt.Errorf("load held entry %d: %w", i, err)
		}
		_, err = local.Message2(h).Main().Get()
		switch {
		case err == nil:
			last = nil
			continue
		case errors.Is(err, errors.NotFound):
		default:
			return fmt.Errorf("load the message behind held entry %d: %w", i, err)
		}
		if last == nil {
			last = &run{start: i}
			runs = append(runs, last)
		}
		last.hashes = append(last.hashes, h)
	}

	for _, r := range runs {
		end := r.start + int64(len(r.hashes))
		err := streamEntries(ctx, src, pages, u, c.Name, uint64(r.start), uint64(end), pageSize, true, func(_ *database.Batch, _ *messages, index uint64, e []byte) error {
			held := r.hashes[index-uint64(r.start)]
			if !bytes.Equal(e, held) {
				return fmt.Errorf("%w: the peer's entry %d is %x and the node holds %x", errNotThePeers, index, e[:4], held[:4])
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("the messages behind held entries %d..%d: %w", r.start, end-1, err)
		}
	}
	return nil
}

// SpineAccounts is what a join takes first, in ModeFullSpine: the accounts
// every block touches, with their chains, so the node can compare its own
// history against a peer's and can append to them when it executes again.
//
// They are pulled and kept like every other account (#4301). The chains are
// not taken in order to verify anything — an anchor is checked against the
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
		// way that copy ever moves is this one: pulled and kept with the rest
		// of the spine, trusted once the whole local root matches a signed
		// anchor (executor spec, "Sync", "The algorithm", step 3), then
		// handed to anchorsrc.Authority. Past Vandenberg a change to them
		// never travels in an anchor (block_end.go:791-793), so this is the
		// whole of how a joining node crosses one (#4301).
		partitionURL.JoinPath(protocol.Network),
		partitionURL.JoinPath(protocol.Globals),
	}
}

// WholeAccounts is every account a join takes whole, in ModeFullSpine, in
// every pass that names it: the spine, and the partition's synthetic ledger.
//
// The synthetic ledger is not a trust root and is not in the spine: it is
// pulled and kept like any account, and failing to pull it fails nothing but
// its own name. It is taken whole because the first block a new process opens
// reads
// its chains back from the store -- the producer cache is seeded from the
// entries of the partition's own recent blocks, the messages behind them, and
// the chain's state before each block's first entry (synth_cache_seed.go
// rebuildCacheBlock). Taken state-only, it held the peer's head and open mark
// set and no mark point below the set, and the seed failed on the first block
// whose entries were in that set, at every handoff (#4434).
func WholeAccounts(partitionURL *url.URL) []*url.URL {
	return append(SpineAccounts(partitionURL), partitionURL.JoinPath(protocol.Synthetic))
}

// DnSpineAccounts is a backward-compatible helper returning
// SpineAccounts(protocol.DnUrl()).
func DnSpineAccounts() []*url.URL { return SpineAccounts(protocol.DnUrl()) }
