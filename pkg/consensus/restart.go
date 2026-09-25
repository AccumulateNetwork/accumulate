// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"encoding/hex"
	"log/slog"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// handedGroup is a leader group handed to the executor: the checkpoint of a
// block produced from an earlier group must not count it committed.
type handedGroup struct {
	leader  types.Round
	digests []types.CertificateDigest
}

// order runs one certificate through Bullshark and hands the groups it
// commits to the executor, one group per leader. It reports false when the
// node is stopping.
func (n *Node) order(cert *types.Certificate) bool {
	// Ordering and recording what was handed over are one step under
	// orderMu, so a checkpoint never sees a commit it cannot attribute to
	// its leader.
	n.orderMu.Lock()
	outputs := n.bullshark.ProcessCertificate(cert)

	// Send committed certificates to the executor, grouped by the LEADER
	// that committed them: one group = one leader's sub-DAG in canonical
	// order = one executor block. The leader boundary is deterministic
	// across validators whatever order certificates arrived in; the
	// trigger boundary (this call) is not.
	var groups [][]*types.Certificate
	var groupLeader types.Round
	for _, output := range outputs {
		if len(groups) == 0 || output.Leader != groupLeader {
			groups = append(groups, nil)
			groupLeader = output.Leader
			if n.dagStore != nil {
				n.handed = append(n.handed, handedGroup{leader: output.Leader})
			}
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], output.Certificate)
		if n.dagStore != nil {
			h := &n.handed[len(n.handed)-1]
			h.digests = append(h.digests, output.Certificate.Digest())
		}
		n.certificatesCommitted.Add(1)
	}
	n.orderMu.Unlock()

	// NOTE: Batch pruning is handled by the executor after reading batches
	// from workers. It must not happen here: the channel is buffered, and
	// pruning before the executor reads causes "Missing batch for
	// certificate" errors.
	for _, group := range groups {
		// BLOCK, never drop: a dropped committed certificate means this node
		// silently skips transactions its peers execute — permanent state
		// divergence (#4122's shape). If the executor lags, backpressure here
		// is the correct response.
		select {
		case n.committed <- group:
			n.committedGroups.Add(1)
		case <-n.ctx.Done():
			return false
		}
	}
	return true
}

// CheckpointAt is the consensus position of the block produced from the
// group whose leader is at leaderRound: Bullshark's commit floor and
// commit-dedup as they were when that group was ordered, not as they are now
// — groups ordered since are in the executor's queue, and a restart from a
// position that counted them committed would never execute them. Zero means
// Bullshark's position now.
//
// With a StateDir, the checkpoint also carries the DAG's tail (#4448): every
// certificate from RescueWindow below the commit floor up, and the batches
// they name that this node holds, written to the DAGStore once each.
func (n *Node) CheckpointAt(leaderRound types.Round) *persist.Checkpoint {
	n.orderMu.Lock()
	lastCommit := n.bullshark.LastCommitRound()
	if leaderRound == 0 || leaderRound > lastCommit || n.dagStore == nil {
		leaderRound = lastCommit
	}
	committed := n.bullshark.GetCommitted()
	lastCommitted := n.bullshark.GetLastCommitted()
	keep := n.handed[:0]
	for _, g := range n.handed {
		if g.leader <= leaderRound {
			continue
		}
		keep = append(keep, g)
		for _, d := range g.digests {
			delete(committed, hex.EncodeToString(d[:]))
		}
	}
	n.handed = keep
	n.orderMu.Unlock()

	cp := persist.NewCheckpoint(n.config.Partition,
		n.primary.CurrentRound(), n.primary.CurrentEpoch(),
		leaderRound, lastCommitted)
	cp.Committed = committed
	if n.dagStore != nil {
		cp.Tail = n.writeTail(leaderRound)
	}
	return cp
}

