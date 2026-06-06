// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestCommittee_Digest(t *testing.T) {
	validators := makeValidators(t, 4)

	t.Run("deterministic", func(t *testing.T) {
		c := types.NewCommittee(validators, 0)
		assert.Equal(t, c.Digest(), c.Digest())
	})

	t.Run("non-zero for populated committee", func(t *testing.T) {
		c := types.NewCommittee(validators, 0)
		assert.False(t, c.Digest().IsZero())
	})

	t.Run("independent of input ordering", func(t *testing.T) {
		c1 := types.NewCommittee(validators, 0)

		reversed := make([]types.ValidatorInfo, len(validators))
		for i, v := range validators {
			reversed[len(validators)-1-i] = v
		}
		c2 := types.NewCommittee(reversed, 0)

		assert.Equal(t, c1.Digest(), c2.Digest())
	})

	t.Run("changes with epoch", func(t *testing.T) {
		c1 := types.NewCommittee(validators, 0)
		c2 := types.NewCommittee(validators, 1)
		assert.NotEqual(t, c1.Digest(), c2.Digest())
	})

	t.Run("changes with stake", func(t *testing.T) {
		c1 := types.NewCommittee(validators, 0)

		mutated := make([]types.ValidatorInfo, len(validators))
		copy(mutated, validators)
		mutated[0] = types.ValidatorInfo{PublicKey: validators[0].PublicKey, Stake: validators[0].Stake + 1}
		c2 := types.NewCommittee(mutated, 0)

		assert.NotEqual(t, c1.Digest(), c2.Digest())
	})

	t.Run("changes with membership", func(t *testing.T) {
		c1 := types.NewCommittee(validators, 0)

		newPub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		extended := append(append([]types.ValidatorInfo{}, validators...),
			types.ValidatorInfo{PublicKey: newPub, Stake: 100})
		c2 := types.NewCommittee(extended, 0)

		assert.NotEqual(t, c1.Digest(), c2.Digest())
	})

	t.Run("does not mutate caller's validator ordering", func(t *testing.T) {
		c := types.NewCommittee(validators, 0)
		before := make([]types.ValidatorInfo, len(c.Validators))
		copy(before, c.Validators)
		_ = c.Digest()
		assert.Equal(t, before, c.Validators)
	})

	t.Run("clone has identical digest", func(t *testing.T) {
		c := types.NewCommittee(validators, 7)
		assert.Equal(t, c.Digest(), c.Clone().Digest())
	})
}
