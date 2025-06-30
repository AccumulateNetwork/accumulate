package liteclient

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func (c *LiteClient) IsProofStale(account string, currentHeight int64) bool {
	va, found := c.cache[account]
	if !found {
		return true // No proof, so it's stale
	}
	return va.Height != currentHeight
}

func (c *LiteClient) StoreProof(account string, receipt *merkle.Receipt, height int64) {
	c.cache[account] = VerifiedAccount{
		Url:     account,
		Receipt: receipt,
		Height:  height,
	}
}
