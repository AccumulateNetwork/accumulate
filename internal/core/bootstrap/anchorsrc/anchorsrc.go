// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package anchorsrc is where a joining node's roots come from, and the reason
// they are worth anything.
//
// It walks a peer's anchor pool, takes the anchors produced by one partition,
// and records a StateTreeAnchor only after a quorum of that partition's
// validators is shown to have signed the anchor transaction. Everything a
// join verifies is verified against one of those roots (executor.md, "Sync"),
// so this is where the chain of trust either terminates in a key the node
// already holds or terminates in the peer.
//
// It replaces pull.DirectoryAnchors, which did the same walk with the
// verification left out: with one source supplying both the root and the
// state that hashes into it, the whole scheme proved only that the peer
// agreed with itself (#4301, DIFFERENCES E11). The component this is ported
// from is internal/core/bootstrap/anchorsrc on bootstrap-v3-merge-1.4.4,
// which #4293 left behind; what changed on the way is recorded on #4301 and
// in the comments below.
//
// # Trust
//
// The peer is not trusted for anything. The validator sets come from the
// node's own store (see Authority) and never from the peer; a signature is
// checked against a key, so nothing has to exist on the network before this
// can work. An anchor is accepted only when valid signatures from DISTINCT
// members of the producing partition's set reach that partition's threshold;
// a second copy from one validator is no second signature.
//
// # Producer routing
//
// To verify partition P's root you need an anchor PRODUCED BY P, and a
// produced anchor lives on the RECEIVING partition's pool. So a BVN's root
// is read from dn.acme/anchors. The Directory anchors to ITSELF as well as
// to every BVN (crosschain/anchoring.go keeps the DN-to-itself branch), so
// its own root is in dn.acme/anchors and in every BVN's pool under the same
// signatures; PoolFor sends the Directory's join to a BVN's pool so that a
// second partition's copy attests it, but either copy verifies against the
// same keys. What the old code could not do was not obtain the Directory's
// root — it was check who signed it (#4301; measured, see the issue).
package anchorsrc

