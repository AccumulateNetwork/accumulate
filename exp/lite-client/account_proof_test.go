// account_proof_test.go
//
// Unit and integration tests for `CreateAccountProof` and `VerifyAccountProof`.
//
// These tests cover Phase 1 of the lite client implementation, which involves
// creating and verifying account state proofs from the BPT.
//
// Responsibilities:
// - Ensure account URLs are parsed correctly
// - Confirm BPT receipt extraction behaves as expected
// - Validate construction of Merkle sibling path
// - Verify root hash matches current state
// - Confirm full proof verifies correctly
// - Ensure errors are thrown when expected

package liteclient

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestParseAccountUrl_Valid(t *testing.T) {
	validInputs := []string{
		"acc://alice",
		"acc://bob/book",
		"acc://authority/path/to/account",
	}

	for _, input := range validInputs {
		u, err := accurl.Parse(input)
		if err != nil {
			t.Errorf("Unexpected error parsing valid URL %q: %v", input, err)
			continue
		}

		// Ensure the stringified version matches the canonical format
		if got := u.String(); got != input {
			t.Errorf("Parsed URL mismatch:\n  Input:    %s\n  Returned: %s", input, got)
		}
	}
}

func TestParseAccountUrl_Invalid(t *testing.T) {
	invalidInputs := []string{
		"",                   // Empty string
		"http://notacc",      // Wrong scheme
		"acc:/missing-slash", // Incorrect scheme format
		"acc://",             // Missing host
		"acc://:invalid",     // Host starts with colon
		"//no-scheme",        // Missing scheme
	}

	for _, input := range invalidInputs {
		_, err := accurl.Parse(input)
		if err == nil {
			t.Errorf("Expected error for invalid URL input %q, but got nil", input)
		}
	}
}

// Store an account in an in-memory DB, create a proof, and check all fields are non-nil and well-formed.
func TestCreateAccountProof_ValidAccount(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

	u1, err := accurl.Parse("acc://alice")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	// Store the first account
	err = db.Update(func(batch *database.Batch) error {
		return batch.Account(u1).Main().Put(&protocol.UnknownAccount{Url: u1})
	})
	if err != nil {
		t.Fatalf("Failed to store first account: %v", err)
	}

	// Commit to update the BPT
	err = db.Update(func(batch *database.Batch) error { return nil })
	if err != nil {
		t.Fatalf("Failed to commit after first insert: %v", err)
	}

	// Test proof with only one account (edge case: may have no siblings)
	var proofSingle *AccountProof
	err = db.View(func(batch *database.Batch) error {
		proofSingle, err = CreateAccountProof(batch, u1.String())
		return err
	})
	if err != nil {
		t.Fatalf("Error creating proof for one account: %v", err)
	}
	if proofSingle == nil {
		t.Fatal("Proof (single) is nil")
	}
	if proofSingle.AccountUrl != u1.String() {
		t.Errorf("Unexpected AccountUrl: got %s, want %s", proofSingle.AccountUrl, u1.String())
	}
	if len(proofSingle.LeafHash) != sha256.Size {
		t.Errorf("Invalid LeafHash size: got %d, want %d", len(proofSingle.LeafHash), sha256.Size)
	}
	if len(proofSingle.RootHash) != sha256.Size {
		t.Errorf("Invalid RootHash size: got %d, want %d", len(proofSingle.RootHash), sha256.Size)
		if proofSingle.RootIndex <= 0 {
			t.Errorf("Invalid RootIndex for single proof: got %d, want > 0", proofSingle.RootIndex)
		}
	}
	if len(proofSingle.Siblings) == 0 {
		t.Log("No siblings found for single-node BPT (expected)")
	} else {
		t.Logf("Found %d sibling(s) in single-node case (unexpected)", len(proofSingle.Siblings))
	}
	if !VerifyAccountProof(proofSingle) {
		t.Error("Verification failed for single account proof")
	}

	// Add a second account to force BPT branching
	u2, _ := accurl.Parse("acc://bob")
	err = db.Update(func(batch *database.Batch) error {
		return batch.Account(u2).Main().Put(&protocol.UnknownAccount{Url: u2})
	})
	if err != nil {
		t.Fatalf("Failed to store second account: %v", err)
	}
	err = db.Update(func(batch *database.Batch) error { return nil })
	if err != nil {
		t.Fatalf("Failed to commit after second insert: %v", err)
	}

	// Generate proof again for the first account
	var proofMulti *AccountProof
	err = db.View(func(batch *database.Batch) error {
		proofMulti, err = CreateAccountProof(batch, u1.String())
		return err
	})
	if err != nil {
		t.Fatalf("Error creating proof with multiple accounts: %v", err)
	}
	if proofMulti == nil {
		t.Fatal("Proof (multi) is nil")
	}
	if len(proofMulti.Siblings) == 0 {
		t.Error("Expected sibling hashes with multiple accounts, got none")
		if proofMulti.RootIndex <= 0 {
			t.Errorf("Invalid RootIndex for multi proof: got %d, want > 0", proofMulti.RootIndex)
		}
	} else {
		t.Logf("Proof with multiple accounts includes %d sibling(s)", len(proofMulti.Siblings))
	}
	if !VerifyAccountProof(proofMulti) {
		t.Error("Verification failed for multi-account proof")
	}
}

