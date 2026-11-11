// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A validator persists the header it authors before broadcasting it so a
// restart never authors that round again. The header was read back only
// when a checkpoint was restored (#4592): a restart that resumes none — no
// checkpoint for the executor's last block, or a store at genesis — knew
// nothing of the round and signed a second, different header for it.
func TestStart_ReadsAuthoredHeaderWithoutCheckpoint(t *testing.T) {
	dir := t.TempDir()
	var keys []ed25519.PrivateKey
	var infos []types.ValidatorInfo
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys = append(keys, priv)
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: 1})
	}
	comm := types.NewCommittee(infos, 0)
	node := func() *Node {
		n, err := NewNode(NodeConfig{Partition: "t", KeyPair: keys[0], StateDir: dir, NumWorkers: 1}, comm, nil, nil)
		require.NoError(t, err)
		return n
	}

	// Authored, persisted, broadcast; then the crash.
	n := node()
	n.ctx, n.cancel = context.WithCancel(context.Background())
	t.Cleanup(n.cancel)
	first := types.NewHeader(keys[0].Public().(ed25519.PublicKey), 50, 0, nil, nil)
	first.Timestamp = 1
	require.NoError(t, first.Sign(keys[0]))
	require.NoError(t, n.persistAuthored(first))

	// The same StateDir, without Restore: the service's no-checkpoint path.
	m := node()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	t.Cleanup(m.cancel)
	last, has := m.primary.LastAuthoredRound()
	require.True(t, has, "the restarted primary knows nothing of round 50")
	require.Equal(t, types.Round(50), last)

	// Reaching round 50 again authors nothing new.
	m.primary.SetRound(49)
	m.primary.AdvanceRound()
	again, err := m.dagStore.Authored()
	require.NoError(t, err)
	require.Equal(t, first.Digest(), again.Digest(), "a second, different header for round 50")
}

// A restart from a checkpoint whose tail holds the certificate of the header
// this node last authored has nothing left to collect votes for: the round
// is done. Reading the header back in NewNode, before the DAG was restored,
// put it back into vote collection (the certificate was not in the DAG
// yet), and the later hand-over from restoreTail found it there and left it:
// the certified header was rebroadcast and its batches proposed again.
func TestRestore_CertifiedAuthoredHeaderStaysOut(t *testing.T) {
	dir := t.TempDir()
	var keys []ed25519.PrivateKey
	var infos []types.ValidatorInfo
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys = append(keys, priv)
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: 1})
	}
	comm := types.NewCommittee(infos, 0)
	node := func() *Node {
		n, err := NewNode(NodeConfig{Partition: "t", KeyPair: keys[0], StateDir: dir, NumWorkers: 1}, comm, nil, nil)
		require.NoError(t, err)
		return n
	}

	// Authored at round 50, persisted, and certified by three of four.
	n := node()
	n.ctx, n.cancel = context.WithCancel(context.Background())
	t.Cleanup(n.cancel)
	h := types.NewHeader(keys[0].Public().(ed25519.PublicKey), 50, 0, nil, nil)
	h.Timestamp = 1
	require.NoError(t, h.Sign(keys[0]))
	require.NoError(t, n.persistAuthored(h))
	var sigs [][]byte
	var signers []uint16
	for i := 0; i < 3; i++ {
		v := types.NewVote(h.Digest(), 50, 0, keys[i].Public().(ed25519.PublicKey))
		require.NoError(t, v.Sign(keys[i]))
		sigs = append(sigs, v.Signature)
		signers = append(signers, uint16(i))
	}
	cert := types.NewCertificate(h, sigs, signers)
	name, err := n.dagStore.PutCertificate(cert)
	require.NoError(t, err)
	cp := &persist.Checkpoint{Partition: "t", CurrentRound: 50, Tail: []string{name}}

	// The same StateDir, restored from that checkpoint.
	m := node()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	t.Cleanup(m.cancel)
	require.NoError(t, m.Restore(cp))
	require.True(t, m.dag.Contains(cert.Digest()), "the tail's certificate is not in the DAG")
	last, has := m.primary.LastAuthoredRound()
	require.True(t, has)
	require.Equal(t, types.Round(50), last)
	require.False(t, m.primary.RoundStatus().OwnHeader,
		"the certified header is back in vote collection: it would be rebroadcast and its batches proposed again")
}

// The authored header is the one file whose loss is an equivocation: a node
// that cannot read it does not know which round never to author again, so
// it does not start. A zero-length file is what an unsynced write-and-rename
// leaves after a power loss; the node started anyway, logged the error, and
// ran on with no watermark.
func TestStart_RefusesZeroLengthAuthoredFile(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	comm := types.NewCommittee([]types.ValidatorInfo{{PublicKey: priv.Public().(ed25519.PublicKey), Stake: 1}}, 0)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"zero-length", nil},
		{"garbage", []byte("a few bytes that are not a header")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "authored")
			require.NoError(t, os.WriteFile(file, tc.data, 0644))
			_, err := NewNode(NodeConfig{Partition: "t", KeyPair: priv, StateDir: dir, NumWorkers: 1}, comm, nil, nil)
			require.Error(t, err, "the node started with an authored file it cannot read")
			require.Contains(t, err.Error(), file, "the error does not name the file")
		})
	}

	// The same read in Restore fails closed too.
	t.Run("restore", func(t *testing.T) {
		dir := t.TempDir()
		n, err := NewNode(NodeConfig{Partition: "t", KeyPair: priv, StateDir: dir, NumWorkers: 1}, comm, nil, nil)
		require.NoError(t, err)
		file := filepath.Join(dir, "authored")
		require.NoError(t, os.WriteFile(file, nil, 0644))
		err = n.Restore(&persist.Checkpoint{Partition: "t", CurrentRound: 5})
		require.Error(t, err, "Restore went on with an authored file it cannot read")
		require.Contains(t, err.Error(), file)
	})
}
