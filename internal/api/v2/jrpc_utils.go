package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

type Client struct {
	jsonrpc2.Client
	serverV2 string
}

func New(server string) (*Client, error) {
	switch server {
	case "local":
		server = "http://127.0.1.1:26660"
	case "testnet":
		server = "https://testnet.accumulatenetwork.io"
	case "beta":
		server = "https://beta.testnet.accumulatenetwork.io"
	case "canary":
		server = "https://canary.testnet.accumulatenetwork.io"
	case "", "mainnet":
		server = "https://mainnet.accumulatenetwork.io"
	}

	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("invalid server: %v", err)
	}

	c := new(Client)
	c.Timeout = 15 * time.Second
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
		fmt.Println("accumulated:", c.serverV2) //nolint:noprint
	}

	return c.Client.Request(ctx, c.serverV2, method, params, result)
}
