// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"crypto/ed25519"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func makeValidators(t *testing.T, n int) []types.ValidatorInfo {
	t.Helper()
	validators := make([]types.ValidatorInfo, n)
	for i := 0; i < n; i++ {
		pub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100, // Equal stake
		}
	}
	return validators
}

func TestCommittee_TotalStake(t *testing.T) {
	t.Run("equal stakes", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 0)
		assert.Equal(t, uint64(400), committee.TotalStake())
	})

	t.Run("unequal stakes", func(t *testing.T) {
		pub1, _, _ := ed25519.GenerateKey(nil)
		pub2, _, _ := ed25519.GenerateKey(nil)
		validators := []types.ValidatorInfo{
			{PublicKey: pub1, Stake: 100},
			{PublicKey: pub2, Stake: 300},
		}
		committee := types.NewCommittee(validators, 0)
		assert.Equal(t, uint64(400), committee.TotalStake())
	})

	t.Run("caching", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 0)

		total1 := committee.TotalStake()
		total2 := committee.TotalStake()
		assert.Equal(t, total1, total2)
	})
}

func TestCommittee_QuorumThreshold(t *testing.T) {
	// For BFT, quorum is > 2/3 of total stake

	t.Run("4 validators with 100 stake each", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 0)

		// Total = 400, 2/3 = 266.67, so quorum = 267
		threshold := committee.QuorumThreshold()
		assert.Equal(t, uint64(267), threshold)

		assert.True(t, committee.HasQuorum(267))
		assert.True(t, committee.HasQuorum(300))
		assert.False(t, committee.HasQuorum(266))
		assert.False(t, committee.HasQuorum(200))
	})

	t.Run("3 validators", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee := types.NewCommittee(validators, 0)

		// Total = 300, 2/3 = 200, so quorum = 201
		threshold := committee.QuorumThreshold()
		assert.Equal(t, uint64(201), threshold)
	})
}

func TestCommittee_ValidityThreshold(t *testing.T) {
	// Validity is > 1/3 of total stake (f+1)

	t.Run("4 validators", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 0)

		// Total = 400, 1/3 = 133.33, so threshold = 134
		threshold := committee.ValidityThreshold()
		assert.Equal(t, uint64(134), threshold)

		assert.True(t, committee.HasValidity(134))
		assert.True(t, committee.HasValidity(200))
		assert.False(t, committee.HasValidity(133))
	})
}

func TestCommittee_FindValidator(t *testing.T) {
	validators := makeValidators(t, 4)
	committee := types.NewCommittee(validators, 0)

	t.Run("found", func(t *testing.T) {
		idx, ok := committee.FindValidator(validators[2].PublicKey)
		assert.True(t, ok)
		assert.Equal(t, uint16(2), idx)
	})

	t.Run("not found", func(t *testing.T) {
		pub, _, _ := ed25519.GenerateKey(nil)
		_, ok := committee.FindValidator(pub)
		assert.False(t, ok)
	})

	t.Run("caching", func(t *testing.T) {
		// First call builds index
		idx1, ok1 := committee.FindValidator(validators[1].PublicKey)
		require.True(t, ok1)

		// Second call uses cached index
		idx2, ok2 := committee.FindValidator(validators[1].PublicKey)
		require.True(t, ok2)

		assert.Equal(t, idx1, idx2)
	})
}

func TestCommittee_GetValidator(t *testing.T) {
	validators := makeValidators(t, 4)
	committee := types.NewCommittee(validators, 0)

	t.Run("valid index", func(t *testing.T) {
		v := committee.GetValidator(2)
		require.NotNil(t, v)
		assert.Equal(t, validators[2].PublicKey, v.PublicKey)
		assert.Equal(t, validators[2].Stake, v.Stake)
	})

	t.Run("out of bounds", func(t *testing.T) {
		v := committee.GetValidator(100)
		assert.Nil(t, v)
	})
}

func TestCommittee_StakeOf(t *testing.T) {
	pub1, _, _ := ed25519.GenerateKey(nil)
	pub2, _, _ := ed25519.GenerateKey(nil)
	pub3, _, _ := ed25519.GenerateKey(nil)

	validators := []types.ValidatorInfo{
		{PublicKey: pub1, Stake: 100},
		{PublicKey: pub2, Stake: 200},
	}
	committee := types.NewCommittee(validators, 0)

	assert.Equal(t, uint64(100), committee.StakeOf(pub1))
	assert.Equal(t, uint64(200), committee.StakeOf(pub2))
	assert.Equal(t, uint64(0), committee.StakeOf(pub3)) // Not in committee
}

