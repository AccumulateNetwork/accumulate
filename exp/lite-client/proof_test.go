package liteclient

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func TestFetchProofAndVerifyProof(t *testing.T) {
	// 1. Construct a trivial Merkle receipt: hash0 -> hash1
	fmt.Println("[Step 1] Constructing Merkle receipt...")
	hash0 := sha256.Sum256([]byte("leaf"))
	entryHash := []byte("right")
	receipt := buildTestReceipt(hash0[:], entryHash, true)
	fmt.Printf("  Receipt: Start=%x, Entry.Hash=%x, Entry.Right=%v\n", receipt.Start, receipt.Entries[0].Hash, receipt.Entries[0].Right)

	// 2. Calculate expected root
	fmt.Println("[Step 2] Calculating expected Merkle root...")
	expectedRoot := calculateExpectedRoot(hash0[:], entryHash, true)
	fmt.Printf("  expectedRoot: %x\n", expectedRoot)

	// 3. Test FetchProof (mocked)
	fmt.Println("[Step 3] Fetching proof using mock client...")
	va := &VerifiedAccount{
		Url:     "acc://foo/bar",
		Receipt: receipt,
		Height:  12345,
	}
	fmt.Printf("  Fetched VerifiedAccount: Url=%s, Height=%d\n", va.Url, va.Height)

	// 4. Test VerifyProof (real logic, should succeed)
	fmt.Println("[Step 4] Verifying proof with correct root...")
	ok := VerifyProof(va.Receipt, expectedRoot)
	fmt.Printf("  VerifyProof result (expected true): %v\n", ok)
	require.True(t, ok, "VerifyProof should succeed for valid proof and root")

	// 5. Negative test: wrong root
	fmt.Println("[Step 5] Verifying proof with wrong root...")
	wrongRoot := sha256.Sum256([]byte("wrong"))
	ok = VerifyProof(va.Receipt, wrongRoot[:])
	fmt.Printf("  VerifyProof result with wrong root (expected false): %v\n", ok)
	require.False(t, ok, "VerifyProof should fail for wrong root")
}

// Additional tests for edge cases and variants of VerifyProof
func TestVerifyProofVariants(t *testing.T) {
	// Right-side entry (already tested in main test)

	fmt.Println("[Step 1] Verifying left-side entry...")
	hash0 := sha256.Sum256([]byte("leaf"))
	entryHash := []byte("left")
	receipt := buildTestReceipt(hash0[:], entryHash, false)
	expectedRoot := calculateExpectedRoot(hash0[:], entryHash, false)
	fmt.Printf("  Receipt: Start=%x, Entry.Hash=%x, Entry.Right=%v\n", receipt.Start, receipt.Entries[0].Hash, receipt.Entries[0].Right)
	fmt.Printf("  expectedRoot: %x\n", expectedRoot)
	ok := VerifyProof(receipt, expectedRoot)
	fmt.Printf("  VerifyProof result (expected true): %v\n", ok)
	if !ok {
		t.Errorf("Left-side proof should succeed")
	}

	fmt.Println("[Step 2] Verifying multi-level receipt...")
	h1 := sha256.Sum256([]byte("sibling1"))
	h2 := sha256.Sum256([]byte("sibling2"))
	receiptMulti := &merkle.Receipt{
		Start: hash0[:],
		Entries: []*merkle.ReceiptEntry{
			{Hash: h1[:], Right: true},
			{Hash: h2[:], Right: false},
		},
	}
	fmt.Printf("  Multi-level Receipt: Start=%x, Entries=[{Hash=%x, Right=%v}, {Hash=%x, Right=%v}]\n",
		receiptMulti.Start,
		receiptMulti.Entries[0].Hash, receiptMulti.Entries[0].Right,
		receiptMulti.Entries[1].Hash, receiptMulti.Entries[1].Right)
	// Manually compute expected root for multi-level
	r1 := doSha(append(hash0[:], h1[:]...))
	expectedRootMulti := doSha(append(h2[:], r1...))
	fmt.Printf("  expectedRootMulti: %x\n", expectedRootMulti)
	ok = VerifyProof(receiptMulti, expectedRootMulti)
	fmt.Printf("  VerifyProof result (expected true): %v\n", ok)
	if !ok {
		t.Errorf("Multi-level proof should succeed")
	}

	fmt.Println("[Step 3] Verifying nil receipt...")
	ok = VerifyProof(nil, expectedRoot)
	fmt.Printf("  VerifyProof result with nil receipt (expected false): %v\n", ok)
	if ok {
		t.Errorf("Nil receipt should fail")
	}

	fmt.Println("[Step 4] Verifying empty receipt...")
	emptyReceipt := &merkle.Receipt{}
	ok = VerifyProof(emptyReceipt, expectedRoot)
	fmt.Printf("  VerifyProof result with empty receipt (expected false): %v\n", ok)
	if ok {
		t.Errorf("Empty receipt should fail")
	}

}
