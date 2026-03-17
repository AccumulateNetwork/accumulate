// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestValidatorUpdateHandler_AddValidator(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Add a new validator
	newPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	handler.AddValidator(newPub, 200)

	assert.True(t, handler.HasPendingUpdates())
	assert.Equal(t, 1, handler.PendingUpdateCount())
}

func TestValidatorUpdateHandler_RemoveValidator(t *testing.T) {
	v1 := newTestValidator(t)
	v2 := newTestValidator(t)
	validators := []*testValidator{v1, v2}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v1.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Remove a validator
	handler.RemoveValidator(v2.pub)

	assert.True(t, handler.HasPendingUpdates())
	assert.Equal(t, 1, handler.PendingUpdateCount())
}

func TestValidatorUpdateHandler_UpdateStake(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Update stake
	handler.UpdateStake(v.pub, 500)

	assert.True(t, handler.HasPendingUpdates())
	assert.Equal(t, 1, handler.PendingUpdateCount())
}

func TestValidatorUpdateHandler_CheckAndApplyUpdates(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Add a new validator
	newPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	handler.AddValidator(newPub, 200)

	// Updates should not be applied yet (round hasn't advanced)
	initialEpoch := p.CurrentEpoch()
	handler.CheckAndApplyUpdates()
	assert.Equal(t, initialEpoch, p.CurrentEpoch())
	assert.True(t, handler.HasPendingUpdates())

	// Advance the round
	p.AdvanceRound()

	// Now updates should be applied
	handler.CheckAndApplyUpdates()
	assert.False(t, handler.HasPendingUpdates())
	assert.Equal(t, initialEpoch+1, p.CurrentEpoch())

	// New validator should be in the committee
	newCommittee := p.Committee()
	assert.True(t, newCommittee.ContainsValidator(newPub))
	assert.Equal(t, uint64(200), newCommittee.StakeOf(newPub))
}

func TestValidatorUpdateHandler_OnValidatorSetChange(t *testing.T) {
	v1 := newTestValidator(t)
	v2 := newTestValidator(t)
	validators := []*testValidator{v1, v2}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v1.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Create a new validator set with one added and one with changed stake
	newPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var key1, key2, key3 [32]byte
	copy(key1[:], v1.pub)
	copy(key2[:], v2.pub)
	copy(key3[:], newPub)

	newValidators := []adapter.ValidatorInfo{
		{PublicKey: key1, Stake: 100, Active: true},
		{PublicKey: key2, Stake: 300, Active: true}, // Changed stake
		{PublicKey: key3, Stake: 150, Active: true}, // New validator
	}

	handler.OnValidatorSetChange(newValidators)

	assert.True(t, handler.HasPendingUpdates())
	// Should have 2 updates: stake change for v2 and addition of new validator
	assert.Equal(t, 2, handler.PendingUpdateCount())
}

func TestValidatorUpdateHandler_MultipleUpdatesBeforeApply(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Queue multiple updates
	newPub1, _, _ := ed25519.GenerateKey(rand.Reader)
	newPub2, _, _ := ed25519.GenerateKey(rand.Reader)

	handler.AddValidator(newPub1, 100)
	handler.AddValidator(newPub2, 200)
	handler.UpdateStake(v.pub, 500)

	assert.Equal(t, 3, handler.PendingUpdateCount())

	// Advance round and apply
	p.AdvanceRound()
	handler.CheckAndApplyUpdates()

	// All updates should be applied
	assert.False(t, handler.HasPendingUpdates())
	newCommittee := p.Committee()
	assert.Equal(t, 3, newCommittee.Len())
	assert.True(t, newCommittee.ContainsValidator(newPub1))
	assert.True(t, newCommittee.ContainsValidator(newPub2))
	assert.Equal(t, uint64(500), newCommittee.StakeOf(v.pub))
}

func TestValidatorUpdateHandler_NoPendingUpdates(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	assert.False(t, handler.HasPendingUpdates())
	assert.Equal(t, 0, handler.PendingUpdateCount())

	// Calling CheckAndApplyUpdates should be safe with no pending updates
	initialEpoch := p.CurrentEpoch()
	handler.CheckAndApplyUpdates()
	assert.Equal(t, initialEpoch, p.CurrentEpoch())
}

func TestValidatorUpdateHandler_ApplyAtRound(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)
	handler := NewValidatorUpdateHandler(p)

	// Set round to 5
	p.SetRound(5)

	newPub, _, _ := ed25519.GenerateKey(rand.Reader)
	handler.AddValidator(newPub, 100)

	assert.Equal(t, types.Round(5), handler.ApplyAtRound())

	// Advance to round 6 and apply
	p.SetRound(6)
	handler.CheckAndApplyUpdates()

	assert.False(t, handler.HasPendingUpdates())
}