func TestCommittee_Validate(t *testing.T) {
	t.Run("valid committee", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 0)
		err := committee.Validate()
		assert.NoError(t, err)
	})

	t.Run("empty committee", func(t *testing.T) {
		committee := types.NewCommittee(nil, 0)
		err := committee.Validate()
		assert.Error(t, err)
	})

	t.Run("invalid public key size", func(t *testing.T) {
		validators := []types.ValidatorInfo{
			{PublicKey: []byte{1, 2, 3}, Stake: 100}, // Invalid size
		}
		committee := types.NewCommittee(validators, 0)
		err := committee.Validate()
		assert.Error(t, err)
	})

	t.Run("zero stake", func(t *testing.T) {
		pub, _, _ := ed25519.GenerateKey(nil)
		validators := []types.ValidatorInfo{
			{PublicKey: pub, Stake: 0},
		}
		committee := types.NewCommittee(validators, 0)
		err := committee.Validate()
		assert.Error(t, err)
	})

	t.Run("duplicate validator", func(t *testing.T) {
		pub, _, _ := ed25519.GenerateKey(nil)
		validators := []types.ValidatorInfo{
			{PublicKey: pub, Stake: 100},
			{PublicKey: pub, Stake: 100}, // Duplicate
		}
		committee := types.NewCommittee(validators, 0)
		err := committee.Validate()
		assert.Error(t, err)
	})
}

func TestCommittee_Clone(t *testing.T) {
	validators := makeValidators(t, 4)
	original := types.NewCommittee(validators, 5)

	clone := original.Clone()

	assert.Equal(t, original.Len(), clone.Len())
	assert.Equal(t, original.Epoch, clone.Epoch)
	assert.Equal(t, original.TotalStake(), clone.TotalStake())

	// Modify clone shouldn't affect original
	clone.Validators[0].Stake = 999
	assert.NotEqual(t, original.Validators[0].Stake, clone.Validators[0].Stake)
}

func TestCommittee_ContainsValidator(t *testing.T) {
	validators := makeValidators(t, 4)
	committee := types.NewCommittee(validators, 0)

	assert.True(t, committee.ContainsValidator(validators[0].PublicKey))
	assert.True(t, committee.ContainsValidator(validators[3].PublicKey))

	pub, _, _ := ed25519.GenerateKey(nil)
	assert.False(t, committee.ContainsValidator(pub))
}

func TestCommittee_ConcurrentAccess(t *testing.T) {
	validators := makeValidators(t, 10)
	committee := types.NewCommittee(validators, 0)

	var wg sync.WaitGroup

	// Concurrent TotalStake calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = committee.TotalStake()
		}()
	}

	// Concurrent FindValidator calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = committee.FindValidator(validators[idx%len(validators)].PublicKey)
		}(i)
	}

	wg.Wait()
}

