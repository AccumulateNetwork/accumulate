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
	checkpointFile     = "consensus-checkpoint.json"
	prevCheckpointFile = "consensus-checkpoint.prev.json"
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
	return s.restart(part, 0, vals...)
}

func (s *Sim) restart(part string, executorFirst time.Duration, vals ...int) error {
	if s.cfg.StateDir == "" {
		return fmt.Errorf("restart needs Config.StateDir")
	}
	nodes := s.byPart[part]
	for _, v := range vals {
		sn := nodes[v]
		sn.stopConsume()
		<-sn.consumeDone
	}
	time.Sleep(executorFirst)
	for _, v := range vals {
		sn := nodes[v]
		old := sn.n()
		old.Stop()
		sn.stoppedEquiv += old.DAG().Equivocations()
	}
	for _, v := range vals {
		sn := nodes[v]
		node, err := consensus.NewNode(sn.cfg, sn.committee, s.hosts[sn.val], sn.ps)
		if err != nil {
			return fmt.Errorf("restart %s/%d: %w", part, sn.val, err)
		}
		height := sn.height.Load()
		restored := false
		for _, file := range []string{checkpointFile, prevCheckpointFile} {
			store := persist.NewStore(s.nodeDir(sn))
			store.SetFilename(file)
			cp, err := store.Load()
			if err != nil || cp.BlockIndex != height {
				continue
			}
			node.Restore(cp)
			sn.lastSaved = cp
			restored = true
			break
		}
		if !restored {
			return fmt.Errorf("restart %s/%d: no checkpoint for block %d", part, sn.val, height)
		}
		sn.nodeP.Store(node)
		if err := node.Start(s.runCtx); err != nil {
			return fmt.Errorf("restart %s/%d: %w", part, sn.val, err)
		}
		s.startConsume(s.runCtx, sn)
	}
	return nil
}

// scheduledRestarts performs each configured restart once its partition
// has reached its height.
func (s *Sim) scheduledRestarts(logf func(string, ...any)) error {
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
		logf("RESTART %s nodes %v at height %d", r.Part, r.Vals, h)
		if err := s.restart(r.Part, r.ExecutorFirst, r.Vals...); err != nil {
			return err
		}
	}
	return nil
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
