package liteclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestVerifyBptProof(t *testing.T) {
	fmt.Println("=== Test: Manual Merkle Proof Verification ===")

	// Step 1: Define the leaf (account key)
	leaf := []byte("account1")
	hash := sha256.Sum256(leaf)
	leafHash := hash[:]
	fmt.Printf("\n[Step 1] Leaf Information\n")
	fmt.Printf("  Leaf Data  : %s\n", leaf)
	fmt.Printf("  Leaf Hash  : %x\n", leafHash)

	// Step 2: Define siblings (Merkle path)
	sibling1 := sha256.Sum256([]byte("sibling1"))
	sibling2 := sha256.Sum256([]byte("sibling2"))
	siblings := [][]byte{sibling1[:], sibling2[:]}

	fmt.Printf("\n[Step 2] Sibling Hashes\n")
	for i, sib := range siblings {
		fmt.Printf("  Level %d     : %x\n", i, sib)
	}

	// Step 3: Compute expected root hash
	current := leafHash
	fmt.Printf("\n[Step 3] Hash Computation\n")
	for i, sib := range siblings {
		h := sha256.New()
		if bytes.Compare(current, sib) < 0 {
			fmt.Printf("  Level %d - Hashing Order: leaf || sibling\n", i)
			h.Write(current)
			h.Write(sib)
		} else {
			fmt.Printf("  Level %d - Hashing Order: sibling || leaf\n", i)
			h.Write(sib)
			h.Write(current)
		}
		current = h.Sum(nil)
		fmt.Printf("  Level %d - Combined Hash : %x\n", i, current)
	}
	rootHash := current
	fmt.Printf("\n[Result] Expected Root Hash: %x\n", rootHash)

	// Step 4: Positive test
	fmt.Printf("\n[Step 4] Positive Test\n")
	ok := VerifyBptProof(leafHash, siblings, rootHash)
	fmt.Printf("  Proof Valid? : %v\n", ok)
	if !ok {
		t.Error("Expected proof verification to succeed")
	} else {
		fmt.Println("  Success: Proof verified as expected")
	}

	// Step 5: Negative test
	fmt.Printf("\n[Step 5] Negative Test\n")
	badRoot := make([]byte, 32)
	fmt.Printf("  Invalid Root : %s\n", hex.EncodeToString(badRoot))
	if VerifyBptProof(leafHash, siblings, badRoot) {
		t.Error("Expected proof verification to fail with invalid root")
	} else {
		fmt.Println("  Success: Proof correctly failed with invalid root")
	}
}

func TestVerifyMockedBptProof(t *testing.T) {
	fmt.Println("\n=== Test: Mocked BptProof Object Verification ===")

	proof := FetchProof("acc://alice")

	fmt.Printf("\n[Mock Proof Details]\n")
	fmt.Printf("  Leaf Hash   : %x\n", proof.LeafHash)
	for i, sib := range proof.Siblings {
		fmt.Printf("  Sibling[%d]  : %x\n", i, sib)
	}
	fmt.Printf("  Root Hash   : %x\n", proof.RootHash)

	if !proof.Verify() {
		t.Error("Expected mocked proof to verify correctly")
	} else {
		fmt.Println("\n[Result] Success: Mocked proof verified correctly")
	}
}
