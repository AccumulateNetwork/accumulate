// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
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
	// A leader above the round limit may belong to a committee this node has
	// not executed the change to; it is ordered when the limit has passed it
	// (orderDeferred, #4593).
	if n.beyondLimit(cert.Round()) {
		return true
	}

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
	// The oldest group waiting to be produced is where the next checkpoint
	// is taken; Bullshark keeps its dedup set a rescue window below it,
	// however far ordering runs ahead of the executor (#4594). Never more
	// than DAGGCDepth below the frontier: past that the checkpoint's tail
	// cannot be written, and a node that never checkpoints (collecting,
	// halted) would otherwise never move the floor.
	if len(n.handed) > 0 {
		floor := n.handed[0].leader
		lc := n.bullshark.LastCommitRound()
		depth := n.config.DAGGCDepth
		if lc > depth && floor < lc-depth {
			floor = lc - depth
		}
		n.bullshark.SetDedupFloor(floor)
	}
	n.orderMu.Unlock()

	// NOTE: Batch pruning is handled by the executor after reading batches
	// from workers. It must not happen here: the channel is buffered, and
	// pruning before the executor reads causes "Missing batch for
	// certificate" errors.
	for _, group := range groups {
		// Its batches stay in the store until the executor retires them
		// (consensus spec, invariant 3).
		for _, cert := range group {
			n.AwaitExecution(cert)
		}
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
// certificate from tailFloor up, and the batches
// they name that this node holds, written to the DAGStore once each.
func (n *Node) CheckpointAt(leaderRound types.Round) *persist.Checkpoint {
	n.orderMu.Lock()
	lastCommit := n.bullshark.LastCommitRound()
	if leaderRound == 0 || leaderRound > lastCommit || n.dagStore == nil {
		leaderRound = lastCommit
	}
	// Only the window the checkpoint keeps: a restored node prunes below it
	// on its first commit anyway.
	var from types.Round
	if w := types.Round(2 * bullshark.RescueWindow); leaderRound > w {
		from = leaderRound - w
	}
	committed := n.bullshark.GetCommittedFrom(from)
	lastCommitted := n.bullshark.GetLastCommitted()
	keep := n.handed[:0]
	for _, g := range n.handed {
		if g.leader <= leaderRound {
			continue
		}
		keep = append(keep, g)
		for _, d := range g.digests {
			d := d
			delete(committed, hex.EncodeToString(d[:]))
		}
	}
	n.handed = keep
	// Every later checkpoint is at or above this one (#4594). Without a DAG
	// store no group is recorded as handed, so the floor would never move
	// again: such a node reports none and prunes at the frontier.
	if n.dagStore != nil {
		n.bullshark.SetDedupFloor(leaderRound)
	}
	n.orderMu.Unlock()

	cp := persist.NewCheckpoint(n.config.Partition,
		n.primary.CurrentRound(), n.primary.CurrentEpoch(),
		leaderRound, lastCommitted)
	cp.Committed = committed
	cp.Committees = persist.EncodeCommittees(n.committees.Committees())
	if n.dagStore != nil {
		cp.Tail = n.writeTail(leaderRound)
	}
	return cp
}

// tailFloor is the round a checkpoint's tail begins at: the rescue window
// below the commit floor that an ordering walk reaches, and a weak-link
// window below that, so that the rounds whose parents a restored DAG does
// not hold are below every walk (#4595).
func tailFloor(lastCommit types.Round) types.Round {
	if depth := types.Round(bullshark.RescueWindow + types.WeakLinkWindow); lastCommit > depth {
		return lastCommit - depth
	}
	return 0
}

// writeTail persists the certificates from tailFloor up, and the batches they
// name that this node holds or has already persisted, keeps the batches of the
// header this node authored last, and removes what neither this checkpoint nor
// the previous one names, sparing what was written since it began.
func (n *Node) writeTail(lastCommit types.Round) []string {
	mark := n.dagStore.Mark()
	floor := tailFloor(lastCommit)
	names := map[string]bool{}
	n.persistRounds(floor, names)
	// The header this node authored last comes back with the DAG at a
	// restart (restoreTail), and a voter takes it only with every batch it
	// names. Authored after the rounds above and not yet certified, no
	// certificate names its batches, and Retain would remove them: the
	// restored header could then never be voted, and its author could not
	// author the round again (run 20261002T090546Z: BVN3 held at round
	// 25839 with two certificates of the three it needed).
	if h, err := n.dagStore.Authored(); err == nil && h != nil {
		n.persistBatches(h, names)
	}
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

// CheckpointSaved tells the node that cp is on disk. The vote records (#4590)
// below the lower of its tail floor and the previous saved checkpoint's are
// dropped: either checkpoint may be the one resumed. A checkpoint computed
// and not saved drops nothing, as the one still on disk needs its records.
func (n *Node) CheckpointSaved(cp *persist.Checkpoint) {
	if n.dagStore == nil || cp == nil {
		return
	}
	floor := tailFloor(cp.LastCommitRound)
	n.dagStore.RetainVoted(min(floor, n.prevFloor))
	n.prevFloor = floor
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

// persistInserted is the DAG's hook: a certificate is on disk as soon as it
// is in the DAG, with the batches it names that this node holds, so the
// store holds the DAG as of a crash and the restored tail is closed under
// parents whatever a weak link names (#4595). Writing from four rounds below
// each authored header missed a straggler named only as a weak link.
func (n *Node) persistInserted(c *types.Certificate) {
	if _, err := n.dagStore.PutCertificate(c); err != nil {
		slog.Error("Persisting certificate for restart", "partition", n.config.Partition, "error", err)
		return
	}
	n.persistBatches(c.Header, map[string]bool{})
}

// persistAuthored is the primary's hook: before a header this node authored
// is broadcast, the header is on disk with the batches it names. The DAG it
// was built on is already there, written as each certificate was inserted.
func (n *Node) persistAuthored(h *types.Header) error {
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
// how many certificates and batches it loaded. An authored header that is
// there but cannot be read is an error, not a warning: the node would not
// know which round never to author again (#4592).
//
// A batch named by a certificate in the checkpoint's committed set executed
// before the checkpoint, and goes into retention, with the certificate
// marked executed in the DAG: in the active store it would be certified and
// never pruned, and the tail would hold the store over its budget for as
// long as the DAG kept the certificate (#4598). That holds for this node's
// own batches too — held as own, the stale-own re-proposal would propose an
// executed batch again (invariant 7). Of the rest, the batches this node
// authored — named by its own certificates in the tail or by the header it
// last authored — come back as its own: the tail is a whole rescue window
// and can be larger than the active store, and the first eviction after a
// restart took the batch of the restored header with the rest, leaving no
// node to serve it and the round one vote short for ever (run
// 20261006T220257Z, #4545). The others are peer copies, as they were: a
// batch whose certificates are all absent from the set stays in the active
// store on the certificate's account (invariant 3), bounded by the DAG's
// retention.
func (n *Node) restoreTail(cp *persist.Checkpoint) (int, int, error) {
	if n.dagStore == nil {
		return 0, 0, nil
	}
	authored, err := n.dagStore.Authored()
	if err != nil {
		return 0, 0, fmt.Errorf("reading the last authored header: %w", err)
	}
	var certs []*types.Certificate
	var batchNames []string
	for _, name := range n.dagStore.Names() {
		if c, err := n.dagStore.Certificate(name); err == nil {
			certs = append(certs, c)
			continue
		}
		batchNames = append(batchNames, name)
	}
	ours := n.ownPayloads(certs, authored)
	// The tail certificates in the checkpoint's committed set, and for each
	// batch one such certificate naming it.
	committed := make(map[types.CertificateDigest]bool, len(cp.Committed))
	for h := range cp.Committed {
		raw, err := hex.DecodeString(h)
		if err != nil || len(raw) != len(types.CertificateDigest{}) {
			continue
		}
		var d types.CertificateDigest
		copy(d[:], raw)
		committed[d] = true
	}
	executed := make(map[types.BatchDigest]*types.Certificate)
	for _, c := range certs {
		if c.Header == nil || !committed[c.Digest()] {
			continue
		}
		for _, e := range c.Header.Payload {
			executed[e.Digest] = c
		}
	}
	batches := 0
	for _, name := range batchNames {
		b, err := n.dagStore.Batch(name)
		if err != nil {
			continue
		}
		d := b.Digest()
		switch c := executed[d]; {
		case c != nil:
			n.workers[workerFor(d[:], len(n.workers))].RestoreRetained(b, c.Digest().String())
		case ours[d]:
			n.workers[workerFor(d[:], len(n.workers))].RestoreOwn(b)
		default:
			_ = multiWorkerBatchStore{n.workers}.StoreBatch(b)
		}
		batches++
	}
	orphans, err := n.dag.Restore(tailFloor(cp.LastCommitRound), certs)
	if err != nil {
		slog.Error("Restoring the DAG tail", "partition", n.config.Partition, "error", err)
	}
	if len(orphans) > 0 {
		slog.Warn("Restored tail is not closed under parents; its orphans are taken in again as late certificates",
			"partition", n.config.Partition, "orphans", len(orphans), "lowest", orphans[0].Round())
		metrics.RestoreOrphansTotal.Add(float64(len(orphans)))
	}
	n.orphans = orphans
	inserted := certs[:0]
	for _, c := range certs {
		if n.dag.Contains(c.Digest()) {
			inserted = append(inserted, c)
		}
	}
	certs = inserted
	for _, c := range certs {
		if committed[c.Digest()] {
			n.dag.MarkExecuted(c)
		}
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

	n.primary.RestoreAuthored(authored)
	return len(certs), batches, nil
}

// takeInOrphans hands the primary what Restore dropped for a missing
// parent, lowest round first, as late certificates: each is held and its
// parents asked for, and enters the DAG once they arrive (#4595).
func (n *Node) takeInOrphans() {
	for _, c := range n.orphans {
		n.primary.OnRestoredCertificate(c)
	}
	n.orphans = nil
}

// ownPayloads is the set of batch digests the given certificates and header
// name under this node's key.
func (n *Node) ownPayloads(certs []*types.Certificate, authored *types.Header) map[types.BatchDigest]bool {
	self := n.config.KeyPair.Public().(ed25519.PublicKey)
	ours := map[types.BatchDigest]bool{}
	name := func(h *types.Header) {
		if h == nil || !h.Author.Equal(self) {
			return
		}
		for _, e := range h.Payload {
			ours[e.Digest] = true
		}
	}
	for _, c := range certs {
		name(c.Header)
	}
	name(authored)
	return ours
}
