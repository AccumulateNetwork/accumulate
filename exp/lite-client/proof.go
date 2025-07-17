package liteclient

import (
	"bytes"
	"context"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type VerifiedAccount struct {
	Url     string
	Receipt *merkle.Receipt
	Height  int64
}

func FetchProof(api *client.Client, ctx context.Context, account string) (*VerifiedAccount, error) {
	// Parse account URL
	u, err := accurl.Parse(account)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	req := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: u}}

	var resp client.ChainQueryResponse

	err = api.RequestAPIv2(ctx, "query", req, &resp)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	if resp.Receipt == nil {
		return nil, fmt.Errorf("no receipt returned for account: %s", account)
	}
	gr := resp.Receipt

	return &VerifiedAccount{
		Url:     account,
		Receipt: &gr.Proof,
		Height:  int64(gr.MajorBlock),
	}, nil
}

func VerifyProof(receipt *merkle.Receipt, expectedRoot []byte) bool {
	if receipt == nil || receipt.Anchor == nil {
		return false
	}
	if !bytes.Equal(receipt.Anchor, expectedRoot) {
		return false
	}
	return receipt.Validate(&merkle.ValidateOptions{
		Relaxed: false,
	})
}

// ValidateAndCacheProof fetches, verifies, and caches a proof for the given account using the provided LiteClient.
func ValidateAndCacheProof(client *LiteClient, ctx context.Context, account string, knownRoot []byte) error {
	// Step 1: Fetch proof for the account from the node
	verified, err := FetchProof(client.v2, ctx, account)
	if err != nil {
		return fmt.Errorf("failed to fetch proof: %w", err)
	}

	// Step 2: Verify the proof against the known root
	if verified.Receipt == nil {
		return fmt.Errorf("no receipt in fetched proof for account: %s", account)
	}
	if !VerifyProof(verified.Receipt, knownRoot) {
		return fmt.Errorf("proof verification failed for account: %s", account)
	}

	// Step 3: Cache/store the verified proof for future use
	client.StoreProof(account, verified.Receipt, verified.Height)

	return nil
}
