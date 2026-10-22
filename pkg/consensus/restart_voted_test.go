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

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestRestartKeepsTheVoteRecord: a node that voted for an author's header
// and restarted, checkpoint or not, refuses a second header from that author
// for that round (#4590). Before the fix the record of what the node had
// voted for started empty at every start.
func TestRestartKeepsTheVoteRecord(t *testing.T) {
	var infos []types.ValidatorInfo
	var privs []ed25519.PrivateKey
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: 100})
		privs = append(privs, priv)
	}
	committee := types.NewCommittee(infos, 1)
	cfg := NodeConfig{
		Partition:    "bvn0",
		KeyPair:      privs[0],
		NumWorkers:   1,
		WorkerConfig: worker.Config{BatchSize: 5},
		StateDir:     t.TempDir(),
	}
	header := func(timestamp int64) *types.Header {
		h := types.NewHeader(infos[1].PublicKey, 0, 1, nil, nil)
		h.Timestamp = timestamp
		if err := h.Sign(privs[1]); err != nil {
			t.Fatal(err)
		}
		return h
	}
	votesSent := func(n *Node) uint64 {
		_, _, _, sent := n.primary.Metrics()
		return sent
	}

	n, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	n.primary.OnHeaderReceived(header(1))
	if votesSent(n) != 1 {
		t.Fatal("the node did not vote for the first header")
	}

	n2, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	n2.primary.OnHeaderReceived(header(2))
	if votesSent(n2) != 0 {
		t.Fatal("the restarted node voted for a second header from the author for the round it had voted in")
	}
	n2.primary.OnHeaderReceived(header(1))
	if votesSent(n2) != 1 {
		t.Fatal("the restarted node did not vote again for the header it had recorded")
	}
}