// Attempt to generate a proof for a non-existent account, verify it returns an error.
func TestCreateAccountProof_MissingAccount(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

	// Use a URL for which we did not store any account
	accountStr := "acc://nonexistent"

	// Try to create the proof and expect an error
	var err error
	var proof *AccountProof
	err = db.View(func(batch *database.Batch) error {
		proof, err = CreateAccountProof(batch, accountStr)
		return err
	})

	if err == nil {
		t.Fatal("Expected error for missing account proof, got nil")
	}

	if proof != nil {
		t.Errorf("Expected nil proof for missing account, got: %+v", proof)
	}
	if proof != nil && proof.RootIndex != 0 {
		t.Errorf("Expected RootIndex 0 for missing proof, got %d", proof.RootIndex)
	}
}

// TestVerifyAccountProof_Correct
// Generate a valid proof and confirm that `VerifyAccountProof()` returns true.
func TestVerifyAccountProof_Correct(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

	u, err := accurl.Parse("acc://alice/correct")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	// Store the account
	err = db.Update(func(batch *database.Batch) error {
		return batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u})
	})
	if err != nil {
		t.Fatalf("Failed to insert account: %v", err)
	}

	// Commit to update BPT
	err = db.Update(func(batch *database.Batch) error { return nil })
	if err != nil {
		t.Fatalf("Failed to commit BPT state: %v", err)
	}

	// Generate the proof
	var proof *AccountProof
	err = db.View(func(batch *database.Batch) error {
		proof, err = CreateAccountProof(batch, u.String())
		return err
	})
	if err != nil {
		t.Fatalf("CreateAccountProof failed: %v", err)
	}

	// Verify that the proof is correct
	if !VerifyAccountProof(proof) {
		t.Error("Expected proof to verify successfully, but it failed")
	}
}

// Manually tamper with leaf/siblings/root and ensure verification fails.
func TestVerifyAccountProof_Incorrect(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

	u, err := accurl.Parse("acc://alice/incorrect")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	// Store the account
	err = db.Update(func(batch *database.Batch) error {
		return batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u})
	})
	if err != nil {
		t.Fatalf("Failed to insert account: %v", err)
	}

	// Commit to update BPT
	err = db.Update(func(batch *database.Batch) error { return nil })
	if err != nil {
		t.Fatalf("Failed to commit BPT state: %v", err)
	}

	// Generate the proof
	var proof *AccountProof
	err = db.View(func(batch *database.Batch) error {
		proof, err = CreateAccountProof(batch, u.String())
		return err
	})
	if err != nil {
		t.Fatalf("CreateAccountProof failed: %v", err)
	}

	// Tamper with the proof to force a failure
	if len(proof.LeafHash) > 0 {
		proof.LeafHash[0] ^= 0xFF // Flip one bit
	}

	if VerifyAccountProof(proof) {
		t.Error("Expected proof verification to fail after tampering, but it succeeded")
	}
}

