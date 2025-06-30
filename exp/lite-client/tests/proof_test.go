package liteclient

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func TestFetchProofAndVerifyProof(t *testing.T) {
	fmt.Println("\n[TEST] TestFetchProofAndVerifyProof")

	// Step 1: Construct a trivial Merkle receipt
	fmt.Println("→ [1] Constructing a simple Merkle receipt...")
	hash0 := sha256.Sum256([]byte("leaf"))
	entryHash := []byte("right")
	receipt := buildTestReceipt(hash0[:], entryHash, true)
	fmt.Printf("    Receipt:\n      Start = %x\n      Entry.Hash = %x\n      Entry.Right = %v\n",
		receipt.Start, receipt.Entries[0].Hash, receipt.Entries[0].Right)

	// Step 2: Calculate expected root
	fmt.Println("→ [2] Calculating expected root...")
	expectedRoot := calculateExpectedRoot(hash0[:], entryHash, true)
	fmt.Printf("    Expected root: %x\n", expectedRoot)

	// Step 3: Create mock VerifiedAccount
	fmt.Println("→ [3] Mocking VerifiedAccount...")
	va := &VerifiedAccount{
		Url:     "acc://foo/bar",
		Receipt: receipt,
		Height:  12345,
	}
	fmt.Printf("    VerifiedAccount: Url = %s, Height = %d\n", va.Url, va.Height)

	// Step 4: Verify correct proof
	fmt.Println("→ [4] Verifying correct proof...")
	ok := VerifyProof(va.Receipt, expectedRoot)
	fmt.Printf("    Result: %v (expected: true)\n", ok)
	require.True(t, ok, "Valid proof should succeed")

	// Step 5: Verify against wrong root
	fmt.Println("→ [5] Verifying with wrong root...")
	wrongRoot := sha256.Sum256([]byte("wrong"))
	ok = VerifyProof(va.Receipt, wrongRoot[:])
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Invalid root should fail")
}

func TestVerifyProofVariants(t *testing.T) {
	fmt.Println("\n[TEST] TestVerifyProofVariants")

	// Step 1: Left-side sibling
	fmt.Println("→ [1] Verifying left-side entry...")
	hash0 := sha256.Sum256([]byte("leaf"))
	entryHash := []byte("left")
	receipt := buildTestReceipt(hash0[:], entryHash, false)
	expectedRoot := calculateExpectedRoot(hash0[:], entryHash, false)
	fmt.Printf("    Left-side expected root: %x\n", expectedRoot)
	ok := VerifyProof(receipt, expectedRoot)
	fmt.Printf("    Result: %v (expected: true)\n", ok)
	require.True(t, ok, "Left-side proof should succeed")

	// Step 2: Multi-level receipt
	fmt.Println("→ [2] Verifying multi-level receipt...")
	h1 := sha256.Sum256([]byte("sibling1"))
	h2 := sha256.Sum256([]byte("sibling2"))
	receiptMulti := &merkle.Receipt{
		Start: hash0[:],
		Entries: []*merkle.ReceiptEntry{
			{Hash: h1[:], Right: true},
			{Hash: h2[:], Right: false},
		},
	}
	r1 := doSha(append(hash0[:], h1[:]...))
	expectedRootMulti := doSha(append(h2[:], r1...))
	fmt.Printf("    Multi-level expected root: %x\n", expectedRootMulti)
	ok = VerifyProof(receiptMulti, expectedRootMulti)
	fmt.Printf("    Result: %v (expected: true)\n", ok)
	require.True(t, ok, "Multi-level proof should succeed")

	// Step 3: Nil receipt
	fmt.Println("→ [3] Verifying nil receipt...")
	ok = VerifyProof(nil, expectedRoot)
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Nil receipt should fail")

	// Step 4: Empty receipt
	fmt.Println("→ [4] Verifying empty receipt...")
	emptyReceipt := &merkle.Receipt{}
	ok = VerifyProof(emptyReceipt, expectedRoot)
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Empty receipt should fail")
}

func TestVerifyProofNilStart(t *testing.T) {
	fmt.Println("\n[TEST] TestVerifyProofNilStart")
	entryHash := sha256.Sum256([]byte("right"))
	receipt := &merkle.Receipt{
		Start: nil,
		Entries: []*merkle.ReceiptEntry{
			{Hash: entryHash[:], Right: true},
		},
	}
	expected := sha256.Sum256([]byte("dummy"))
	ok := VerifyProof(receipt, expected[:])
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Receipt with nil Start should fail")
}

func TestVerifyProofNilEntryHash(t *testing.T) {
	fmt.Println("\n[TEST] TestVerifyProofNilEntryHash")
	start := sha256.Sum256([]byte("leaf"))
	receipt := &merkle.Receipt{
		Start: start[:],
		Entries: []*merkle.ReceiptEntry{
			{Hash: nil, Right: true},
		},
	}
	expected := sha256.Sum256([]byte("dummy"))
	ok := VerifyProof(receipt, expected[:])
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Receipt with nil Entry.Hash should fail")
}

func TestVerifyProofWithMismatchedLengths(t *testing.T) {
	fmt.Println("\n[TEST] TestVerifyProofWithMismatchedLengths")
	start := sha256.Sum256([]byte("leaf"))
	receipt := &merkle.Receipt{
		Start: start[:],
		Entries: []*merkle.ReceiptEntry{
			{Hash: []byte("short"), Right: true},
		},
	}
	expected := sha256.Sum256([]byte("dummy"))
	ok := VerifyProof(receipt, expected[:])
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Entry hash of invalid length should fail")
}

func TestVerifyProofEmptyEntries(t *testing.T) {
	fmt.Println("\n[TEST] TestVerifyProofEmptyEntries")
	start := sha256.Sum256([]byte("leaf"))
	receipt := &merkle.Receipt{
		Start:   start[:],
		Entries: []*merkle.ReceiptEntry{},
	}
	expected := sha256.Sum256(start[:])
	ok := VerifyProof(receipt, expected[:])
	fmt.Printf("    Result: %v (expected: false)\n", ok)
	require.False(t, ok, "Empty entries list should fail")
}

// buildTestReceipt constructs a trivial Merkle receipt for testing.
func buildTestReceipt(leaf []byte, entryHash []byte, right bool) *merkle.Receipt {
	return &merkle.Receipt{
		Start: leaf,
		Entries: []*merkle.ReceiptEntry{
			{Hash: entryHash, Right: right},
		},
	}
}

// calculateExpectedRoot calculates the expected Merkle root for a single-entry receipt.
func calculateExpectedRoot(start []byte, entryHash []byte, right bool) []byte {
	if right {
		return doSha(append(start, entryHash...))
	}
	return doSha(append(entryHash, start...))
}
