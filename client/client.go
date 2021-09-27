package client

import (
	"context"
	"fmt"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

type APIClient struct {
	Server string
	jsonrpc2.Client
}

const (
	ServerDefault = "http://localhost:34000/v1"
)

// NewAPIClient creates new API client with default config
func NewAPIClient() *APIClient {
	c := &APIClient{Server: ServerDefault}
	c.Timeout = 15 * time.Second
	return c
}

// Request makes request to API server
func (c *APIClient) Request(ctx context.Context,
	method string, params, result interface{}) error {

	if c.DebugRequest {
		fmt.Println("accumulated:", c.Server)
	}
	return c.Client.Request(ctx, c.Server, method, params, result)
}
