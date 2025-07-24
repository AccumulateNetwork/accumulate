package liteclient

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func (c *LiteClient) IsProofStale(account string, currentHeight int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	va, found := c.cache[account]
	if !found {
		return true // No proof, so it's stale
	}
	return va.Height != currentHeight
}

func (c *LiteClient) StoreProof(account string, receipt *merkle.Receipt, height int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[account] = VerifiedAccount{
		Url:     account,
		Receipt: receipt,
		Height:  height,
	}
	fmt.Printf("Stored proof for account %s at height %d\n", account, height)
}

// GetStoredProof retrieves a stored proof from the cache
func (c *LiteClient) GetStoredProof(accountUrl string) (VerifiedAccount, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	proof, exists := c.cache[accountUrl]
	return proof, exists
}
