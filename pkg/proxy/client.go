package proxy

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

//go:generate go run ../../tools/cmd/gen-types --package proxy --out types_gen.go types.yml

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

// RequestAPIv2 makes a JSON RPC request
func (c *Client) RequestAPIv2(ctx context.Context, method string, params, result interface{}) error {
	if c.DebugRequest {
		fmt.Println("accuproxy:", c.serverRpc) //nolint:noprint
	}

	return c.Client.Request(ctx, c.serverRpc, method, params, result)
}

// GetNetworkConfig get full network description
func (c *Client) GetNetworkConfig(ctx context.Context, req *NetworkConfigRequest) (*NetworkConfigResponse, error) {
	var resp NetworkConfigResponse

	err := c.RequestAPIv2(ctx, "network", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetSubnetList get list of subnets on a given network
func (c *Client) GetSubnetList(ctx context.Context, req *SubnetListRequest) (*SubnetListResponse, error) {
	var resp SubnetListResponse

	err := c.RequestAPIv2(ctx, "subnets", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetSeedList get list of seed ip's for a particular subnet
func (c *Client) GetSeedList(ctx context.Context, req *SeedListRequest) (*SeedListResponse, error) {
	var resp SeedListResponse

	err := c.RequestAPIv2(ctx, "seed-list", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetIp get list of seed ip's.
func (c *Client) GetSeedCount(ctx context.Context, req *SeedCountRequest) (*SeedCountResponse, error) {
	var resp SeedCountResponse

	err := c.RequestAPIv2(ctx, "seed-count", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
