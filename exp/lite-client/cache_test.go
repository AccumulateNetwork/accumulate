package liteclient

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func TestStoreProofAndIsProofStale(t *testing.T) {
	fmt.Println("[Step 1] Initializing LiteClient and test data...")
	client := &LiteClient{
		cache: make(map[string]VerifiedAccount),
	}
	account := "acc://foo/bar"
	receipt := &merkle.Receipt{Start: []byte("leaf")}
	height := int64(42)

	// Initially, cache should be empty
	_, found := client.cache[account]
	fmt.Printf("[Step 2] Cache lookup for %s: found=%v\n", account, found)
	require.False(t, found, "cache should be empty initially")

	// Store a proof
	fmt.Println("[Step 3] Storing proof in cache...")
	client.StoreProof(account, receipt, height)
	va, found := client.cache[account]
	fmt.Printf("[Step 4] Cache lookup after StoreProof: found=%v, Url=%s, Receipt=%v, Height=%d\n", found, va.Url, va.Receipt, va.Height)
	require.True(t, found, "cache should have entry after StoreProof")
	require.Equal(t, account, va.Url)
	require.Equal(t, receipt, va.Receipt)
	require.Equal(t, height, va.Height)

	// IsProofStale: should return false if heights match
	fmt.Println("[Step 5] Testing IsProofStale with matching height...")
	client.cache[account] = VerifiedAccount{Url: account, Receipt: receipt, Height: 100}
	stale := client.IsProofStale(account, 100)
	fmt.Printf("  IsProofStale(account, 100): %v (expected false)\n", stale)
	require.False(t, stale, "IsProofStale should return false if heights match")

	// IsProofStale: should return true if heights differ
	fmt.Println("[Step 6] Testing IsProofStale with different height...")
	stale = client.IsProofStale(account, 200)
	fmt.Printf("  IsProofStale(account, 200): %v (expected true)\n", stale)
	require.True(t, stale, "IsProofStale should return true if heights differ")

	// IsProofStale: should return true if proof is missing
	fmt.Println("[Step 7] Testing IsProofStale with missing proof...")
	delete(client.cache, account)
	stale = client.IsProofStale(account, 100)
	fmt.Printf("  IsProofStale(account, 100) after delete: %v (expected true)\n", stale)
	require.True(t, stale, "IsProofStale should return true if proof missing")
}
