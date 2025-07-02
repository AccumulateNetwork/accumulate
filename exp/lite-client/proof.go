package liteclient

import (
	"bytes"
	"context"
	"crypto/sha256"
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

	// GeneralQuery object is what is used to request data from an Accumulate Node
	// It contains the account URL
	req := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: u}}
	// resp is a placeholder for a variable that will be filled with the response
	// from the Accumulate node
	var resp client.ChainQueryResponse

	// Fetch proof receipt via v2 API
	// ctx is the context for cancellation and timeouts
	// "query" is the API endpoint to call
	// req is the request object
	// resp is a pointer to the object that must be populated
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

// doSha computes the SHA-256 hash of the input.
func doSha(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
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