import (
	"context"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ErrNotAnchored says no verified anchor covers that block yet. It is a wait,
// not a failure: the anchor for a block arrives a few blocks later, and the
// caller asks again.
var ErrNotAnchored = errors.NotReady.With("no verified anchor covers that block yet")

const (
	// DefaultPageSize is how many pool entries are read per call.
	DefaultPageSize = 64

	// DefaultBackfill is how far back from the end of the pool's chain the
	// first read starts. The anchors a join needs are the current ones, and
	// entry 0 is the first anchor the network ever executed.
	DefaultBackfill = 1024

	// DefaultMaxRoots is how many (partition, block) roots are kept. The
	// oldest go first; a root that old belongs to a block no pull is still
	// waiting to settle.
	DefaultMaxRoots = 4096
)

// Source reads one producer partition's verified roots out of one anchor
// pool.
//
// It reads forward and remembers what it has read, so a join that walks the
// network block by block reads each anchor once. Both ends are bounded,
// because this runs for as long as a node takes to join.
type Source struct {
	// Query reaches the partition whose pool this is — a NAMED PEER, never
	// this node. A joining node's own querier answers from the store the
	// join exists to fill (#4303).
	Query api.Querier

	// Pool is the anchor pool to read: the RECEIVING partition's, the one
	// that holds anchors produced by Producer. See PoolFor.
	Pool *url.URL

	// Producer is the partition whose roots this source answers for. An
	// anchor produced by anybody else is skipped.
	Producer *url.URL

	// Authority is the validator sets anchors are checked against. It is the
	// node's own, and this source only READS it — nothing an anchor carries
	// moves it (see consider).
	Authority *Authority

	// OnAnchor, if set, is called for every VERIFIED anchor, in chain order.
	// The tracker's Observe is the intended consumer, and the point of the
	// package is that an unverified root never reaches it.
	OnAnchor func(partition *url.URL, block uint64, root [32]byte)

	// OnRefused, if set, is called when an anchor is read and not accepted.
	// A join that records nothing looks exactly like a network with no
	// anchors, and the difference matters to whoever is reading the log.
	OnRefused func(block uint64, err error)

	PageSize uint64
	Backfill uint64
	MaxRoots int

	mu      sync.Mutex
	roots   map[anchorKey][32]byte
	order   []anchorKey
	next    uint64
	started bool

	// doubt counts consecutive reads whose peer said the chain ends below
	// the cursor. One is a peer that lags; doubtRounds of them is a cursor
	// that is wrong.
	doubt int
}

type anchorKey struct {
	partition string
	block     uint64
}

// New constructs a Source. The pool is the receiving partition's, per
// PoolFor; the producer is the partition whose roots are wanted.
func New(q api.Querier, pool, producer *url.URL, authority *Authority) (*Source, error) {
	switch {
	case q == nil:
		return nil, errors.BadRequest.With("anchorsrc.New: a querier is required")
	case pool == nil:
		return nil, errors.BadRequest.With("anchorsrc.New: an anchor pool is required")
	case producer == nil:
		return nil, errors.BadRequest.With("anchorsrc.New: a producer partition is required")
	case authority == nil:
		return nil, errors.BadRequest.With("anchorsrc.New: an authority is required — an unverified root is not a root")
	}
	return &Source{Query: q, Pool: pool, Producer: producer, Authority: authority}, nil
}

// PoolFor is the anchor pool a partition's join reads its own roots from.
//
// Everyone's anchors are in the Directory's pool, and the Directory's are in
// every BVN's as well as its own. The Directory's join is sent to a BVN's
// pool so that the copy it reads is a second partition's; which BVN is
// chosen is not a trust decision — an anchor from the wrong pool simply
// fails to verify or is not this producer's — so the first is taken, and a
// Directory join therefore depends on one partition's peers being reachable
// (review finding 9).
func PoolFor(producer *url.URL, bvns []string) (*url.URL, error) {
	if producer == nil {
		return nil, errors.BadRequest.With("anchorsrc.PoolFor: a producer partition is required")
	}
	id, ok := protocol.ParsePartitionUrl(producer)
	if !ok {
		return nil, errors.BadRequest.WithFormat("%v is not a partition", producer)
	}
	if !strings.EqualFold(id, protocol.Directory) {
		return protocol.DnUrl().JoinPath(protocol.AnchorPool), nil
	}
	for _, bvn := range bvns {
		if strings.EqualFold(bvn, protocol.Directory) {
			continue
		}
		return protocol.PartitionUrl(bvn).JoinPath(protocol.AnchorPool), nil
	}
	// Said plainly, because this is the case that could not be answered at
	// all before: the Directory's own root does not exist in the Directory's
	// own pool, so a network with no BVN cannot produce one.
	return nil, errors.NotReady.With(
		"the directory's own root lives in a BVN's anchor pool and this node's network definition names no BVN")
}

// AnchoredRoot is the verified root for a partition's block, reading forward
// from where it last stopped. It returns ErrNotAnchored when no verified
// anchor covers that block yet.
//
// It implements pull.Verifier, so it is what the pull settles accounts
// against.
func (s *Source) AnchoredRoot(ctx context.Context, partition *url.URL, block uint64) ([32]byte, error) {
	key := anchorKey{strings.ToLower(partition.String()), block}

	s.mu.Lock()
	defer s.mu.Unlock()

	if root, ok := s.roots[key]; ok {
		return root, nil
	}
	if err := s.readLocked(ctx); err != nil {
		return [32]byte{}, errors.UnknownError.Wrap(err)
	}
	if root, ok := s.roots[key]; ok {
		return root, nil
	}
	return [32]byte{}, errors.NotReady.WithFormat("%v block %d: %w", partition, block, ErrNotAnchored)
}

// Rewind makes the source read its window again from the start.
//
// The join calls it when the trusted sets move. An anchor refused because
// this node had not reached the set that signed it is skipped and the cursor
// moves past it, so without a rewind the roots that were in flight across a
// change are lost for good and the node waits for the next one — which, on a
// BVN, is the ordinary case, because the roots and the definition come from
// different peers with different lag (review finding 4).
func (s *Source) Rewind() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = false
}

// Read takes whatever anchors the pool has gained since the last call,
// calling OnAnchor for each one that verifies. A caller that is only feeding
// a tracker uses this instead of asking for a specific block.
func (s *Source) Read(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked(ctx)
}

