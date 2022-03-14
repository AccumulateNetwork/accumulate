package client

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

//go:generate go run ../../tools/cmd/gen-sdk --package client --out api_v2_sdk_gen.go ../api/v2/methods.yml

type Client struct {
	jsonrpc2.Client
	serverV2 string
}

// New creates new API client with default config
func New(server string) (*Client, error) {
	c := new(Client)
	c.Timeout = 15 * time.Second

	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("invalid server: %v", err)
	}

	switch u.Path {
	case "":
		c.serverV2 = server + "/v2"
	case "/":
		c.serverV2 = server + "v2"
	case "/v2":
		c.serverV2 = server
	default:
		if !strings.HasSuffix(u.Path, "/v2") {
			return nil, fmt.Errorf("invalid server: URL path must be empty or end with /v2")
		}
	}

	return c, nil
}

// RequestAPIv2 makes a JSON RPC request to the Accumulate API v2.
func (c *Client) RequestAPIv2(ctx context.Context, method string, params, result interface{}) error {
	if c.DebugRequest {
		fmt.Println("accumulated:", c.serverV2)
	}

	return c.Client.Request(ctx, c.serverV2, method, params, result)
}
