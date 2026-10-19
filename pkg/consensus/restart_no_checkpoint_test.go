// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
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