func TestCommittee_UpdateValidators(t *testing.T) {
	t.Run("add validator", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee := types.NewCommittee(validators, 1)

		newPub, _, _ := ed25519.GenerateKey(nil)
		updates := []types.ValidatorUpdate{
			{
				PublicKey: newPub,
				Type:      types.ValidatorUpdateAdd,
				NewStake:  200,
			},
		}

		newCommittee := committee.UpdateValidators(updates)

		assert.Equal(t, 4, newCommittee.Len())
		assert.Equal(t, uint64(2), newCommittee.Epoch) // Epoch incremented
		assert.True(t, newCommittee.ContainsValidator(newPub))
		assert.Equal(t, uint64(200), newCommittee.StakeOf(newPub))
	})

	t.Run("remove validator", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 1)

		updates := []types.ValidatorUpdate{
			{
				PublicKey: validators[1].PublicKey,
				Type:      types.ValidatorUpdateRemove,
			},
		}

		newCommittee := committee.UpdateValidators(updates)

		assert.Equal(t, 3, newCommittee.Len())
		assert.Equal(t, uint64(2), newCommittee.Epoch)
		assert.False(t, newCommittee.ContainsValidator(validators[1].PublicKey))
		// Others still present
		assert.True(t, newCommittee.ContainsValidator(validators[0].PublicKey))
		assert.True(t, newCommittee.ContainsValidator(validators[2].PublicKey))
		assert.True(t, newCommittee.ContainsValidator(validators[3].PublicKey))
	})

	t.Run("update stake", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee := types.NewCommittee(validators, 1)

		originalStake := validators[0].Stake
		newStake := uint64(500)

		updates := []types.ValidatorUpdate{
			{
				PublicKey: validators[0].PublicKey,
				Type:      types.ValidatorUpdateStake,
				NewStake:  newStake,
			},
		}

		newCommittee := committee.UpdateValidators(updates)

		assert.Equal(t, 3, newCommittee.Len())
		assert.Equal(t, newStake, newCommittee.StakeOf(validators[0].PublicKey))
		// Original committee unchanged
		assert.Equal(t, originalStake, committee.StakeOf(validators[0].PublicKey))
	})

	t.Run("multiple updates", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee := types.NewCommittee(validators, 1)

		newPub, _, _ := ed25519.GenerateKey(nil)
		updates := []types.ValidatorUpdate{
			{
				PublicKey: newPub,
				Type:      types.ValidatorUpdateAdd,
				NewStake:  150,
			},
			{
				PublicKey: validators[0].PublicKey,
				Type:      types.ValidatorUpdateRemove,
			},
			{
				PublicKey: validators[2].PublicKey,
				Type:      types.ValidatorUpdateStake,
				NewStake:  300,
			},
		}

		newCommittee := committee.UpdateValidators(updates)

		assert.Equal(t, 4, newCommittee.Len()) // 4 - 1 + 1 = 4
		assert.Equal(t, uint64(2), newCommittee.Epoch)
		assert.True(t, newCommittee.ContainsValidator(newPub))
		assert.False(t, newCommittee.ContainsValidator(validators[0].PublicKey))
		assert.Equal(t, uint64(300), newCommittee.StakeOf(validators[2].PublicKey))
	})

	t.Run("empty updates returns clone", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee := types.NewCommittee(validators, 1)

		newCommittee := committee.UpdateValidators(nil)

		assert.Equal(t, committee.Len(), newCommittee.Len())
		assert.Equal(t, committee.Epoch, newCommittee.Epoch)
	})

	t.Run("stake update on non-existent validator is no-op", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee := types.NewCommittee(validators, 1)

		unknownPub, _, _ := ed25519.GenerateKey(nil)
		updates := []types.ValidatorUpdate{
			{
				PublicKey: unknownPub,
				Type:      types.ValidatorUpdateStake,
				NewStake:  500,
			},
		}

		newCommittee := committee.UpdateValidators(updates)

		// Validator not added, so stake should be 0
		assert.Equal(t, uint64(0), newCommittee.StakeOf(unknownPub))
		assert.Equal(t, 3, newCommittee.Len())
	})
}

func TestCommittee_DiffWith(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee1 := types.NewCommittee(validators, 1)
		committee2 := types.NewCommittee(validators, 2)

		diff := committee1.DiffWith(committee2)
		assert.Empty(t, diff)
	})

	t.Run("validator added", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee1 := types.NewCommittee(validators, 1)

		newPub, _, _ := ed25519.GenerateKey(nil)
		extendedValidators := append(validators, types.ValidatorInfo{
			PublicKey: newPub,
			Stake:     100,
		})
		committee2 := types.NewCommittee(extendedValidators, 2)

		diff := committee1.DiffWith(committee2)

		require.Len(t, diff, 1)
		assert.Equal(t, types.ValidatorUpdateAdd, diff[0].Type)
		assert.Equal(t, newPub, ed25519.PublicKey(diff[0].PublicKey))
	})

	t.Run("validator removed", func(t *testing.T) {
		validators := makeValidators(t, 4)
		committee1 := types.NewCommittee(validators, 1)
		committee2 := types.NewCommittee(validators[:3], 2)

		diff := committee1.DiffWith(committee2)

		require.Len(t, diff, 1)
		assert.Equal(t, types.ValidatorUpdateRemove, diff[0].Type)
		assert.Equal(t, validators[3].PublicKey, ed25519.PublicKey(diff[0].PublicKey))
	})

	t.Run("stake changed", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee1 := types.NewCommittee(validators, 1)

		// Create committee2 with different stake for first validator
		validators2 := make([]types.ValidatorInfo, len(validators))
		copy(validators2, validators)
		validators2[0] = types.ValidatorInfo{
			PublicKey: validators[0].PublicKey,
			Stake:     500,
		}
		committee2 := types.NewCommittee(validators2, 2)

		diff := committee1.DiffWith(committee2)

		require.Len(t, diff, 1)
		assert.Equal(t, types.ValidatorUpdateStake, diff[0].Type)
		assert.Equal(t, uint64(500), diff[0].NewStake)
	})

	t.Run("nil target", func(t *testing.T) {
		validators := makeValidators(t, 3)
		committee1 := types.NewCommittee(validators, 1)

		diff := committee1.DiffWith(nil)
		assert.Nil(t, diff)
	})
}
