package client

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/url"
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

// RequestV1 Deprecated - Request makes request to API server
func (c *APIClient) RequestV1(ctx context.Context,
	method string, params, result interface{}) error {

	if c.DebugRequest {
		fmt.Println("accumulated:", c.Server)
	}
	return c.Client.Request(ctx, c.Server, method, params, result)
}

// RequestV2 makes request to API server (version 2)
func (c *APIClient) RequestV2(ctx context.Context,
	method string, params, result interface{}) error {
	serverUrl, err := url.Parse(c.Server)
	if err != nil {
		return err
	}

	serverUrl.Path = path.Join(serverUrl.Path, "..", "v2")
	server := serverUrl.String()

	if c.DebugRequest {
		fmt.Println("accumulated:", server)
	}
	return c.Client.Request(ctx, server, method, params, result)
}
