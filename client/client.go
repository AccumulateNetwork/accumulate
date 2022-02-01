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
	ServerDefault = "http://localhost:34000/v2"
)

// NewAPIClient creates new API client with default config
func NewAPIClient() *APIClient {
	c := &APIClient{Server: ServerDefault}
	c.Timeout = 15 * time.Second
	return c
}

// Send a transaction request to the API server.
// This is the transaction's final departure into the JSON-RPC-2
// black box, from which it proceeds to a remote host inside the
// Accumulate network.
//
// Execution resumes, on the server side, at
// /internal/api/v2/jrpc_execute.go#execute with a protocol
// selected from
// /internal/api/v2/api_gen.go
// according to the provided action.
//
// The JSON-RPC-2 response is returned verbatim.
func (c *APIClient) Request(ctx context.Context,
	action string, params, result interface{}) error {

	if c.DebugRequest {
		fmt.Println("accumulated:", c.Server)
	}
	return c.Client.Request(ctx, c.Server, action, params, result)
}
