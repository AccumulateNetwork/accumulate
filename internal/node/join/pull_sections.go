// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	stderrors "errors"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	keys "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The pull's side of sections (executor spec, "Sync", §3a; #4485): the top of
// the tree taken at the start of a pull, every section the node does not hold
// whole made not loaded, and every section the pull writes into loaded
// before the state is complete.

// sectionID is one section of the state tree.
type sectionID = bpt.Section

// SystemAccounts is the partition's accounts the node reads outside a block
// -- at the handoff and after it -- whose sections every pull loads, so none
// of them is refused (executor spec, "Sync", §3a). A batch no section loads
// into reads them:
//
//   - the handoff's settle (Executor.SettleStagingAt): the system ledger, the
//     executor's globals reloaded from the state (reloadGlobals: the oracle,
//     the globals, the network definition, the routing table, and the
//     executor versions from the ledger), the anchor pool's Directory block,
//     and the synthetic and anchor ledgers' Delivered;
//   - staging, dispatch, the root watch and the seed: the ledger, the anchor
//     pool, the synthetic ledger;
//   - the anchor authority: the operators' key book and page, the network
//     definition and the globals.
//
// Votes and evidence are read only by a block's own batch, which loads.
func SystemAccounts(partition *url.URL) []*url.URL {
	return append(pull.SpineAccounts(partition),
		partition.JoinPath(protocol.Synthetic),
		partition.JoinPath(protocol.Oracle),
		partition.JoinPath(protocol.Routing),
	)
}

// openFor opens the sections the accounts' leaves fall in, where the node has
// not loaded them: the pull is about to write into them.
func (s *PulledState) openFor(p *syncing, urls ...*url.URL) {
	var secs []sectionID
	for _, u := range urls {
		secs = append(secs, database.SectionOfAccount(u))
	}
	s.openSections(p, secs...)
}

// openSection opens one section.
func (s *PulledState) openSection(p *syncing, sec sectionID) { s.openSections(p, sec) }

// openSections takes the sections' markers out of the tree, in one commit,
// and owes the sections' loads to the pull: a section is either not loaded --
// its marker's hash, untouched since the top was taken -- or loaded whole.
func (s *PulledState) openSections(p *syncing, secs ...sectionID) {
	var open []sectionID
	seen := map[sectionID]bool{}
	for _, sec := range secs {
		if !seen[sec] && s.secs.NotLoaded(sec) {
			seen[sec] = true
			open = append(open, sec)
		}
	}
	if len(open) == 0 {
		return
	}
	batch := s.db.Begin(true)
	defer batch.Discard()
	batch.TagWrites("open", 0)
	for _, sec := range open {
		if err := batch.BPT().Delete(sec.MarkerKey()); err != nil {
			s.log.Info("A section's marker could not be removed; the sections are not opened", "partition", s.partition, "section", sec, "error", err)
			return
		}
	}
	if err := batch.Commit(); err != nil {
		s.log.Info("The sections' markers could not be removed; the sections are not opened", "partition", s.partition, "sections", len(open), "error", err)
		return
	}
	for _, sec := range open {
		s.secs.opened(sec)
		p.opened[sec] = false
	}
}

// openDiff opens the section a located difference is in, when the node has
// not loaded it: the marker's hash is not the peer's. It reports whether it
// did.
func (s *PulledState) openDiff(p *syncing, kh [32]byte) bool {
	sec := bpt.SectionOf(kh)
	if !bpt.IsSectionMarker(kh) && !s.secs.NotLoaded(sec) {
		return false
	}
	if s.secs.NotLoaded(sec) {
		s.openSection(p, sec)
	} else {
		p.opened[sec] = false
	}
	return true
}

// covered is whether the pull has the whole tree: the top taken, and every
// section it opened loaded.
func (p *syncing) covered() bool { return p.topTaken && openedLoaded(p) }

// openedLoaded is whether every section the pull opened is loaded.
func openedLoaded(p *syncing) bool {
	for _, done := range p.opened {
		if !done {
			return false
		}
	}
	return true
}