// LatestAnchor is the most recent verified anchor for the producer, or
// (0, zero, nil) when there is none yet.
func (s *Source) LatestAnchor(ctx context.Context) (uint64, [32]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.readLocked(ctx); err != nil {
		return 0, [32]byte{}, errors.UnknownError.Wrap(err)
	}
	var best uint64
	var root [32]byte
	want := strings.ToLower(s.Producer.String())
	for k, r := range s.roots {
		if k.partition == want && k.block >= best {
			best, root = k.block, r
		}
	}
	return best, root, nil
}

// FindAnchor looks for a verified anchor carrying expectedRoot and reports
// the block it was anchored for.
func (s *Source) FindAnchor(ctx context.Context, expectedRoot [32]byte) (uint64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.readLocked(ctx); err != nil {
		return 0, false, errors.UnknownError.Wrap(err)
	}
	want := strings.ToLower(s.Producer.String())
	for k, r := range s.roots {
		if k.partition == want && r == expectedRoot {
			return k.block, true, nil
		}
	}
	return 0, false, nil
}

// record keeps a root, evicting the oldest when the map is full.
func (s *Source) record(key anchorKey, root [32]byte) {
	max := s.MaxRoots
	if max <= 0 {
		max = DefaultMaxRoots
	}
	if s.roots == nil {
		s.roots = map[anchorKey][32]byte{}
	}
	if _, ok := s.roots[key]; !ok {
		s.order = append(s.order, key)
	}
	s.roots[key] = root
	for len(s.order) > max {
		delete(s.roots, s.order[0])
		s.order = s.order[1:]
	}
}

// maxPagesPerRead bounds one call's paging. A page that does not advance the
// cursor would otherwise loop forever under the lock, and a peer chooses how
// many records a page has (#4301, threat review F5). The cursor keeps its
// place between calls, so a node far behind catches up over several rounds
// rather than in one.
const maxPagesPerRead = 64

// doubtRounds is how many consecutive reads must report a chain shorter than
// the cursor before the cursor is believed to be the wrong one. The peers
// rotate per call, so this is that many different peers agreeing; one peer
// lagging by an entry is the ordinary case and must cost nothing.
const doubtRounds = 8

func (s *Source) readLocked(ctx context.Context) error {
	if s.roots == nil {
		s.roots = map[anchorKey][32]byte{}
	}
	pageSize := s.PageSize
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}
	backfill := s.Backfill
	if backfill == 0 {
		backfill = DefaultBackfill
	}

	q := api.Querier2{Querier: s.Query}

	// **The cursor is this node's, and a peer's count only ever sets it when
	// this node has nothing of its own.**
	//
	// Every number in this read comes from a peer, and the peers ROTATE per
	// call (join/sources.go, peerQuerier): the count and each page come from
	// different nodes. That makes the two failure directions ordinary rather
	// than adversarial, and they pull opposite ways.
	//
	// Too HIGH — a peer claiming a chain of 2^40 — would park the cursor
	// past the end of the real chain, every honest peer would then answer
	// nothing for that range, and the node would verify no anchor again for
	// the life of the process (threat review F5). Too LOW — the ordinary
	// case, a count-peer one entry behind the peer that served the last page
	// — used to rewind the cursor a full window and re-verify up to 1024
	// anchors, at threshold ed25519 verifications each, over and over
	// (review re-check, finding 1).
	//
	// So: a count BELOW the cursor is that peer's condition and not this
	// node's business, and nothing happens — unless doubtRounds peers say so
	// in a row, in which case the cursor is what is wrong and it re-anchors
	// to the window, exactly as a cold start would. And the cursor is not
	// this node's until a page has actually been consumed, so a first read
	// against a peer that names a window and serves nothing at it is
	// re-anchored next round against another peer.
	chain, err := q.QueryChain(ctx, s.Pool, &api.ChainQuery{Name: "main"})
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil // The pool holds no anchors yet
	default:
		return errors.UnknownError.WithFormat("read %v's anchor chain: %w", s.Pool, err)
	}
	// window is where a cold start reads from: near the end, because the
	// anchors a join needs are the current ones and entry 0 is the first
	// anchor the network ever executed. It is at most Backfill entries, and
	// MaxRoots is four times that, so re-reading it cannot evict what it
	// just recorded.
	window := uint64(0)
	if chain.Count > backfill {
		window = chain.Count - backfill
	}

	switch {
	case !s.started:
		// Nothing consumed yet, so there is no cursor to protect.
		s.doubt = 0
		s.next = window

	case s.next > chain.Count:
		s.doubt++
		if s.doubt < doubtRounds {
			// One peer is behind the one that served the last page. There is
			// nothing new HERE, and no reason to believe the cursor is wrong.
			return nil
		}
		// Every peer asked, for doubtRounds reads running, says the chain
		// ends below this cursor. The cursor is wrong, not the peers.
		s.doubt = 0
		s.next = window

	default:
		s.doubt = 0
	}

	for page := 0; page < maxPagesPerRead; page++ {
		// What was ASKED FOR, kept here, because what comes back is the
		// peer's and the cursor must not be.
		start := s.next
		count, expand := pageSize, true
		rec, err := q.QueryMainChainEntries(ctx, s.Pool, &api.ChainQuery{
			Name:  "main",
			Range: &api.RangeOptions{Start: start, Count: &count, Expand: &expand},
		})
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return nil // Nothing new
		default:
			return errors.UnknownError.WithFormat("read %v's anchors: %w", s.Pool, err)
		}
		if rec == nil || len(rec.Records) == 0 {
			return nil
		}

		// The cursor is this node's from here: a page came back, so the
		// window this peer named is real.
		s.started = true

		for _, entry := range rec.Records {
			s.consider(entry)
		}

		// Advanced by what was asked for and answered, never by an index the
		// peer chose.
		s.next = start + uint64(len(rec.Records))

		if uint64(len(rec.Records)) < count {
			return nil
		}
	}
	return nil
}

