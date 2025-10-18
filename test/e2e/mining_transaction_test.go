// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMiningTransaction_BasicValidation(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Test 1: Valid mining transaction
	t.Run("ValidMiningTransaction", func(t *testing.T) {
		// Create a valid mining transaction
		minerADI := alice
		minerADIBytes := []byte(minerADI.String())
		minerADIHash := sha256.Sum256(minerADIBytes)

		// Create bound nonce (nonce + SHA256(miner_ADI))
		nonce := make([]byte, 8)
		copy(nonce, []byte("testnce1"))
		boundNonce := append(nonce, minerADIHash[:]...)

		// Create transaction data and block hash
		transactionData := []byte("test-transaction-data")
		blockHash := make([]byte, 32)
		copy(blockHash, []byte("test-block-hash-32-bytes-long-12"))

		// Create easy baseline target (all 0xFF bytes = maximum difficulty)
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:      boundNonce,
			TransactionData: transactionData,
			BlockHash:       blockHash,
			BaselineTarget:  baselineTarget, // Very easy target for testing
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		// Submit mining transaction
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())
	})

	// Test 2: Invalid bound nonce
	t.Run("InvalidBoundNonce", func(t *testing.T) {
		// Create mining transaction with invalid bound nonce
		invalidBoundNonce := []byte("invalid-nonce-wrong-format")

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:      invalidBoundNonce,
			TransactionData: []byte("test-data"),
			BlockHash:       make([]byte, 32),
			BaselineTarget:  baselineTarget,
			MinerADI:        alice,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		// Submit should fail
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Fails())
	})

	// Test 3: Missing required fields
	t.Run("MissingRequiredFields", func(t *testing.T) {
		// Test missing BoundNonce
		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			// BoundNonce missing
			TransactionData: []byte("test-data"),
			BlockHash:       make([]byte, 32),
			BaselineTarget:  baselineTarget,
			MinerADI:        alice,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(3).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Fails())
	})
}

func TestMiningTransaction_ProofOfWork(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Test 1: Valid proof-of-work (easy target)
	t.Run("ValidProofOfWork", func(t *testing.T) {
		minerADI := alice
		minerADIBytes := []byte(minerADI.String())
		minerADIHash := sha256.Sum256(minerADIBytes)

		// Create bound nonce that will result in a small hash
		nonce := make([]byte, 8)
		copy(nonce, []byte{0, 0, 0, 0, 0, 0, 0, 1}) // Small nonce for easy hash
		boundNonce := append(nonce, minerADIHash[:]...)

		transactionData := []byte("test-data")
		blockHash := make([]byte, 32)

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:      boundNonce,
			TransactionData: transactionData,
			BlockHash:       blockHash,
			BaselineTarget:  baselineTarget, // Very easy target
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(4).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())
	})

	// Test 2: Invalid proof-of-work (impossible target)
	t.Run("InvalidProofOfWork", func(t *testing.T) {
		minerADI := alice
		minerADIBytes := []byte(minerADI.String())
		minerADIHash := sha256.Sum256(minerADIBytes)

		nonce := make([]byte, 8)
		copy(nonce, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) // Large nonce
		boundNonce := append(nonce, minerADIHash[:]...)

		// Create impossible baseline target (all zeros = minimum difficulty)
		baselineTarget := make([]byte, 32)
		// Leave all bytes as 0x00 for impossible target

		miningTx := &MiningTransaction{
			BoundNonce:      boundNonce,
			TransactionData: []byte("test-data"),
			BlockHash:       make([]byte, 32),
			BaselineTarget:  baselineTarget, // Impossible target - hash must be 0
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(5).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Fails())
	})
}

func TestMiningTransaction_TransactionBodyConsensus(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Test 1: Valid transaction body with matching hash
	t.Run("ValidTransactionBodyWithHash", func(t *testing.T) {
		minerADI := alice
		minerADIBytes := []byte(minerADI.String())
		minerADIHash := sha256.Sum256(minerADIBytes)

		nonce := make([]byte, 8)
		boundNonce := append(nonce, minerADIHash[:]...)

		transactionBody := []byte("test-transaction-body")
		transactionHash := sha256.Sum256(transactionBody)

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:               boundNonce,
			TransactionData:          []byte("test-data"),
			BlockHash:                make([]byte, 32),
			BaselineTarget:           baselineTarget,
			MinerADI:                 minerADI,
			Timestamp:                uint64(time.Now().Unix()),
			EpochNumber:              1,
			CandidateTransactionHash: transactionHash[:],
			TransactionBody:          transactionBody,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(6).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())
	})

	// Test 2: Invalid transaction body with mismatched hash
	t.Run("InvalidTransactionBodyWithMismatchedHash", func(t *testing.T) {
		minerADI := alice
		minerADIBytes := []byte(minerADI.String())
		minerADIHash := sha256.Sum256(minerADIBytes)

		nonce := make([]byte, 8)
		boundNonce := append(nonce, minerADIHash[:]...)

		transactionBody := []byte("test-transaction-body")
		wrongHash := []byte("wrong-hash-wrong-hash-wrong-hash")

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:               boundNonce,
			TransactionData:          []byte("test-data"),
			BlockHash:                make([]byte, 32),
			BaselineTarget:           baselineTarget,
			MinerADI:                 minerADI,
			Timestamp:                uint64(time.Now().Unix()),
			EpochNumber:              1,
			CandidateTransactionHash: wrongHash,
			TransactionBody:          transactionBody,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(7).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Fails())
	})
}

func TestMiningTransaction_BoundNonceValidation(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Test 1: Valid bound nonce format
	t.Run("ValidBoundNonceFormat", func(t *testing.T) {
		minerADI := alice
		minerADIBytes := []byte(minerADI.String())
		minerADIHash := sha256.Sum256(minerADIBytes)

		// Create proper bound nonce: nonce + SHA256(miner_ADI)
		nonce := make([]byte, 16) // Arbitrary nonce length
		copy(nonce, []byte("test-nonce-12345"))
		boundNonce := append(nonce, minerADIHash[:]...)

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:      boundNonce,
			TransactionData: []byte("test-data"),
			BlockHash:       make([]byte, 32),
			BaselineTarget:  baselineTarget,
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(8).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())
	})

	// Test 2: Bound nonce with wrong ADI hash
	t.Run("BoundNonceWithWrongADIHash", func(t *testing.T) {
		minerADI := alice
		wrongADIBytes := []byte("wrong-adi-url")
		wrongADIHash := sha256.Sum256(wrongADIBytes)

		nonce := make([]byte, 16)
		copy(nonce, []byte("test-nonce-12345"))
		// Use wrong ADI hash in bound nonce
		boundNonce := append(nonce, wrongADIHash[:]...)

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:      boundNonce,
			TransactionData: []byte("test-data"),
			BlockHash:       make([]byte, 32),
			BaselineTarget:  baselineTarget,
			MinerADI:        minerADI, // Correct ADI, but bound nonce has wrong hash
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(9).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Fails())
	})

	// Test 3: Bound nonce too short
	t.Run("BoundNonceTooShort", func(t *testing.T) {
		// Create bound nonce that's less than 32 bytes
		shortBoundNonce := []byte("too-short")

		// Create easy baseline target
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF
		}

		miningTx := &MiningTransaction{
			BoundNonce:      shortBoundNonce,
			TransactionData: []byte("test-data"),
			BlockHash:       make([]byte, 32),
			BaselineTarget:  baselineTarget,
			MinerADI:        alice,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     1,
		}

		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				Body(miningTx).
				SignWith(alice, "book", "1").Version(1).Timestamp(10).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Fails())
	})
}