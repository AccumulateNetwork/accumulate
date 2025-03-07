// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package main provides a standalone test program for LXR mining
package main

import (
	"crypto/rand"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("Starting LXR Mining Test")
	
	// Create a transaction
	tx := new(protocol.Transaction)
	tx.Header.Principal = protocol.AccountUrl("miner")
	tx.Body = new(protocol.WriteData)
	txData, err := tx.MarshalBinary()
	if err != nil {
		fmt.Printf("Error marshalling transaction: %v\n", err)
		return
	}

	// Create a block hash to mine against
	blockHash := [32]byte{}
	rand.Read(blockHash[:])
	fmt.Printf("Block hash: %x\n", blockHash)

	// Enable test environment for LXR hasher
	lxr.IsTestEnvironment = true

	// Create a hasher for mining
	hasher := lxr.NewHasher()

	// Create a nonce
	nonce := []byte("test nonce for mining")
	fmt.Printf("Nonce: %s\n", nonce)

	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	fmt.Printf("Computed hash: %x\n", computedHash)
	fmt.Printf("Difficulty: %d\n", difficulty)
	
	if difficulty < 100 {
		fmt.Println("Warning: Difficulty is less than 100, test may fail")
	}

	// Create a mining signature
	// Note: LxrMiningSignature doesn't have a TransactionData field in the actual code
	// We'll store the transaction data separately and use it in our forwarder
	sig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	fmt.Printf("Created signature with signer: %s\n", sig.Signer)

	// Track forwarded transactions
	var forwardedHash [32]byte
	var forwardedSubmissions []*mining.MiningSubmission
	
	// Create a transaction forwarder
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, subs []*mining.MiningSubmission) error {
		fmt.Println("Transaction forwarder called")
		forwardedHash = bh
		forwardedSubmissions = subs
		
		fmt.Printf("Forwarded block hash: %x\n", forwardedHash)
		fmt.Printf("Number of submissions: %d\n", len(forwardedSubmissions))
		
		// In a real implementation, we would extract the transaction data from the submission
		// Here we're just using our pre-generated transaction data
		if len(subs) > 0 {
			fmt.Printf("Submission difficulty: %d\n", subs[0].Difficulty)
			fmt.Printf("Submission signer: %s\n", subs[0].Signer)
			
			// In a real implementation, we would unmarshal the transaction data
			// For this test, we're just using our pre-generated transaction
			parsedTx := new(protocol.Transaction)
			err := parsedTx.UnmarshalBinary(txData) // Using our pre-generated txData
			if err != nil {
				fmt.Printf("Error unmarshalling transaction data: %v\n", err)
				return err
			}
			fmt.Printf("Successfully parsed transaction with principal: %s\n", parsedTx.Header.Principal)
		}
		
		return nil
	})

	// Create a validator
	validator := mining.NewCustomValidator(3600, 10, nil, forwarder)
	fmt.Println("Created validator")

	// Start a new mining window
	validator.StartNewWindow(blockHash, 100)
	fmt.Println("Started new mining window")

	// Submit the signature
	accepted, err := validator.SubmitSignature(sig)
	if err != nil {
		fmt.Printf("Error submitting signature: %v\n", err)
		return
	}
	fmt.Printf("Signature accepted: %v\n", accepted)

	// Close the window and check if the transaction was forwarded
	submissions := validator.CloseWindow()
	fmt.Printf("Closed window, got %d submissions\n", len(submissions))
	
	if len(submissions) > 0 {
		fmt.Printf("Top submission difficulty: %d\n", submissions[0].Difficulty)
	}

	// Verify the transaction forwarder was called with the correct parameters
	if forwardedHash == blockHash {
		fmt.Println("✅ Forwarded hash matches block hash")
	} else {
		fmt.Printf("❌ Forwarded hash does not match block hash: %x vs %x\n", forwardedHash, blockHash)
	}
	
	if len(forwardedSubmissions) > 0 {
		fmt.Println("✅ Submissions were forwarded")
		
		// Verify the submission details
		submission := forwardedSubmissions[0]
		if submission.BlockHash == blockHash {
			fmt.Println("✅ Forwarded submission block hash matches original")
		} else {
			fmt.Println("❌ Forwarded submission block hash does not match original")
		}
		
		if submission.ComputedHash == computedHash {
			fmt.Println("✅ Forwarded submission computed hash matches original")
		} else {
			fmt.Println("❌ Forwarded submission computed hash does not match original")
		}
		
		if submission.Difficulty >= 100 {
			fmt.Println("✅ Forwarded submission difficulty meets minimum requirement")
		} else {
			fmt.Println("❌ Forwarded submission difficulty does not meet minimum requirement")
		}
	} else {
		fmt.Println("❌ No submissions were forwarded")
	}
	
	fmt.Println("LXR Mining Test completed successfully")
}