// loadOpened loads every section the pull opened and has not loaded: the
// section's leaves from a peer as of L, and every account they name
// that this node does not hold or holds differently, unless a record brought
// it current -- the page may be older than the record. The node's own leaves
// in the section that the page does not name are asked of every peer, and
// forgotten only if none holds one (pullOne). A page of a block before the
// pull's start is asked of the next peer.
func (s *PulledState) loadOpened(ctx context.Context, p *syncing) {
	var todo []sectionID
	for sec, done := range p.opened {
		if !done {
			todo = append(todo, sec)
		}
	}
	if len(todo) == 0 {
		return
	}
	// A page runs in descending key order, so the highest section first:
	// one page covers every section from its start down to the last whose
	// leaves it ends inside of, and a round's calls grow with the pages its
	// sections fill, not with how many sections it opened.
	sort.Slice(todo, func(i, j int) bool { return todo[i] > todo[j] })
	srcs := s.partitionSources(ctx)
	if len(srcs) == 0 {
		return
	}
	for len(todo) > 0 && ctx.Err() == nil {
		covered, ok := s.loadOpenedPage(ctx, p, srcs, todo)
		if !ok {
			return
		}
		todo = todo[covered:]
	}
}

// loadOpenedPage loads the sections a page from the highest of todo covers,
// asking the peers in turn until one answers at or after the pull's start. It
// returns how many of todo it loaded, and false when no peer answered.
func (s *PulledState) loadOpenedPage(ctx context.Context, p *syncing, srcs []pull.Source, todo []sectionID) (int, bool) {
	// Each peer is asked twice: a page below the start may be a peer a
	// block behind, and the next ask finds it a block on.
	for i := 0; i < 2*len(srcs); i++ {
		turn := (s.pageTurn + i) % len(srcs)
		src := srcs[turn]
		// As of L, the records' end: what the page shows is then what the
		// records left, and the load raises no block the records must
		// catch up to (executor spec, "Sync", §3a; #4518).
		page, err := src.QueryBptPage(ctx, s.partition, &api.BptPageQuery{StartHash: todo[0].ScanStart(), Count: sectionPage, ForHeight: p.last})
		if err != nil && p.last != 0 {
			// A peer that cannot answer as of L answers its current state,
			// of a block past L, and the records catch up to it
			// (DIFFERENCES E29).
			page, err = src.QueryBptPage(ctx, s.partition, &api.BptPageQuery{StartHash: todo[0].ScanStart(), Count: sectionPage})
		}
		if err != nil {
			s.log.Info("A peer could not page a section; the next is asked", "partition", s.partition, "peer", sourceKey(src), "section", todo[0], "error", err)
			continue
		}
		if page.Block < p.start {
			s.log.Info("A section's page was of a block before the pull's start; it is asked again",
				"partition", s.partition, "peer", sourceKey(src), "page-block", page.Block, "start", p.start)
			continue
		}
		p.highest = max(p.highest, page.Block)
		p.sectionPages++
		s.pageTurn++

		// The sections the page covers whole: every one above the section
		// its last leaf is in, or every one, when the page is the tree's
		// last. A section larger than a page is paged alone.
		bySection := map[sectionID][]*api.BptLeafSummary{}
		var lastSec sectionID
		seen := false
		for _, e := range page.Entries {
			if e == nil {
				continue
			}
			es := bpt.SectionOf(e.KeyHash)
			if es > todo[0] {
				continue
			}
			bySection[es] = append(bySection[es], e)
			lastSec, seen = es, true
		}
		n := 0
		switch {
		case page.Done:
			n = len(todo)
		case seen:
			for n < len(todo) && todo[n] > lastSec {
				n++
			}
		}
		if n == 0 {
			leaves, urls, block, err := sectionLeavesAt(ctx, src, s.partition, todo[0], p.last)
			if err != nil && p.last != 0 {
				leaves, urls, block, err = sectionLeavesAt(ctx, src, s.partition, todo[0], 0)
			}
			if err != nil || block < p.start {
				s.log.Info("A peer could not page a large section; the next is asked", "partition", s.partition, "peer", sourceKey(src), "section", todo[0], "block", block, "error", err)
				continue
			}
			p.highest = max(p.highest, block)
			bySection = map[sectionID][]*api.BptLeafSummary{}
			for i, l := range leaves {
				bySection[todo[0]] = append(bySection[todo[0]], &api.BptLeafSummary{KeyHash: l.KeyHash, ValueHash: l.Hash, Account: urls[i]})
			}
			n = 1
		}

		var stale, unnamed []*url.URL
		for _, sec := range todo[:n] {
			local, err := s.localLeaves(sec)
			if err != nil {
				s.log.Info("This node's leaves of a section could not be read", "partition", s.partition, "section", sec, "error", err)
				return 0, false
			}
			named := map[[32]byte]bool{}
			for _, e := range bySection[sec] {
				named[e.KeyHash] = true
				if e.Account == nil || p.current[accountKey(e.Account)] {
					continue
				}
				if h, ok := local[e.KeyHash]; !ok || h.hash != e.ValueHash {
					stale = append(stale, e.Account)
				}
			}
			for k, h := range local {
				if !named[k] && h.url != nil && !p.current[accountKey(h.url)] {
					unnamed = append(unnamed, h.url)
				}
			}
		}
		// The page's own peer first. A leaf the node holds that the page
		// does not name is asked of every peer, and forgotten only if none
		// holds one.
		s.takeAccounts(ctx, p, append(append([]pull.Source(nil), srcs[turn:]...), srcs[:turn]...), stale)
		for _, u := range unnamed {
			s.pullOne(ctx, p, u)
		}
		for _, sec := range todo[:n] {
			p.opened[sec] = true
			s.secs.pulled(sec)
		}
		return n, true
	}
	return 0, false
}

