package liteclient

import (
	"context"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (c *LiteClient) FetchProof(ctx context.Context, account string) (*VerifiedAccount, error) {
	// Parse account URL
	u, err := accurl.Parse(account)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Build and send query request
	req := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: u}}
	var resp client.ChainQueryResponse

	// Fetch proof receipt via v2 API
	err = c.v2.RequestAPIv2(ctx, "query", req, &resp)
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

func (c *LiteClient) VerifyProof(receipt *merkle.Receipt, root []byte) bool {
	// TODO: Verify that the receipt matches the known BPT root hash
	return false
}

func (c *LiteClient) ValidateAndCacheProof(ctx context.Context, account string, knownRoot []byte) error {
	// TODO: Fetch, verify, and cache the account proof if valid
	return nil
}
