// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// cometRPCClient is a minimal CometBFT JSON-RPC client that replaces
// rpchttp.HTTP for the init commands. It speaks to CometBFT's HTTP RPC
// endpoints (/status, /genesis_chunked) using simple HTTP GET requests.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// cometRPCClient is a lightweight HTTP client for CometBFT's RPC API.
type cometRPCClient struct {
	baseURL string
	client  *http.Client
}

// newCometRPCClient creates a new CometBFT RPC client.
// addr should be like "tcp://host:port" or "http://host:port".
func newCometRPCClient(addr string) (*cometRPCClient, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse RPC address: %w", err)
	}
	// Convert tcp:// to http://
	switch u.Scheme {
	case "tcp", "":
		u.Scheme = "http"
	case "http", "https":
		// ok
	default:
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	return &cometRPCClient{
		baseURL: u.String(),
		client:  &http.Client{},
	}, nil
}

// jsonRPCResponse wraps a CometBFT JSON-RPC response.
type jsonRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	} `json:"error"`
}

func (c *cometRPCClient) get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	u, err := url.Parse(c.baseURL + "/" + path)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpc jsonRPCResponse
	if err := json.Unmarshal(body, &rpc); err != nil {
		return nil, fmt.Errorf("decode RPC response: %w", err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	return rpc.Result, nil
}

// cometStatusResult contains the fields from CometBFT's /status response
// that are actually used by the init commands.
type cometStatusResult struct {
	NodeInfo struct {
		DefaultNodeID string `json:"default_node_id"`
	} `json:"node_info"`
}

// Status calls CometBFT's /status endpoint.
func (c *cometRPCClient) Status(ctx context.Context) (*cometStatusResult, error) {
	raw, err := c.get(ctx, "status", nil)
	if err != nil {
		return nil, err
	}
	var result cometStatusResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &result, nil
}

// cometGenesisChunk contains the fields from CometBFT's /genesis_chunked response.
type cometGenesisChunk struct {
	Chunk       int    `json:"chunk"`
	TotalChunks int    `json:"total"`
	Data        string `json:"data"`
}

// GenesisChunked calls CometBFT's /genesis_chunked endpoint.
func (c *cometRPCClient) GenesisChunked(ctx context.Context, chunk uint) (*cometGenesisChunk, error) {
	raw, err := c.get(ctx, "genesis_chunked", map[string]string{
		"chunk": strconv.FormatUint(uint64(chunk), 10),
	})
	if err != nil {
		return nil, err
	}
	var result cometGenesisChunk
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode genesis chunk: %w", err)
	}
	return &result, nil
}