// TestExtractSiblingsFromReceipt
func TestExtractSiblingsFromReceipt(t *testing.T) {
	// Mock a simple Merkle receipt with known sibling hashes
	var h1 = sha256.Sum256([]byte("sibling1"))
	var h2 = sha256.Sum256([]byte("sibling2"))
	var h3 = sha256.Sum256([]byte("sibling3"))
	mockHashes := [][]byte{
		h1[:],
		h2[:],
		h3[:],
	}

	// Construct the receipt entries
	var entries []*merkle.ReceiptEntry
	for _, h := range mockHashes {
		entry := &merkle.ReceiptEntry{Hash: h}
		entries = append(entries, entry)
	}

	receipt := &merkle.Receipt{
		Entries: entries,
	}

	// Call the function under test
	siblings := extractSiblingsFromReceipt(receipt)

	// Check that the extracted siblings match the original
	if len(siblings) != len(mockHashes) {
		t.Fatalf("Expected %d siblings, got %d", len(mockHashes), len(siblings))
	}
	for i := range siblings {
		if !bytes.Equal(siblings[i], mockHashes[i]) {
			t.Errorf("Sibling mismatch at index %d: got %x, want %x", i, siblings[i], mockHashes[i])
		}
	}
}

// TestCreateAccountProof_RealisticData
// Simulate multiple account types (e.g., TokenAccount, ADI) and verify each proof can be created and verified.
func TestCreateAccountProof_RealisticData(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

	type testCase struct {
		name    string
		account protocol.Account
	}

	// Define multiple realistic account types
	testCases := []testCase{
		{
			name:    "KeyBook",
			account: &protocol.KeyBook{Url: accurl.MustParse("acc://adi/book")},
		},
		{
			name: "ADI",
			account: &protocol.ADI{
				Url: accurl.MustParse("acc://adi"),
				AccountAuth: protocol.AccountAuth{
					Authorities: []protocol.AuthorityEntry{
						{Url: accurl.MustParse("acc://adi/book")},
					},
				},
			},
		},
		{
			name: "TokenAccount",
			account: &protocol.TokenAccount{
				Url:      accurl.MustParse("acc://user/token"),
				TokenUrl: accurl.MustParse("acc://acme"),
			},
		},
	}

	// Store all accounts
	for _, tc := range testCases {
		err := db.Update(func(batch *database.Batch) error {
			url := tc.account.GetUrl()
			return batch.Account((*accurl.URL)(url)).Main().Put(tc.account)
		})
		if err != nil {
			t.Fatalf("Failed to store %s: %v", tc.name, err)
		}
	}

	// Commit once after all inserts
	err := db.Update(func(batch *database.Batch) error { return nil })
	if err != nil {
		t.Fatalf("Failed to commit BPT state: %v", err)
	}

	// Generate and verify proofs for each
	for _, tc := range testCases {
		var proof *AccountProof
		err := db.View(func(batch *database.Batch) error {
			proof, err = CreateAccountProof(batch, tc.account.GetUrl().String())
			return err
		})
		if err != nil {
			t.Errorf("[%s] CreateAccountProof failed: %v", tc.name, err)
			continue
		}
		if proof == nil {
			t.Errorf("[%s] Expected non-nil proof", tc.name)
			continue
		}
		if proof.RootIndex < 0 {
			t.Errorf("[%s] Invalid RootIndex: got %d (should be >= 0)", tc.name, proof.RootIndex)
			t.Logf("Account: %s", tc.account.GetUrl())
			t.Logf("LeafHash: %x", proof.LeafHash)
			t.Logf("RootHash: %x", proof.RootHash)
			t.Logf("Siblings: %d", len(proof.Siblings))
			continue
		}
		if !VerifyAccountProof(proof) {
			t.Errorf("[%s] Expected proof to verify, but it failed", tc.name)
			t.Logf("Proof details: %+v", proof)
		}
	}
}