// localLeaf is one leaf of this node's tree.
type localLeaf struct {
	url  *url.URL
	hash [32]byte
}

// localLeaves is this node's leaves in a section, by key hash; a marker has
// no account.
func (s *PulledState) localLeaves(sec sectionID) (map[[32]byte]localLeaf, error) {
	batch := s.db.Begin(false)
	defer batch.Discard()
	return leavesOf(batch.BPT(), sec)
}

func leavesOf(tree *bpt.BPT, sec sectionID) (map[[32]byte]localLeaf, error) {
	out := map[[32]byte]localLeaf{}
	start := sec.ScanStart()
	for {
		pairs, next, err := tree.GetRange(start, sectionPage)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		for _, kv := range pairs {
			kh := kv.Key.Hash()
			es := bpt.SectionOf(kh)
			if es > sec {
				continue
			}
			if es < sec {
				return out, nil
			}
			out[kh] = localLeaf{url: accountOfKey(kv.Key), hash: [32]byte(kv.Value)}
		}
		if len(pairs) < sectionPage {
			return out, nil
		}
		start = next
	}
}

// takeTop takes the top of a peer's tree, as of the peer's block T at or
// after L, and makes every section this node does not hold as the top has it
// not loaded (install). T is the highest block a reply was read at until L
// reaches it: a section changed after T is named by a record after it.
func (s *PulledState) takeTop(ctx context.Context, p *syncing) error {
	var last error = errors.NotFound.With("no peer to take the top of the tree from")
	for _, src := range s.partitionSources(ctx) {
		top, at, _, err := fetchTop(ctx, src, s.partition)
		if err != nil {
			last = errors.UnknownError.WithFormat("%s: %w", sourceKey(src), err)
			continue
		}
		if at < p.last {
			last = errors.Conflict.WithFormat("%s: the top is of block %d, before the records' %d", sourceKey(src), at, p.last)
			continue
		}
		// The node's own system accounts are not forgotten: its services
		// read them while it joins. Their sections are loaded by the pull,
		// over what the node holds.
		keep := map[sectionID]bool{}
		for _, u := range SystemAccounts(s.partition) {
			keep[database.SectionOfAccount(u)] = true
		}
		n, differ, err := s.install(top, at, keep)
		if err != nil {
			return err
		}
		for _, sec := range differ {
			p.opened[sec] = false
		}
		p.topTaken, p.topAt = true, at
		p.highest = max(p.highest, at)
		s.log.Info("Took the top of the BPT; the sections that differ are not loaded",
			"partition", s.partition, "block", at, "not-loaded", s.secs.count(), "replaced", n, "peer", sourceKey(src))
		return nil
	}
	return last
}