// writeTail persists the certificates at and above RescueWindow below the
// commit floor, and the batches they name that this node holds or has
// already persisted, and removes what neither this checkpoint nor the
// previous one names, sparing what was written since it began.
func (n *Node) writeTail(lastCommit types.Round) []string {
	mark := n.dagStore.Mark()
	floor := types.Round(0)
	if lastCommit > bullshark.RescueWindow {
		floor = lastCommit - bullshark.RescueWindow
	}
	names := map[string]bool{}
	n.persistRounds(floor, names)
	tail := make([]string, 0, len(names))
	for name := range names {
		tail = append(tail, name)
	}
	sort.Strings(tail)

	retain := map[string]bool{}
	for name := range names {
		retain[name] = true
	}
	for name := range n.prevTail {
		retain[name] = true
	}
	n.dagStore.Retain(retain, mark)
	n.prevTail = names
	return tail
}

// persistRounds writes every certificate in the DAG at or above from, and
// the batches they name that this node holds, adding each name to names.
func (n *Node) persistRounds(from types.Round, names map[string]bool) {
	for _, round := range n.dag.Rounds() {
		if round < from {
			continue
		}
		for _, cert := range n.dag.GetRound(round) {
			name, err := n.dagStore.PutCertificate(cert)
			if err != nil {
				slog.Error("Persisting certificate for restart", "partition", n.config.Partition, "error", err)
				continue
			}
			names[name] = true
			n.persistBatches(cert.Header, names)
		}
	}
}

// persistBatches writes the batches the header names that this node holds,
// adding the name of each batch held or already persisted to names.
func (n *Node) persistBatches(h *types.Header, names map[string]bool) {
	if h == nil {
		return
	}
	for _, e := range h.Payload {
		if n.dagStore.HasBatch(e.Digest) {
			names[persist.BatchName(e.Digest)] = true
			continue
		}
		b, _ := multiWorkerBatchStore{n.workers}.GetBatch(e.Digest)
		if b == nil {
			continue
		}
		name, err := n.dagStore.PutBatch(b)
		if err != nil {
			slog.Error("Persisting batch for restart", "partition", n.config.Partition, "error", err)
			continue
		}
		names[name] = true
	}
}

// persistAuthored is the primary's hook: before a header this node authored
// is broadcast, the header is on disk, and so is the DAG from a few rounds
// below its parents up, with the batches this node holds. A block checkpoint
// follows the executor, which can be many rounds behind consensus when the
// node stops; the rounds between it and the frontier are then on disk only
// because each header this node authored wrote them.
func (n *Node) persistAuthored(h *types.Header) error {
	from := types.Round(0)
	if h.Round > 4 {
		from = h.Round - 4
	}
	n.persistRounds(from, map[string]bool{})
	for _, e := range h.Payload {
		if b, _ := (multiWorkerBatchStore{n.workers}).GetBatch(e.Digest); b != nil {
			if _, err := n.dagStore.PutBatch(b); err != nil {
				return err
			}
		}
	}
	return n.dagStore.PutAuthored(h)
}

// restoreTail loads every persisted certificate into the DAG and every
// persisted batch into the batch store, queues the certificates for
// ordering, and hands the last authored header to the primary. It reports
// how many certificates and batches it loaded.
func (n *Node) restoreTail(cp *persist.Checkpoint) (int, int) {
	if n.dagStore == nil {
		return 0, 0
	}
	var certs []*types.Certificate
	batches := 0
	for _, name := range n.dagStore.Names() {
		if c, err := n.dagStore.Certificate(name); err == nil {
			certs = append(certs, c)
			continue
		}
		if b, err := n.dagStore.Batch(name); err == nil {
			_ = multiWorkerBatchStore{n.workers}.StoreBatch(b)
			batches++
		}
	}
	if err := n.dag.Restore(certs); err != nil {
		slog.Error("Restoring the DAG tail", "partition", n.config.Partition, "error", err)
	}
	n.prevTail = map[string]bool{}
	for _, name := range cp.Tail {
		n.prevTail[name] = true
	}
	sort.Slice(certs, func(i, j int) bool {
		if certs[i].Round() != certs[j].Round() {
			return certs[i].Round() < certs[j].Round()
		}
		return string(certs[i].Author()) < string(certs[j].Author())
	})
	n.orderMu.Lock()
	n.restored = certs
	n.orderMu.Unlock()

	authored, err := n.dagStore.Authored()
	if err != nil {
		slog.Error("Reading the last authored header", "partition", n.config.Partition, "error", err)
	}
	n.primary.RestoreAuthored(authored)
	return len(certs), batches
}