// consider takes one pool entry and, if this source's producer made it and a
// quorum of that producer's validators signed it, records its root.
//
// **The producer first, then the signatures.** A pool holds every partition's
// anchors — a BVN reading dn.acme/anchors sees the Directory's and every
// other BVN's — and this source answers for one of them. Verifying the rest
// costs a full signature check per anchor for a root nobody here wants, and
// it makes noise that reads as a finding: a partition that has not yet
// executed a network update signs under the older version, fails this node's
// floor, and is logged as an anchor refused (review re-check, finding 2).
// Somebody else's anchor is not refused. It is not this source's.
//
// Nothing an anchor carries moves the validator sets. A change used to travel
// as DirectoryAnchor.Updates and this package used to apply them; past
// Vandenberg the Directory never populates that field (block_end.go:792, and
// the change leaves as a messaging.NetworkUpdate, network_accounts.go:128),
// so the walk could not fire on this line at all — and while it was here, a
// quorum of ONE BVN could have used it to name the validators of every
// partition (#4301, review finding 1, threat finding F1). The sets move one
// way only: through Authority.Update, from a definition the join pulled and
// verified as a leaf under a root the trusted set signed.
func (s *Source) consider(rec *api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]) {
	if rec == nil || rec.Value == nil || rec.Value.Message == nil || rec.Value.Message.Transaction == nil {
		return
	}
	body, ok := rec.Value.Message.Transaction.Body.(protocol.AnchorBody)
	if !ok {
		return // The pool holds other transactions too
	}
	pa := body.GetPartitionAnchor()
	if pa == nil || pa.Source == nil {
		return
	}

	// The producer, not the pool. PartitionAnchor.Source is the partition
	// that produced the anchor (acc://dn.acme), never the pool it landed in.
	if !pa.Source.Equal(s.Producer) {
		return // Somebody else's anchor, in a pool that holds everyone's
	}
	produced, ok := protocol.ParsePartitionUrl(pa.Source)
	if !ok {
		return
	}

	err := s.verify(produced, rec.Value)
	if err != nil {
		if s.OnRefused != nil {
			s.OnRefused(pa.MinorBlockIndex, err)
		}
		return
	}

	// The MINOR block index, always. It is the block a peer serves an
	// account at (pull.Pending.Block), and it is what the tracker matches.
	// MajorBlockIndex is metadata and is zero on almost every anchor. The
	// root recorded is the one the join proves a pass's root against, by
	// equality (ProveRoot).
	s.record(anchorKey{strings.ToLower(pa.Source.String()), pa.MinorBlockIndex}, pa.StateTreeAnchor)
	if s.OnAnchor != nil {
		s.OnAnchor(pa.Source, pa.MinorBlockIndex, pa.StateTreeAnchor)
	}
}
