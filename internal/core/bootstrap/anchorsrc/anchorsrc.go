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
// produced anchor lives on the RECEIVING partition's pool, never on its own.
// So a BVN's root is read from dn.acme/anchors, and the Directory's own root
// is read from a BVN's anchor pool. A node that reads every partition's root
// from dn.acme/anchors can never obtain the Directory's — which is what
// pull.DirectoryAnchors was, and what join/state.go asked for every
// partition. PoolFor is the rule.
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
	// node's own, and this source advances it: an anchor that verifies under
	// the set the authority trusts may carry the change to the next set.
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

// PoolFor is the anchor pool that holds the anchors a partition PRODUCES.
//
// The Directory's own anchors are the ones it sent out, so they are in a
// BVN's pool; everyone else's are in the Directory's. bvns is the BVN list
// from the node's own network definition. Which BVN is chosen is not a trust
// decision — an anchor from the wrong pool simply fails to verify or is not
// this producer's — so the first is taken.
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

	// **The cursor is re-anchored on every read, against the chain as this
	// read's peer reports it.** It is a number derived from what peers said,
	// and the peers rotate: one page whose last entry claimed index 2^40
	// would otherwise park the cursor past the end of the real chain for the
	// life of the process, every honest peer would answer NotFound for that
	// range, and nothing would ever reset it — one response, for a node that
	// verifies no anchor again (#4301, threat review F5). A peer that reports
	// a count too LOW only makes this node re-read entries it has already
	// considered, which is idempotent.
	chain, err := q.QueryChain(ctx, s.Pool, &api.ChainQuery{Name: "main"})
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil // The pool holds no anchors yet
	default:
		return errors.UnknownError.WithFormat("read %v's anchor chain: %w", s.Pool, err)
	}
	if !s.started || s.next > chain.Count {
		s.started = true
		s.next = 0
		if chain.Count > backfill {
			// The anchors a join needs are the current ones, and entry 0 is
			// the first anchor the network ever executed.
			s.next = chain.Count - backfill
		}
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

// consider verifies one pool entry and, if it holds, records its root and
// walks the authority over whatever change it carried.
//
// **In that order.** The updates are applied only after the signatures have
// been checked against the set the authority trusts now, because that is the
// whole of the walk's safety: each change is anchored and signed by the
// preceding set.
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

	// The walk, and only from the Directory.
	//
	// A verified anchor is its partition's quorum over whatever it carries,
	// and a BVN's quorum is not the bar for naming the validators. Only the
	// Directory executes a change to the network accounts, and the executor
	// refuses a DirectoryAnchor from any other source outright
	// (execute/v2/chain/directory_anchor.go:40). Without this test a quorum
	// of ONE BVN — three keys of four in the soak topology, against the
	// Directory's eight of twelve — could put its own keys in every
	// partition's set as a joining node sees it, and every root it signed
	// afterwards would verify (#4301, threat review F1).
	if dir, ok := body.(*protocol.DirectoryAnchor); ok && len(dir.Updates) > 0 {
		switch {
		case !protocol.IsDnUrl(pa.Source):
			if s.OnRefused != nil {
				s.OnRefused(pa.MinorBlockIndex, errors.Unauthorized.WithFormat(
					"%v carried an operator change and is not the directory's", pa.Source))
			}
		default:
			if err := s.Authority.Apply(dir.Updates); err != nil && s.OnRefused != nil {
				s.OnRefused(pa.MinorBlockIndex, errors.UnknownError.WithFormat("walk the validator set forward: %w", err))
			}
		}
	}

	if !pa.Source.Equal(s.Producer) {
		// Verified, but it is somebody else's root. It still walked the
		// authority above, which is why a BVN reads the Directory's anchors
		// out of its own pool.
		return
	}

	// The MINOR block index, always. It is the block a peer serves an
	// account at (pull.Pending.Block), and it is what the tracker matches.
	// MajorBlockIndex is metadata and is zero on almost every anchor.
	s.record(anchorKey{strings.ToLower(pa.Source.String()), pa.MinorBlockIndex}, pa.StateTreeAnchor)
	if s.OnAnchor != nil {
		s.OnAnchor(pa.Source, pa.MinorBlockIndex, pa.StateTreeAnchor)
	}
}
