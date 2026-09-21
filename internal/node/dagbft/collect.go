// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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

	// Block is the block this group is, stamped when it was collected.
	//
	// IT IS STAMPED, NOT COUNTED. The handoff used to derive it by
	// arithmetic: `collectFrom` captured at StartCollecting, `skip = q -
	// collectFrom`, and `buffer[skip:]` produced as blocks q+1 onwards. That
	// is only true if the node collected EVERY group from collectFrom+1 on,
	// and two callers broke it -- StartCollecting ran before Start() had set
	// the index it read, and the join's restart emptied the buffer and reset
	// the base while the network moved on. Neither violation is visible to
	// the handoff's guards, and the result is every block after the handoff
	// numbered wrong, silently: the divergence the join exists to prevent
	// (#4351). A number on the group is a fact recorded where it is known;
	// a hole in the sequence is then a thing the handoff can see.
	Block uint64

	// Reach is, per stream, the highest sequence number this group's block
	// carried: what the gap check is made against (#4362).
	Reach []execute.StreamReach
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

// StartCollecting puts the service in collecting mode: every committed group
// is taken into staging and buffered, and none is executed.
//
// `from` is the block this node's own EXECUTOR last executed, so the first
// group collected is block from+1 and each one after it is the next block. It
// is handed in rather than read here for the reason #4351 exists: the daemon
// calls this before Start(), and Start() is what sets the index this used to
// read, so it read zero and the mapping was repaired only by a later call
// that also threw the buffer away. The daemon holds the number from
// SystemData.ExecutedBlock, which no pull can move (#4344).
//
// It may be called once. A second call would have to say what the blocks
// already collected are, and there is no answer to that which is not a
// guess; a join that needs to start again restarts the node.
func (s *Service) StartCollecting(from uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.collecting {
		return
	}
	s.collecting = true
	s.bufferOverrun = false
	s.buffer = nil
	s.bufferBytes = 0
	s.nextCollected = from + 1
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

// Handoff leaves collecting mode at block q and produces the collected group
// numbered q + 1 and every one after it, in order. It blocks until that is
// done.
//
// The executor's state is q's state — the pull put it there — and staging has
// been settled at q, so the next block this node executes is q + 1, which is
// exactly what its peers execute next. From there it is a validator like any
// other (executor spec, "Sync", §5).
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
	// Each group carries the block it is. The groups at or below q are
	// blocks this node's state already contains and must NOT be produced
	// again: producing one of them as block q+1 would execute an old block's
	// transactions against a newer state, under a block number that is not
	// theirs — every node's divergence in one step.
	groups, err := groupsAfter(s.buffer, q)
	if err != nil {
		s.mu.Unlock()
		return errors.UnknownError.WithFormat("%s: %w", s.config.Partition.ID, err)
	}

	s.buffer = nil
	s.collecting = false
	s.lastBlockIndex = q
	s.mu.Unlock()

	s.logger.Info("Joined: executing from the block after the state",
		"partition", s.config.Partition.ID, "block", q, "toProduce", len(groups))

	for i, g := range groups {
		err := s.produce(g.Certs, g.Batches, g.Leader, g.IsLeader, g.payloadEntries(), false)
		if err != nil {
			return errors.UnknownError.WithFormat("produce buffered group %d of %d (round %d): %w",
				i+1, len(groups), g.Round(), err)
		}
		// Each one is a committed group this node has now executed. Without
		// this the executor reads as permanently behind by the size of the
		// buffer, and past MaxExecutionLag a joined validator's primary
		// proposes empty headers for ever (consensus spec, invariant 9).
		s.node.ReportExecuted()
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

// A blockCollector is an adapter that can take a committed block into the
// executor's staging without executing it (#4292).
type blockCollector interface {
	CollectBlock(ctx context.Context, params adapter.BlockParams) (*execute.CollectedBlock, error)
}

// collectIntoStaging takes one group into staging without executing it, and
// records what it carried on each stream.
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

	g.Reach = out.Reach

	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Debug("Collected a committed group into staging",
		"partition", s.config.Partition.ID,
		"block", g.Block,
		"round", g.Round(),
		"certs", len(g.Certs),
		"batches", len(g.Batches),
		"held", out.Held)
	return nil
}

// groupsAfter is the collected groups above block q, checked.
//
// It refuses rather than guessing, and the two refusals are different
// answers: the group for q+1 not being there at all is the state having run
// ahead of the blocks consensus has delivered, which is a wait; a hole in the
// numbers is a buffer that cannot say which block is which, which is a fault
// (#4351).
func groupsAfter(buffer []*CollectedGroup, q uint64) ([]*CollectedGroup, error) {
	i := 0
	for i < len(buffer) && buffer[i].Block <= q {
		i++
	}
	out := buffer[i:]
	if len(out) == 0 {
		return nil, errors.NotReady.WithFormat(
			"the state is block %d and no block after it has been collected yet", q)
	}
	if out[0].Block != q+1 {
		return nil, errors.Conflict.WithFormat(
			"the state is block %d and the first block collected after it is %d: the blocks between were never collected",
			q, out[0].Block)
	}
	for j := 1; j < len(out); j++ {
		if out[j].Block != out[j-1].Block+1 {
			return nil, errors.Conflict.WithFormat(
				"the collected blocks jump from %d to %d", out[j-1].Block, out[j].Block)
		}
	}
	return out, nil
}

// GapsAt reports whether the block collected as q+1 can be executed: per
// stream, whether the run from what the PULLED state says was delivered is
// contiguous through the numbers that block carried (executor spec, "Sync",
// §4).
//
// errors.NotReady means the block has not been collected yet.
func (s *Service) GapsAt(q uint64) ([]execute.StreamGap, error) {
	s.mu.RLock()
	var g *CollectedGroup
	for _, c := range s.buffer {
		if c.Block == q+1 {
			g = c
			break
		}
	}
	collected := len(s.buffer)
	s.mu.RUnlock()

	if g == nil {
		return nil, errors.NotReady.WithFormat(
			"%s: the state is block %d and the block after it is not among the %d collected",
			s.config.Partition.ID, q, collected)
	}

	gapper, ok := s.adapter.(stagingGapper)
	if !ok {
		return nil, errors.NotAllowed.WithFormat(
			"%s: this executor cannot say where its streams stand", s.config.Partition.ID)
	}
	gaps, err := gapper.StagingGaps(g.Reach)
	return gaps, errors.UnknownError.Wrap(err)
}

// A stagingGapper is an adapter whose executor can compare what a block
// carried against what the pulled state says was delivered.
type stagingGapper interface {
	StagingGaps([]execute.StreamReach) ([]execute.StreamGap, error)
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
	// is over: the blocks since the node started listening are no longer all
	// in hand, and nothing may be produced from a buffer with a hole in it.
	s.mu.Lock()
	full := len(s.buffer) >= maxCollectedGroups || s.bufferBytes+g.bytes() > maxCollectedBytes
	if full {
		s.bufferOverrun = true
		s.mu.Unlock()
		return errors.NotReady.WithFormat("%s: the join buffer is full at %d groups and %d bytes; the join cannot finish",
			s.config.Partition.ID, len(s.buffer), s.bufferBytes)
	}
	g.Block = s.nextCollected
	s.nextCollected++
	s.mu.Unlock()

	// Into staging, always. The node collects into ITS OWN stage from the
	// moment it starts listening; there is no peer's stage to load into
	// first and nothing to wait for before holding what arrives (#4322).
	err := s.collectIntoStaging(g)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer = append(s.buffer, g)
	s.bufferBytes += g.bytes()
	s.logger.Debug("Buffered a committed group while joining",
		"partition", s.config.Partition.ID,
		"block", g.Block,
		"round", leader.Header.Round,
		"certs", len(certs),
		"batches", len(batches),
		"buffered", len(s.buffer))
	return nil
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
