package liteclient

import (
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

type LiteClient struct {
	v2    *client.Client
	cache map[string]VerifiedAccount
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
