package liteclient

import (
	"context"
	"fmt"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

type LiteClient struct {
	v2    *v2.Client
	v3    api.Querier
	cache map[string]VerifiedAccount
}

func NewLiteClient(server string) (*LiteClient, error) {
	v2Client, err := v2.New(server)
	if err != nil {
		return nil, fmt.Errorf("failed to create v2 client: %w", err)

	}
	// v3 API is served at /v3 endpoint
	v3Client := jsonrpc.NewClient(server + "/v3")
	return &LiteClient{
		v2:    v2Client,
		v3:    v3Client,
		cache: make(map[string]VerifiedAccount),
	}, nil
}

func (c *LiteClient) RetrieveAccountStates(ctx context.Context, accountUrls []string) error {
	err := RetrieveAndValidateProof(ctx, accountUrls, c)
	if err != nil {
		return fmt.Errorf("phase 1 failed: unable to retrieve or validate account proof: %w", err)
	}
	return nil
}

func RetrieveAndValidateProof(ctx context.Context, accountUrls []string, c *LiteClient) error {
	rootHash, err := FetchBPTRootHash(ctx, c.v2, "dn")
	if err != nil {
		rootHash = []byte("placeholder-root-hash")
	}

	for _, url := range accountUrls {

		err := ValidateAndCacheProof(c, ctx, url, rootHash)
		if err != nil {
			return fmt.Errorf("error validating and caching proof for %s: %w", url, err)
		}
	}

	return nil
}
