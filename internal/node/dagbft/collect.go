// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Collecting mode (executor spec, "Sync", step 1; #4292).
//
// A node that is joining — and a node that restarted, which is the same thing
// (#4205) — subscribes to consensus and takes every committed block from then
// on into TWO places: staging, collected rather than executed, and a buffer,
// in the order consensus committed them. It executes nothing. When the join
// has pulled the state and matched the root at a block Q, the buffered groups
// from Q + 1 on are produced as blocks in order and the node is a validator
// from there (#4294).
//
// Nothing here advances the executor's block index, saves a checkpoint,
// records a state hash or publishes a block event: none of those happened.

// A CollectedGroup is one committed leader group a joining node kept: the
// certificates it executed nothing of and the batches they name, in the
// certificate's canonical payload order. The leader is the last certificate,
// and its header carries the block's time and round.
type CollectedGroup struct {
	Certs    []*types.Certificate
	Batches  []*types.Batch
	Leader   *types.Certificate
	IsLeader bool
}

// Round is the leader round this group was committed at: the buffer's order,
// and what a produced block reports.
func (g *CollectedGroup) Round() types.Round { return g.Leader.Header.Round }

// Time is the block time this group would be produced with, before the
// strictly-increasing clamp: the leader's header timestamp, identical on
// every validator (#4054).
func (g *CollectedGroup) Time() time.Time {
	return time.Unix(0, g.Leader.Header.Timestamp).UTC()
}

// maxCollectedGroups and maxCollectedBytes bound the join's buffer.
//
// The spec says a joining node buffers every committed block from the moment
// it starts listening; in a process that is a bound or it is a memory fault,
// and this node is holding every batch of every buffered block. The count
// alone does not bound the memory — at 500 tps a block's batches are of the
// order of half a megabyte, so eight thousand groups is gigabytes before the
// count ever trips — so the bytes are bounded too, as staged proofs are and
// for the same reason (#4282).
//
// Past either bound the join cannot be exact — the buffer would no longer be
// every block since the snapshot — so it is marked overrun and the join must
// start again from a newer snapshot (#4294). They are vars only so a test can
// lower them.
var (
	maxCollectedGroups = 8192
	maxCollectedBytes  = 1 << 30 // 1 GiB of batches
)

// bytes is what this group costs to hold: its batches, which are the bulk of
// it by orders of magnitude.
func (g *CollectedGroup) bytes() int {
	n := 0
	for _, b := range g.Batches {
		for _, tx := range b.Transactions {
			n += len(tx)
		}
	}
	return n
}

// StartCollecting puts the service in collecting mode: committed groups are
// taken into staging and buffered instead of executed.
func (s *Service) StartCollecting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collecting = true
	s.bufferOverrun = false
}

// Collecting reports whether this node is joining: collecting committed
// blocks into staging and executing none of them.
func (s *Service) Collecting() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.collecting
}

// Buffered is the groups collected so far, oldest first.
func (s *Service) Buffered() []*CollectedGroup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*CollectedGroup, len(s.buffer))
	copy(out, s.buffer)
	return out
}

// BufferedCount is how many groups are buffered, without copying them.
func (s *Service) BufferedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.buffer)
}

// BufferOverrun reports that more blocks were committed than the buffer
// holds. The blocks between the snapshot and now are no longer all in hand,
// so the join cannot be exact and must start again (#4294).
func (s *Service) BufferOverrun() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bufferOverrun
}

// StopCollecting leaves collecting mode and hands over the buffer, emptied,
// in one step: a group committed between the two would otherwise be executed
// by neither path (#4294 produces what this returns).
func (s *Service) StopCollecting() []*CollectedGroup {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collecting = false
	s.bufferOverrun = false
	s.bufferBytes = 0
	out := s.buffer
	s.buffer = nil
	return out
}

