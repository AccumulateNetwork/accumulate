// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// voteRecordFixture is a four-validator committee whose first validator is
// the node under test and whose second authors the headers it votes for,
// with one state directory every node of the fixture restarts from.
type voteRecordFixture struct {
	cfg       NodeConfig
	committee *types.Committee
	author    *testKeys
}

type testKeys struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func newVoteRecordFixture(t *testing.T) *voteRecordFixture {
	var infos []types.ValidatorInfo
	var keys []*testKeys
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: 100})
		keys = append(keys, &testKeys{pub, priv})
	}
	return &voteRecordFixture{
		cfg: NodeConfig{
			Partition:    "bvn0",
			KeyPair:      keys[0].priv,
			NumWorkers:   1,
			WorkerConfig: worker.Config{BatchSize: 5},
			StateDir:     t.TempDir(),
		},
		committee: types.NewCommittee(infos, 1),
		author:    keys[1],
	}
}

// node is a node started from the fixture's state directory, as a restart
// starts one.
func (f *voteRecordFixture) node(t *testing.T) *Node {
	n, err := NewNode(f.cfg, f.committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// header is the author's header for round, differing from the author's
// other headers for the round only in its timestamp.
func (f *voteRecordFixture) header(t *testing.T, round types.Round, timestamp int64) *types.Header {
	h := types.NewHeader(f.author.pub, round, 1, nil, nil)
	h.Timestamp = timestamp
	if err := h.Sign(f.author.priv); err != nil {
		t.Fatal(err)
	}
	return h
}

func votesSent(n *Node) uint64 {
	_, _, _, sent := n.primary.Metrics()
	return sent
}

// refusesTheOtherHeader restarts a node from cp and checks that it refuses a
// second header from the author for round and votes again for the one it
// recorded.
func (f *voteRecordFixture) refusesTheOtherHeader(t *testing.T, cp *persist.Checkpoint, round types.Round) {
	t.Helper()
	n := f.node(t)
	n.Restore(cp)
	n.primary.OnHeaderReceived(f.header(t, round, 2))
	if votesSent(n) != 0 {
		t.Fatalf("restarted from the checkpoint at commit %d, the node voted for a second header from the author for round %d", cp.LastCommitRound, round)
	}
	n.primary.OnHeaderReceived(f.header(t, round, 1))
	if votesSent(n) != 1 {
		t.Fatalf("restarted from the checkpoint at commit %d, the node did not vote again for the header it had recorded at round %d", cp.LastCommitRound, round)
	}
}

// TestRestartKeepsTheVoteRecord: a node that voted for an author's header
// and restarted, checkpoint or not, refuses a second header from that author
// for that round (#4590). Before the fix the record of what the node had
// voted for started empty at every start.
func TestRestartKeepsTheVoteRecord(t *testing.T) {
	f := newVoteRecordFixture(t)
	n := f.node(t)
	n.primary.OnHeaderReceived(f.header(t, 0, 1))
	if votesSent(n) != 1 {
		t.Fatal("the node did not vote for the first header")
	}

	n2 := f.node(t)
	n2.primary.OnHeaderReceived(f.header(t, 0, 2))
	if votesSent(n2) != 0 {
		t.Fatal("the restarted node voted for a second header from the author for the round it had voted in")
	}
	n2.primary.OnHeaderReceived(f.header(t, 0, 1))
	if votesSent(n2) != 1 {
		t.Fatal("the restarted node did not vote again for the header it had recorded")
	}
}

// TestVoteRecordSurvivesTheCheckpointsBelowIt: a vote at round r survives
// two checkpoints whose tail floors are at or below r, and a restart from
// either refuses the other header for the round (#4590). A checkpoint that
// dropped every record, as one pruning at its own commit would, passed the
// restart test above, which writes no checkpoint.
func TestVoteRecordSurvivesTheCheckpointsBelowIt(t *testing.T) {
	f := newVoteRecordFixture(t)
	const r = 40
	n := f.node(t)
	n.primary.SetRound(r)
	n.primary.OnHeaderReceived(f.header(t, r, 1))
	if votesSent(n) != 1 {
		t.Fatal("the node did not vote for the first header")
	}

	// The first checkpoint's floor is a rescue window below r, the second's
	// is r itself: both keep the record.
	n.bullshark.SetLastCommitRound(r)
	c1 := n.CheckpointAt(0)
	n.CheckpointSaved(c1)
	n.bullshark.SetLastCommitRound(r + bullshark.RescueWindow)
	c2 := n.CheckpointAt(0)
	n.CheckpointSaved(c2)

	f.refusesTheOtherHeader(t, c1, r)
	f.refusesTheOtherHeader(t, c2, r)
}

// TestVoteRecordsFollowTheCheckpointsSaved: the vote records a checkpoint
// drops are chosen by the checkpoints saved, not by the ones computed
// (#4590). A checkpoint whose save failed is not on disk, and the one before
// it is still the one a restart may resume from. Before the fix the floor of
// the checkpoint computed last was the one the next checkpoint pruned down
// to, and the records the checkpoint on disk still needed were gone.
func TestVoteRecordsFollowTheCheckpointsSaved(t *testing.T) {
	f := newVoteRecordFixture(t)
	const r = 40
	n := f.node(t)
	n.primary.SetRound(r)
	n.primary.OnHeaderReceived(f.header(t, r, 1))
	if votesSent(n) != 1 {
		t.Fatal("the node did not vote for the first header")
	}

	// c1, floor 8, is saved. c2, floor 48, is computed, and its save fails.
	// c3, floor 68, is saved: the checkpoints on disk are c1 and c3.
	n.bullshark.SetLastCommitRound(r)
	c1 := n.CheckpointAt(0)
	n.CheckpointSaved(c1)
	n.bullshark.SetLastCommitRound(r + bullshark.RescueWindow + 8)
	_ = n.CheckpointAt(0)
	n.bullshark.SetLastCommitRound(r + bullshark.RescueWindow + 28)
	c3 := n.CheckpointAt(0)
	n.CheckpointSaved(c3)

	// Resumed from either checkpoint on disk, the node still holds the
	// record: c1's floor is below r, and c3 was saved after c1.
	f.refusesTheOtherHeader(t, c1, r)
	f.refusesTheOtherHeader(t, c3, r)

	// Once both checkpoints on disk are above r, the record is dropped.
	c4 := n.CheckpointAt(0)
	n.CheckpointSaved(c4)
	if got := n.dagStore.Voted(); len(got) != 0 {
		t.Fatalf("a record below the floor of both checkpoints on disk was kept: %v", got)
	}
}
