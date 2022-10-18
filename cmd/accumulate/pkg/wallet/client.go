// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package wallet

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

//go:generate go run ../../../../tools/cmd/gen-sdk --package wallet --api-path gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api --out wallet_api_sdk_gen.go ../../walletd/api/methods.yml

type Client struct {
	jsonrpc2.Client
	serverRpc string
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
		c.serverRpc = server + "/rpc"
	case "/":
		c.serverRpc = server + "rpc"
	case "/rpc":
		c.serverRpc = server
	default:
		if !strings.HasSuffix(u.Path, "/rpc") {
			return nil, fmt.Errorf("invalid server: URL path must be empty or end with /rpc")
		}
	}

	return c, nil
}

// RequestAPIv1 makes a JSON RPC request
func (c *Client) RequestAPIv2(ctx context.Context, method string, params, result interface{}) error {
	if c.DebugRequest {
		fmt.Println("wallet:", c.serverRpc) //nolint:noprint
	}

	return c.Client.Request(ctx, c.serverRpc, method, params, result)
}