// A handoffRequest is the join asking to leave collecting mode at a block.
// It is served by the block production loop, not by the caller's goroutine:
// the loop is the only thing that produces blocks, so a handoff that ran
// beside it could execute a buffered group and a newly committed one at once,
// and they would be the same block index (#4294).
type handoffRequest struct {
	q    uint64
	done chan error
}

// Handoff leaves collecting mode at block q and produces every buffered group
// in order, from q + 1. It blocks until that is done.
//
// The executor's state is q's state — the pull put it there — and staging has
// been settled at q (#4292), so the next block this node executes is q + 1,
// which is exactly what its peers execute next. From there it is a validator
// like any other (executor spec, "Sync", step 4).
func (s *Service) Handoff(q uint64) error {
	if !s.Collecting() {
		return errors.NotAllowed.WithFormat("%s: not joining", s.config.Partition.ID)
	}
	req := handoffRequest{q: q, done: make(chan error, 1)}
	select {
	case s.handoff <- req:
	case <-s.ctx.Done():
		return errors.UnknownError.Wrap(s.ctx.Err())
	}
	select {
	case err := <-req.done:
		return err
	case <-s.ctx.Done():
		return errors.UnknownError.Wrap(s.ctx.Err())
	}
}

// performHandoff runs in the block production loop. It takes the buffer and
// leaves collecting mode in one step, sets the block this node stands at, and
// produces what it collected.
//
// A group that fails to produce stops the handoff with the buffer already
// taken: the node is no longer collecting and no longer joining, and the
// groups that were not produced are gone. That is a fault, not a state to
// recover from in place — the node must join again — so it is returned to the
// caller and logged as an error rather than swallowed.
func (s *Service) performHandoff(q uint64) error {
	s.mu.Lock()
	if !s.collecting {
		s.mu.Unlock()
		return errors.NotAllowed.WithFormat("%s: not joining", s.config.Partition.ID)
	}
	if s.bufferOverrun {
		s.mu.Unlock()
		return errors.NotReady.WithFormat("%s: the join buffer overran; the join must start again", s.config.Partition.ID)
	}
	groups := s.buffer
	s.buffer = nil
	s.collecting = false
	s.lastBlockIndex = q
	s.mu.Unlock()

	s.logger.Info("Joined: executing from the block after the state",
		"partition", s.config.Partition.ID, "block", q, "buffered", len(groups))

	for i, g := range groups {
		err := s.produceGroup(g.Certs, g.Batches, g.Leader, g.IsLeader, g.payloadEntries())
		if err != nil {
			return errors.UnknownError.WithFormat("produce buffered group %d of %d (round %d): %w",
				i+1, len(groups), g.Round(), err)
		}
	}
	return nil
}

// payloadEntries is how many batches the group's certificates named: what
// says whether the block it produces is empty.
func (g *CollectedGroup) payloadEntries() int {
	n := 0
	for _, cert := range g.Certs {
		n += len(cert.Header.Payload)
	}
	return n
}

// ApplyStaging takes a peer's staging — `load` is the join's LoadStaging —
// and then applies to it every block this node has buffered since it started
// collecting, in order, and every one that arrives after.
//
// The order is the spec's (executor.md, "Sync", step 2): staging comes from a
// validator as of its block P, and the buffered blocks after P are applied to
// THAT staging. A node that collected into its own staging first would have
// nothing to load into — a stage with entries in it is not a peer's stage —
// and it would be holding, before it knew what its peers hold, whatever its
// partially pulled state made of the blocks it saw.
//
// It runs in the block production loop, like the handoff, because that is
// where the buffer is written: loading beside it would apply the buffered
// blocks and a newly committed one to staging at once, in no order.
func (s *Service) ApplyStaging(load func() error) error {
	if !s.Collecting() {
		return errors.NotAllowed.WithFormat("%s: not joining", s.config.Partition.ID)
	}
	req := stagingRequest{load: load, done: make(chan error, 1)}
	select {
	case s.applyStaging <- req:
	case <-s.ctx.Done():
		return errors.UnknownError.Wrap(s.ctx.Err())
	}
	select {
	case err := <-req.done:
		return err
	case <-s.ctx.Done():
		return errors.UnknownError.Wrap(s.ctx.Err())
	}
}