// fetchTop reads the top of a peer's tree: the root's stored block, stamped
// with the peer's block, and the block under every position of it that is a
// branch, as of the same block, checked block by block (bpt.
// SectionHashesFromBlocks).
func fetchTop(ctx context.Context, src pull.Source, partition *url.URL) (*bpt.SectionHashes, uint64, [32]byte, error) {
	rec, err := src.QueryBptBlock(ctx, partition, &api.BptBlockQuery{})
	if err != nil {
		return nil, 0, [32]byte{}, errors.UnknownError.WithFormat("the root's block: %w", err)
	}
	at, root := rec.Block, rec.BptRoot
	top := slotsOfRecord(rec)
	below := map[byte][]bpt.BlockSlot{}
	for _, sl := range top {
		if !sl.Branch {
			continue
		}
		sub, err := src.QueryBptBlock(ctx, partition, &api.BptBlockQuery{Prefix: []byte{sl.Index}, ForHeight: at})
		if err != nil {
			return nil, 0, [32]byte{}, errors.UnknownError.WithFormat("the block under %d as of %d: %w", sl.Index, at, err)
		}
		if sub.BptRoot != root {
			return nil, 0, [32]byte{}, errors.Conflict.WithFormat("the block under %d is of root %x, not %x", sl.Index, sub.BptRoot[:8], root[:8])
		}
		below[sl.Index] = slotsOfRecord(sub)
	}
	hashes, err := bpt.SectionHashesFromBlocks(root, top, below)
	if err != nil {
		return nil, 0, [32]byte{}, err
	}
	return hashes, at, root, nil
}

func slotsOfRecord(rec *api.BptBlockRecord) []bpt.BlockSlot {
	out := make([]bpt.BlockSlot, 0, len(rec.Slots))
	for _, s := range rec.Slots {
		if s != nil {
			out = append(out, bpt.BlockSlot{Index: byte(s.Index), Branch: s.Branch, KeyHash: s.KeyHash, Hash: s.Hash})
		}
	}
	return out
}

// ownTop is this node's own tree read by sections: every section's hash,
// markers included, and the sections under a stored block that could not be
// read as this tree's, with what was wrong with each block.
type ownTop struct {
	*bpt.SectionHashes
	root      [32]byte
	condemned []sectionID
	broken    []*bpt.InconsistentBlockError
}

