// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/proof"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

func TestServiceCreateProof(t *testing.T) {
	// Setup
	db := database.OpenInMemory(nil)

	service := NewService(db, nil)
	defer service.Close()

	// Create test account
	batch := db.Begin(true)
	accountUrl := protocol.AccountUrl("alice")
	account := &protocol.UnknownAccount{Url: accountUrl}
	require.NoError(t, batch.Account(accountUrl).Main().Put(account))
	require.NoError(t, batch.Commit())

	// Test proof creation
	var root [32]byte
	for i := 0; i < 32; i++ {
		root[i] = byte(i)
	}

	req := &ProofRequest{
		Account: accountUrl,
		Anchor:  protocol.PartitionUrl("Directory"),
		Sequence: 1,
		Root:    root,
	}

	proof, err := service.CreateProof(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, proof)
	require.NotEmpty(t, proof.Proof)
	require.Equal(t, ProofTypeTransaction, proof.Type)
}

func TestServiceValidateProof(t *testing.T) {
	db := database.OpenInMemory(nil)

	service := NewService(db, nil)
	defer service.Close()

	var root [32]byte
	for i := 0; i < 32; i++ {
		root[i] = byte(i)
	}

	// Create a valid proof
	batch := db.Begin(true)
	accountUrl := protocol.AccountUrl("bob")
	account := &protocol.UnknownAccount{Url: accountUrl}
	require.NoError(t, batch.Account(accountUrl).Main().Put(account))
	require.NoError(t, batch.Commit())

	req := &ProofRequest{
		Account: accountUrl,
		Anchor:  protocol.PartitionUrl("Directory"),
		Sequence: 1,
		Root:    root,
	}

	proof, err := service.CreateProof(context.Background(), req)
	require.NoError(t, err)

	// Validate the proof - validation should succeed with the correct root
	proofHash := sha256Sum(proof.Proof)
	result, err := service.ValidateProof(context.Background(), proof.Proof, proofHash, req.Anchor)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Valid)

	// Validation should fail with incorrect root
	badRoot := [32]byte{}
	result, err = service.ValidateProof(context.Background(), proof.Proof, badRoot, req.Anchor)
	require.NoError(t, err)
	require.False(t, result.Valid)
}
