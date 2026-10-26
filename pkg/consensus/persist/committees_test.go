// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A checkpoint carries the committee of each round through a save and a
// load, so a restart orders its tail with them (#4593).
func TestCheckpoint_CarriesTheCommitteesThroughASaveAndALoad(t *testing.T) {
	var infos []types.ValidatorInfo
	for i := 0; i < 4; i++ {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		infos = append(infos, types.ValidatorInfo{PublicKey: pub, Stake: uint64(i + 1)})
	}
	first := types.NewCommittee(infos, 1)
	second := types.NewCommittee(infos[1:], 2)
	second.ActivationRound = 300

	cp := NewCheckpoint("p", 10, 1, 8, nil)
	cp.Committees = EncodeCommittees([]*types.Committee{first, second})
	store := NewStore(t.TempDir())
	require.NoError(t, store.Save(cp))
	loaded, err := store.Load()
	require.NoError(t, err)

	got, err := DecodeCommittees(loaded.Committees)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for i, want := range []*types.Committee{first, second} {
		require.Equal(t, want.Epoch, got[i].Epoch)
		require.Equal(t, want.ActivationRound, got[i].ActivationRound)
		require.Equal(t, want.Validators, got[i].Validators)
	}

	loaded.Committees[1].Validators[0].PublicKey = "zz"
	_, err = DecodeCommittees(loaded.Committees)
	require.Error(t, err)
}