// localTop reads this node's own tree by sections. A block below the root
// whose slots do not hash to the hash the root records for it condemns the
// sixteen sections under it rather than the read: they are taken again, and
// the rest of the tree is read as it is (executor spec, "Sync", §3b).
func localTop(tree *bpt.BPT) (*ownTop, error) {
	root, err := tree.GetRootHash()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	top, err := tree.GetStoredBlock(nil)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	out := &ownTop{root: root}
	below := map[byte][]bpt.BlockSlot{}
	skip := map[byte]bool{}
	for _, sl := range top.Slots {
		if !sl.Branch {
			continue
		}
		sub, err := tree.GetStoredBlock([]byte{sl.Index})
		var bad *bpt.InconsistentBlockError
		switch {
		case err == nil:
			below[sl.Index] = sub.Slots
		case stderrors.As(err, &bad):
			skip[sl.Index] = true
			out.broken = append(out.broken, bad)
		default:
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	out.SectionHashes, out.condemned, err = bpt.SectionHashesFromBlocksSkipping(root, top.Slots, below, skip)
	return out, err
}

// install makes this node's tree the top's: a section whose hash the node
// already holds stays as it is -- loaded, or not loaded under a marker of the
// same hash -- and every other has the node's leaves in it forgotten and,
// unless the top has it empty, a marker in their place. A section in keep is
// left as the node holds it and returned among differ, for the pull to load
// over it. It returns how many sections it replaced.
func (s *PulledState) install(top *bpt.SectionHashes, at uint64, keep map[sectionID]bool) (int, []sectionID, error) {
	read := s.db.Begin(false)
	local, err := localTop(read.BPT())
	if err != nil {
		read.Discard()
		return 0, nil, errors.UnknownError.WithFormat("read this node's own top: %w", err)
	}
	for _, b := range local.broken {
		s.repairLog.sectionHash(s.partition, at, b, top)
	}
	condemned := map[sectionID]bool{}
	for _, sec := range local.condemned {
		condemned[sec] = true
	}
	type change struct {
		sec    sectionID
		leaves map[[32]byte]localLeaf
	}
	var changes []change
	var differ []sectionID
	for i := 0; i < bpt.Sections; i++ {
		if !condemned[sectionID(i)] && local.Present[i] == top.Present[i] && local.Hash[i] == top.Hash[i] {
			continue
		}
		sec := sectionID(i)
		if keep[sec] {
			// Loaded by the pull, over what the node holds -- unless it is
			// condemned, when what the node holds cannot be read as its own
			// and is forgotten like any other section (§3b).
			differ = append(differ, sec)
			if !condemned[sec] {
				continue
			}
		}
		leaves, err := leavesOf(read.BPT(), sec)
		if err != nil {
			read.Discard()
			return 0, nil, errors.UnknownError.Wrap(err)
		}
		changes = append(changes, change{sec, leaves})
	}
	read.Discard()

	batch := s.db.Begin(true)
	defer batch.Discard()
	batch.TagWrites("install", at)
	for _, c := range changes {
		for k, l := range c.leaves {
			if bpt.IsSectionMarker(k) || l.url == nil {
				if err := batch.BPT().Delete(keys.KeyFromHash(k)); err != nil {
					return 0, nil, errors.UnknownError.Wrap(err)
				}
				continue
			}
			if err := batch.ForgetAccount(l.url); err != nil {
				return 0, nil, errors.UnknownError.WithFormat("forget %v: %w", l.url, err)
			}
		}
		if top.Present[c.sec] {
			h := top.Hash[c.sec]
			if err := batch.BPT().Insert(c.sec.MarkerKey(), h[:]); err != nil {
				return 0, nil, errors.UnknownError.Wrap(err)
			}
		}
	}
	if err := batch.Commit(); err != nil {
		return 0, nil, errors.UnknownError.WithFormat("write the top: %w", err)
	}

	// Every section but those kept is now the top's.
	check := s.db.Begin(false)
	got, err := localTop(check.BPT())
	check.Discard()
	if err != nil {
		return 0, nil, errors.UnknownError.Wrap(err)
	}
	if len(got.broken) > 0 {
		return 0, nil, errors.InternalError.WithFormat("with the top installed, %v", got.broken[0])
	}
	for _, b := range local.broken {
		s.repairLog.repaired(s.partition, b)
	}
	for i := 0; i < bpt.Sections; i++ {
		if !keep[sectionID(i)] && (got.Present[i] != top.Present[i] || got.Hash[i] != top.Hash[i]) {
			return 0, nil, errors.InternalError.WithFormat("section %d with the top installed is not the top's", i)
		}
	}
	return len(changes), differ, errors.UnknownError.Wrap(s.secs.read())
}

// Fill loads every section the node has not loaded while it executes, on a
// goroutine of its own beside the backfill and with its lifecycle (#4472):
// Run starts it after each handoff and ends it, and waits for it, before the
// node is demoted and pulls again (executor spec, "Sync", §3a, "The fill").
func (s *PulledState) Fill(ctx context.Context) {
	s.secs.fill(ctx, func() bool { return s.machine.State() == nodestate.StateActive })
}

// SectionsNotLoaded is how many sections of the state tree this node holds by
// their hash alone.
func (s *PulledState) SectionsNotLoaded() int { return s.secs.count() }

// SectionsTakenUnverified is how many sections a block has taken as a peer
// held them after this node's block, no peer's hashing to the marker: each is
// a block the root check decides, and may repair (executor spec, "Sync",
// §3a).
func (s *PulledState) SectionsTakenUnverified() int64 { return s.secs.unverified.Load() }
