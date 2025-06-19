package liteclient

import client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"

func (c *LiteClient) IsProofStale(account string, currentHeight int64) bool {
	// TODO: Compare current height vs. cached proof height
	return true
}

func (c *LiteClient) StoreProof(account string, receipt *client.GeneralReceipt, height int64) {
	// TODO: Store verified proof in cache
}