// A stagingRequest is the join asking to load a peer's staging and apply what
// has been buffered to it.
type stagingRequest struct {
	load func() error
	done chan error
}

// applyStagingNow runs in the block production loop.
func (s *Service) applyStagingNow(load func() error) error {
	s.mu.Lock()
	if !s.collecting {
		s.mu.Unlock()
		return errors.NotAllowed.WithFormat("%s: not joining", s.config.Partition.ID)
	}
	if s.stagingReady {
		s.mu.Unlock()
		return errors.NotAllowed.WithFormat("%s: staging has already been taken", s.config.Partition.ID)
	}
	buffered := make([]*CollectedGroup, len(s.buffer))
	copy(buffered, s.buffer)
	s.mu.Unlock()

	err := load()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for i, g := range buffered {
		err := s.collectIntoStaging(g)
		if err != nil {
			return errors.UnknownError.WithFormat("collect buffered group %d of %d (round %d): %w",
				i+1, len(buffered), g.Round(), err)
		}
	}

	s.mu.Lock()
	s.stagingReady = true
	s.mu.Unlock()
	s.logger.Info("Staging taken from a peer; the blocks since are applied to it",
		"partition", s.config.Partition.ID, "applied", len(buffered))
	return nil
}

// A blockCollector is an adapter that can take a committed block into the
// executor's staging without executing it (#4292).
type blockCollector interface {
	CollectBlock(ctx context.Context, params adapter.BlockParams) (*execute.CollectedBlock, error)
}

