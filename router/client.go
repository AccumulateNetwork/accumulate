package router

import (
	"context"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

// Client makes RPC requests to Public API
type Client struct {
	endpoint string
	jsonrpc2.Client
}

// NewClient returns a pointer to a Client
func NewClient(endpoint string) *Client {
	c := &Client{endpoint: endpoint}
	c.Timeout = 15 * time.Second
	return c
}

// Request makes a request to Public API
func (c *Client) Request(ctx context.Context,
	method string, params, result interface{}) error {
	return c.Client.Request(ctx, c.endpoint, method, params, result)
}
