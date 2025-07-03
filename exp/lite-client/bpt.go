package liteclient

import (
	"context"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// FetchBPTRootHash fetches the BPT root hash from the node status endpoint.
// partition should be "dn" or "bvn0.acme" (without acc:// prefix)
func FetchBPTRootHash(ctx context.Context, cl *client.Client, partition string) ([]byte, error) {
	status, err := cl.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query node status: %w", err)
	}
	if partition == "dn" {
		return status.DnBptHash[:], nil
	} else if partition == "bvn0.acme" {
		return status.BvnBptHash[:], nil
	}
	return nil, fmt.Errorf("unsupported partition: %s", partition)
}
