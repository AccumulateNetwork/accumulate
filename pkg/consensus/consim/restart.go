// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// The checkpoint files, as internal/node/dagbft keeps them: the position of
// the block being produced and of the one before.
const (
	checkpointFile     = persist.CheckpointFile
	prevCheckpointFile = persist.PrevCheckpointFile
)

func (s *Sim) nodeDir(sn *simNode) string {
	return filepath.Join(s.cfg.StateDir, sn.part, fmt.Sprint(sn.val))
}

// checkpoint writes the consensus position of the block about to be
// executed from group, keeping the previous one, as the service's
// saveCheckpoint does.
func (s *Sim) checkpoint(sn *simNode, node *consensus.Node, group []*types.Certificate) {
	if s.cfg.StateDir == "" {
		return
	}
	if sn.cpCur == nil {
		sn.cpCur = persist.NewStore(s.nodeDir(sn))
		sn.cpCur.SetFilename(checkpointFile)
		sn.cpPrev = persist.NewStore(s.nodeDir(sn))
		sn.cpPrev.SetFilename(prevCheckpointFile)
	}
	cp := node.CheckpointAt(group[len(group)-1].Round())
	cp.BlockIndex = sn.height.Load() + 1
	if sn.lastSaved != nil {
		_ = sn.cpPrev.Save(sn.lastSaved)
	}
	if err := sn.cpCur.Save(cp); err == nil {
		sn.lastSaved = cp
	}
}

func (s *Sim) startConsume(ctx context.Context, sn *simNode) {
	cctx, cancel := context.WithCancel(ctx)
	sn.stopConsume = cancel
	sn.consumeDone = make(chan struct{})
	s.wg.Add(1)
	go s.consume(cctx, sn)
}

// Restart stops the named nodes of a partition — all of them before any
// comes back, so naming every node restarts the partition as a whole — and
// starts each again from its persisted state: a new consensus.Node on the
// same host, restored from the checkpoint whose block is the node's last
// executed block, as internal/node/dagbft's seedFromCheckpoint restores it.
func (s *Sim) Restart(part string, vals ...int) error {
	return s.restart(s.targets(part, vals), 0)
}

// RestartNetwork stops every node of every partition and, once all are
// stopped, starts each again from its persisted state, as a network whose
// containers are recreated together comes back.
func (s *Sim) RestartNetwork() error {
	return s.restart(s.allNodes(), 0)
}

func (s *Sim) allNodes() []*simNode {
	var all []*simNode
	for _, part := range s.parts {
		all = append(all, s.byPart[part]...)
	}
	return all
}

func (s *Sim) targets(part string, vals []int) []*simNode {
	var out []*simNode
	for _, v := range vals {
		out = append(out, s.byPart[part][v])
	}
	return out
}

func (s *Sim) restart(nodes []*simNode, executorFirst time.Duration) error {
	if s.cfg.StateDir == "" {
		return fmt.Errorf("restart needs Config.StateDir")
	}
	for _, sn := range nodes {
		sn.stopConsume()
		<-sn.consumeDone
	}
	time.Sleep(executorFirst)
	for _, sn := range nodes {
		old := sn.n()
		old.Stop()
		sn.stoppedEquiv += old.DAG().Equivocations()
	}
	for _, sn := range nodes {
		node, err := consensus.NewNode(sn.cfg, sn.committee, s.hosts[sn.val], sn.ps)
		if err != nil {
			return fmt.Errorf("restart %s/%d: %w", sn.part, sn.val, err)
		}
		// The executor's last block: with DiscardEmptyBlocks, the last one
		// that carried anything, as the real executor keeps no empty block.
		last := sn.height.Load()
		if s.cfg.DiscardEmptyBlocks {
			last = sn.kept.Load()
		}
		cp := persist.LoadResume(s.nodeDir(sn), sn.part, last)
		if cp == nil {
			return fmt.Errorf("restart %s/%d: no checkpoint to resume executor block %d from", sn.part, sn.val, last)
		}
		node.Restore(cp)
		sn.lastSaved = cp
		sn.height.Store(cp.BlockIndex)
		sn.nodeP.Store(node)
	}
	// Start them together, as a partition restarted as a whole comes back:
	// each start waits seconds for its gossip mesh, and one at a time the
	// first nodes back would run that long without the rest.
	errs := make(chan error, len(nodes))
	for _, sn := range nodes {
		go func() {
			if err := sn.n().Start(s.runCtx); err != nil {
				errs <- fmt.Errorf("restart %s/%d: %w", sn.part, sn.val, err)
				return
			}
			errs <- nil
		}()
	}
	for range nodes {
		if err := <-errs; err != nil {
			return err
		}
	}
	for _, sn := range nodes {
		s.startConsume(s.runCtx, sn)
	}
	return nil
}

// scheduledRestarts performs each configured restart once its partition
// has reached its height.
// It returns the partitions it restarted: the time a restart takes is not a
// stall, so the caller restarts their progress clocks.
func (s *Sim) scheduledRestarts(logf func(string, ...any)) ([]string, error) {
	var restarted []string
	for i := range s.cfg.Restarts {
		r := &s.cfg.Restarts[i]
		if r.done {
			continue
		}
		var h uint64
		for _, sn := range s.byPart[r.Part] {
			if x := sn.height.Load(); x > h {
				h = x
			}
		}
		if h < r.AtHeight {
			continue
		}
		r.done = true
		if r.Network {
			logf("RESTART every node of every partition at %s height %d", r.Part, h)
			if err := s.restart(s.allNodes(), r.ExecutorFirst); err != nil {
				return restarted, err
			}
			restarted = append(restarted, s.parts...)
			continue
		}
		logf("RESTART %s nodes %v at height %d", r.Part, r.Vals, h)
		if err := s.restart(s.targets(r.Part, r.Vals), r.ExecutorFirst); err != nil {
			return restarted, err
		}
		restarted = append(restarted, r.Part)
	}
	return restarted, nil
}

// Sequences reports, per node of a partition, the blocks it executed: the
// leader round and the certificates of each, in order.
func (s *Sim) Sequences(part string) [][]string {
	var out [][]string
	for _, sn := range s.byPart[part] {
		sn.seqMu.Lock()
		out = append(out, append([]string(nil), sn.seq...))
		sn.seqMu.Unlock()
	}
	return out
}

// CheckSequences reports the first block at which two nodes of a partition
// executed different things.
func (s *Sim) CheckSequences(part string) error {
	seqs := s.Sequences(part)
	for i := 1; i < len(seqs); i++ {
		n := min(len(seqs[0]), len(seqs[i]))
		for b := 0; b < n; b++ {
			if seqs[0][b] != seqs[i][b] {
				return fmt.Errorf("%s: node %d and node %d differ at block %d:\n  %s\n  %s",
					part, 0, i, b+1, seqs[0][b], seqs[i][b])
			}
		}
	}
	return nil
}

// Equivocations is the number of conflicting certificates the DAGs of a
// partition's nodes refused, including nodes since restarted.
func (s *Sim) Equivocations(part string) uint64 {
	var n uint64
	for _, sn := range s.byPart[part] {
		n += sn.stoppedEquiv + sn.n().DAG().Equivocations()
	}
	return n
}
