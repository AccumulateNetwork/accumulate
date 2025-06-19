package liteclient

import (
	"context"
	"sync"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

type LiteClient struct {
	v2    *client.Client
	cache map[string]VerifiedAccount
	mu    sync.RWMutex
}

// NewLiteClient creates a new LiteClient for Phase 1 (account proof creation).
func NewLiteClient(server string) (*LiteClient, error) {
	cli, err := client.New(server)
	if err != nil {
		return nil, err
	}
	return &LiteClient{
		v2:    cli,
		cache: make(map[string]VerifiedAccount),
	}, nil
}

func (c *LiteClient) QueryAccountWithReceipt(ctx context.Context, account string) (*client.ChainQueryResponse, *client.GeneralReceipt, error) {
	// TODO: Query account state + receipt from full node
	return nil, nil, nil
}
