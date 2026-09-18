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

// maxCollectedGroups bounds the join's buffer.
//
// The spec says a joining node buffers every committed block from the moment
// it starts listening; in a process that is a bound or it is a memory fault,
// and this node is already holding every batch of every buffered block. At
// one group per leader round, four rounds a second, this is about half an
// hour of a live network — well past the time a state pull takes, and short
// of the point where the buffer is the largest thing in the process.
//
// Past it the join cannot be exact — the buffer would no longer be every
// block since the snapshot — so the buffer is marked overrun and the join
// must start again from a newer snapshot (#4294). It is a var only so a test
// can lower it.
var maxCollectedGroups = 8192

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
	out := s.buffer
	s.buffer = nil
	return out
}

// collectGroup takes one committed group into staging without executing it,
// and keeps it.
//
// The buffer is appended to only after the collect succeeds: a group whose
// staging intake failed is not a block this node may later produce as though
// it had collected it.
func (s *Service) collectGroup(certs []*types.Certificate, batches []*types.Batch, leader *types.Certificate, isLeader bool) error {
	collector, ok := s.adapter.(interface {
		CollectBlock(ctx context.Context, params adapter.BlockParams) (int, error)
	})
	if !ok {
		return errors.NotAllowed.WithFormat("%s: the adapter cannot collect a block without executing it",
			s.config.Partition.ID)
	}

	held, err := collector.CollectBlock(s.ctx, adapter.BlockParams{
		// No index: a collecting node does not know which block this is
		// until the join matches the root (#4294). Everything a collected
		// block does is keyed on the stream and the number, not the block.
		Time:        time.Unix(0, leader.Header.Timestamp).UTC(),
		IsLeader:    isLeader,
		LeaderRound: leader.Header.Round,
		Certificate: leader,
		Batches:     batches,
	})
	if err != nil {
		return fmt.Errorf("collect block: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buffer) >= maxCollectedGroups {
		s.bufferOverrun = true
		return errors.NotReady.WithFormat("%s: the join buffer is full at %d groups; the join must start again from a newer snapshot",
			s.config.Partition.ID, len(s.buffer))
	}
	s.buffer = append(s.buffer, &CollectedGroup{Certs: certs, Batches: batches, Leader: leader, IsLeader: isLeader})
	s.logger.Debug("Collected a committed group while joining",
		"partition", s.config.Partition.ID,
		"round", leader.Header.Round,
		"certs", len(certs),
		"batches", len(batches),
		"held", held,
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