// collectIntoStaging takes one group into staging without executing it.
func (s *Service) collectIntoStaging(g *CollectedGroup) error {
	collector, ok := s.adapter.(blockCollector)
	if !ok {
		return errors.NotAllowed.WithFormat("%s: the adapter cannot collect a block without executing it",
			s.config.Partition.ID)
	}

	out, err := collector.CollectBlock(s.ctx, adapter.BlockParams{
		// No index: a collecting node does not know which block this is
		// until the join matches the root (#4294). Everything a collected
		// block does is keyed on the stream and the number, not the block.
		Time:        g.Time(),
		IsLeader:    g.IsLeader,
		LeaderRound: g.Round(),
		Certificate: g.Leader,
		Batches:     g.Batches,
	})
	if err != nil {
		// A block this node could not take into staging is a block it cannot
		// produce either, and the certificate is already marked executed, so
		// it will not come again: the buffer has a hole and the join must
		// start over rather than hand off a run of blocks with one missing.
		s.mu.Lock()
		s.bufferOverrun = true
		s.mu.Unlock()
		return fmt.Errorf("collect block: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nameAccounts(out.Accounts)
	s.logger.Debug("Collected a committed group into staging",
		"partition", s.config.Partition.ID,
		"round", g.Round(),
		"certs", len(g.Certs),
		"batches", len(g.Batches),
		"held", out.Held,
		"accounts", len(out.Accounts))
	return nil
}

// collectGroup keeps one committed group, and takes it into staging if
// staging is this node's peers' (ApplyStaging). Until then the group is only
// buffered: nothing is held before the node knows what its peers hold.
//
// The buffer is appended to only after the collect succeeds: a group whose
// staging intake failed is not a block this node may later produce as though
// it had collected it.
func (s *Service) collectGroup(certs []*types.Certificate, batches []*types.Batch, leader *types.Certificate, isLeader bool) error {
	// Refused here, before anything is buffered: a node whose executor cannot
	// collect would buffer blocks it can never take into staging, and the
	// only way out of that is to execute them — from a staging its peers do
	// not have (#4290).
	if _, ok := s.adapter.(blockCollector); !ok {
		return errors.NotAllowed.WithFormat("%s: the adapter cannot collect a block without executing it",
			s.config.Partition.ID)
	}

	g := &CollectedGroup{Certs: certs, Batches: batches, Leader: leader, IsLeader: isLeader}

	// The bounds are checked before anything is taken into staging, so a group
	// past them is in neither the buffer nor the stage. Past a bound the join
	// is over: the blocks since the snapshot are no longer all in hand, and
	// nothing may be produced from a buffer with a hole in it.
	s.mu.Lock()
	full := len(s.buffer) >= maxCollectedGroups || s.bufferBytes+g.bytes() > maxCollectedBytes
	if full {
		s.bufferOverrun = true
	}
	ready := s.stagingReady
	s.mu.Unlock()
	if full {
		return errors.NotReady.WithFormat("%s: the join buffer is full at %d groups and %d bytes; the join must start again from a newer snapshot",
			s.config.Partition.ID, len(s.buffer), s.bufferBytes)
	}

	if ready {
		err := s.collectIntoStaging(g)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer = append(s.buffer, g)
	s.bufferBytes += g.bytes()
	s.logger.Debug("Buffered a committed group while joining",
		"partition", s.config.Partition.ID,
		"round", leader.Header.Round,
		"certs", len(certs),
		"batches", len(batches),
		"staged", ready,
		"buffered", len(s.buffer))
	return nil
}


// maxNamedAccounts bounds the set of accounts the collected blocks have named
// and the pull has not yet been told about. It is large enough for the blocks
// of a long pull and small enough that a joining node cannot be made to hold
// the whole account tree in a map. Past it the names are dropped, and the
// pull falls back to what it finds by comparing the peer's BPT pages with its
// own — slower, and correct (#4293).
const maxNamedAccounts = 1 << 16

// nameAccounts records what a collected block named. The caller holds s.mu.
func (s *Service) nameAccounts(accounts []*url.URL) {
	if s.named == nil {
		s.named = map[string]*url.URL{}
	}
	for _, u := range accounts {
		if len(s.named) >= maxNamedAccounts {
			if !s.namedFull {
				s.namedFull = true
				s.logger.Info("The accounts the collected blocks name no longer fit; the pull will find the rest by page",
					"partition", s.config.Partition.ID, "limit", maxNamedAccounts)
			}
			return
		}
		s.named[strings.ToLower(u.String())] = u
	}
}

// NamedAccounts is every account the blocks collected since the last call
// named — what the state pull must fetch for those blocks (executor spec,
// "Sync", step 3). It drains: a round pulls what that round's blocks named.
func (s *Service) NamedAccounts() []*url.URL {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*url.URL, 0, len(s.named))
	for _, u := range s.named {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) < 0 })
	s.named = nil
	s.namedFull = false
	return out
}

// pruneCommitted retires the batches a committed group named from every
// worker.
//
// Record which block did the pruning. If a later certificate names one of
// these digests, the executor's wait diagnostic reports "pruned after block N"
// instead of an unattributable "missing=1" — that is the difference between
// naming the #4125 halt and guessing at it. Name the certificate, not just
// its round: a round is not unique — every validator authors a header per
// round — so "pruned at round 260" cannot distinguish the same certificate
// arriving twice from two certificates of the same round sharing a batch.
// Block 0 is a group a joining node collected: no block produced it.
func (s *Service) pruneCommitted(certs []*types.Certificate, blockIndex uint64) {
	for _, cert := range certs {
		if len(cert.Header.Payload) == 0 {
			continue
		}
		digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
		for _, entry := range cert.Header.Payload {
			digests = append(digests, entry.Digest)
		}
		where := fmt.Sprintf("block %d", blockIndex)
		if blockIndex == 0 {
			where = "collected while joining"
		}
		prunedBy := fmt.Sprintf("%s round %d cert %s author %x",
			where, cert.Header.Round, cert.Digest().String()[:16], cert.Header.Author[:4])
		commit := worker.CommitInfo{Cert: cert.Digest().String(), Detail: prunedBy}
		for _, w := range s.node.Workers() {
			w.PruneCommitted(digests, commit)
		}
	}
}
