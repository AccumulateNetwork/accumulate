// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DefaultEndpoint is the default Accumulate API endpoint
const DefaultEndpoint = "https://mainnet.accumulatenetwork.io/v3"

// RegisterAccumulateTools registers all Accumulate-related tools
func (s *Server) RegisterAccumulateTools(endpoint string) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	client := jsonrpc.NewClient(endpoint)

	// Wallet/Key management tools
	s.registerWalletTools()

	// Network/Node information tools
	s.registerNetworkTools(client)

	// Query tools
	s.registerQueryTools(client)

	// Search tools
	s.registerSearchTools(client)

	// Transaction submission tools
	s.registerTransactionTools(client)

	// Transaction building tools
	s.registerTxBuildTools(client)

	// Protocol information resources
	s.registerResources()
}

func (s *Server) registerNetworkTools(client *jsonrpc.Client) {
	s.RegisterTool(
		Tool{
			Name:        "network_status",
			Description: "Get the current status of the Accumulate network including oracle price, globals, and routing info",
			InputSchema: JSONSchema{Type: "object", Properties: map[string]JSONSchema{}},
		},
		makeNetworkStatusHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "node_info",
			Description: "Get information about the connected node",
			InputSchema: JSONSchema{Type: "object", Properties: map[string]JSONSchema{}},
		},
		makeNodeInfoHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "consensus_status",
			Description: "Get the consensus status of a specific partition",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"partition": {Type: "string"},
					"node_id":   {Type: "string"},
				},
			},
		},
		makeConsensusStatusHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "metrics",
			Description: "Get metrics from the node",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"partition": {Type: "string"},
				},
			},
		},
		makeMetricsHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "find_service",
			Description: "Find services on the network by type",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"service": {Type: "string"},
					"known":   {Type: "boolean"},
				},
			},
		},
		makeFindServiceHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "list_snapshots",
			Description: "List available snapshots for a partition",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"partition": {Type: "string"},
				},
			},
		},
		makeListSnapshotsHandler(client),
	)
}

func (s *Server) registerQueryTools(client *jsonrpc.Client) {
	s.RegisterTool(
		Tool{
			Name:        "query_account",
			Description: "Query an Accumulate account by URL to get its current state",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url": {Type: "string"},
				},
				Required: []string{"url"},
			},
		},
		makeQueryAccountHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "query_transaction",
			Description: "Query a transaction by its hash/txid to get status and details",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"txid": {Type: "string"},
				},
				Required: []string{"txid"},
			},
		},
		makeQueryTransactionHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "query_directory",
			Description: "List entries in an Accumulate identity/directory account",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":   {Type: "string"},
					"start": {Type: "number"},
					"count": {Type: "number"},
				},
				Required: []string{"url"},
			},
		},
		makeQueryDirectoryHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "query_chain",
			Description: "Query a specific chain on an account (main, pending, signature, etc)",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":   {Type: "string"},
					"chain": {Type: "string"},
					"start": {Type: "number"},
					"count": {Type: "number"},
					"index": {Type: "number"},
				},
				Required: []string{"url", "chain"},
			},
		},
		makeQueryChainHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "query_data",
			Description: "Query data entries from a data account",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":   {Type: "string"},
					"start": {Type: "number"},
					"count": {Type: "number"},
					"index": {Type: "number"},
				},
				Required: []string{"url"},
			},
		},
		makeQueryDataHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "query_block",
			Description: "Query block information by minor or major block number",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":         {Type: "string"},
					"minor":       {Type: "number"},
					"major":       {Type: "number"},
					"minor_start": {Type: "number"},
					"minor_count": {Type: "number"},
					"major_start": {Type: "number"},
					"major_count": {Type: "number"},
				},
				Required: []string{"url"},
			},
		},
		makeQueryBlockHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "query_pending",
			Description: "Query pending transactions for an account",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":   {Type: "string"},
					"start": {Type: "number"},
					"count": {Type: "number"},
				},
				Required: []string{"url"},
			},
		},
		makeQueryPendingHandler(client),
	)
}

func (s *Server) registerSearchTools(client *jsonrpc.Client) {
	s.RegisterTool(
		Tool{
			Name:        "search_public_key",
			Description: "Search for accounts associated with a public key",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"public_key": {Type: "string"},
					"type":       {Type: "string"},
				},
				Required: []string{"public_key"},
			},
		},
		makeSearchPublicKeyHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "search_public_key_hash",
			Description: "Search for accounts associated with a public key hash",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"public_key_hash": {Type: "string"},
				},
				Required: []string{"public_key_hash"},
			},
		},
		makeSearchPublicKeyHashHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "search_delegate",
			Description: "Search for delegation relationships",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":      {Type: "string"},
					"delegate": {Type: "string"},
				},
				Required: []string{"url", "delegate"},
			},
		},
		makeSearchDelegateHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "search_anchor",
			Description: "Search for an anchor by hash",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"url":    {Type: "string"},
					"anchor": {Type: "string"},
				},
				Required: []string{"url", "anchor"},
			},
		},
		makeSearchAnchorHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "search_message_hash",
			Description: "Search for a message by its hash",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"hash": {Type: "string"},
				},
				Required: []string{"hash"},
			},
		},
		makeSearchMessageHashHandler(client),
	)
}

func (s *Server) registerTransactionTools(client *jsonrpc.Client) {
	s.RegisterTool(
		Tool{
			Name:        "submit",
			Description: "Submit a signed transaction envelope to the network",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"envelope": {Type: "string"},
				},
				Required: []string{"envelope"},
			},
		},
		makeSubmitHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "validate",
			Description: "Validate a transaction envelope without submitting it",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"envelope": {Type: "string"},
				},
				Required: []string{"envelope"},
			},
		},
		makeValidateHandler(client),
	)

	s.RegisterTool(
		Tool{
			Name:        "faucet",
			Description: "Request test tokens from the faucet (testnet only)",
			InputSchema: JSONSchema{
				Type: "object",
				Properties: map[string]JSONSchema{
					"account": {Type: "string"},
				},
				Required: []string{"account"},
			},
		},
		makeFaucetHandler(client),
	)
}

// Network tool handlers

func makeNetworkStatusHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("network status failed: %w", err)
		}

		return marshalResult(status)
	}
}

func makeNodeInfoHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		info, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("node info failed: %w", err)
		}

		return marshalResult(info)
	}
}

func makeConsensusStatusHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		opts := api.ConsensusStatusOptions{}
		if p, ok := args["partition"].(string); ok && p != "" {
			opts.Partition = p
		}

		// Get nodeId from args or fetch from node-info
		if nodeId, ok := args["node_id"].(string); ok && nodeId != "" {
			opts.NodeID = nodeId
		} else {
			// Fetch node info to get the peer ID
			nodeInfo, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
			if err != nil {
				return ToolCallResult{}, fmt.Errorf("get node info: %w", err)
			}
			opts.NodeID = nodeInfo.PeerID.String()
		}

		status, err := client.ConsensusStatus(ctx, opts)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("consensus status failed: %w", err)
		}

		return marshalResult(status)
	}
}

func makeMetricsHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		opts := api.MetricsOptions{}
		if p, ok := args["partition"].(string); ok && p != "" {
			opts.Partition = p
		}

		metrics, err := client.Metrics(ctx, opts)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("metrics failed: %w", err)
		}

		return marshalResult(metrics)
	}
}

func makeFindServiceHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		opts := api.FindServiceOptions{}
		if s, ok := args["service"].(string); ok && s != "" {
			sa, err := api.ParseServiceAddress(s)
			if err != nil {
				return ToolCallResult{}, fmt.Errorf("invalid service address: %w", err)
			}
			opts.Service = sa
		}
		if k, ok := args["known"].(bool); ok {
			opts.Known = k
		}

		results, err := client.FindService(ctx, opts)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("find service failed: %w", err)
		}

		return marshalResult(results)
	}
}

func makeListSnapshotsHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		opts := api.ListSnapshotsOptions{}
		if p, ok := args["partition"].(string); ok && p != "" {
			opts.Partition = p
		}

		snapshots, err := client.ListSnapshots(ctx, opts)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("list snapshots failed: %w", err)
		}

		return marshalResult(snapshots)
	}
}

// Query tool handlers

func makeQueryAccountHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, nil)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeQueryTransactionHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		txid, ok := args["txid"].(string)
		if !ok || txid == "" {
			return ToolCallResult{}, fmt.Errorf("txid is required")
		}

		u, err := url.Parse(txid)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid txid: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, nil)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeQueryDirectoryHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		query := &api.DirectoryQuery{
			Range: &api.RangeOptions{},
		}

		if start, ok := args["start"].(float64); ok {
			query.Range.Start = uint64(start)
		}

		count := uint64(20)
		if c, ok := args["count"].(float64); ok {
			count = uint64(c)
		}
		query.Range.Count = &count

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("directory query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeQueryChainHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		chainName, ok := args["chain"].(string)
		if !ok || chainName == "" {
			return ToolCallResult{}, fmt.Errorf("chain is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		query := &api.ChainQuery{
			Name: chainName,
		}

		if idx, ok := args["index"].(float64); ok {
			i := uint64(idx)
			query.Index = &i
		} else if args["start"] != nil || args["count"] != nil {
			query.Range = &api.RangeOptions{}
			if start, ok := args["start"].(float64); ok {
				query.Range.Start = uint64(start)
			}
			count := uint64(20)
			if c, ok := args["count"].(float64); ok {
				count = uint64(c)
			}
			query.Range.Count = &count
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("chain query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeQueryDataHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		query := &api.DataQuery{}

		if idx, ok := args["index"].(float64); ok {
			i := uint64(idx)
			query.Index = &i
		} else if args["start"] != nil || args["count"] != nil {
			query.Range = &api.RangeOptions{}
			if start, ok := args["start"].(float64); ok {
				query.Range.Start = uint64(start)
			}
			count := uint64(20)
			if c, ok := args["count"].(float64); ok {
				count = uint64(c)
			}
			query.Range.Count = &count
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("data query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeQueryBlockHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		query := &api.BlockQuery{}

		if minor, ok := args["minor"].(float64); ok {
			m := uint64(minor)
			query.Minor = &m
		}
		if major, ok := args["major"].(float64); ok {
			m := uint64(major)
			query.Major = &m
		}
		if args["minor_start"] != nil || args["minor_count"] != nil {
			query.MinorRange = &api.RangeOptions{}
			if start, ok := args["minor_start"].(float64); ok {
				query.MinorRange.Start = uint64(start)
			}
			if count, ok := args["minor_count"].(float64); ok {
				c := uint64(count)
				query.MinorRange.Count = &c
			}
		}
		if args["major_start"] != nil || args["major_count"] != nil {
			query.MajorRange = &api.RangeOptions{}
			if start, ok := args["major_start"].(float64); ok {
				query.MajorRange.Start = uint64(start)
			}
			if count, ok := args["major_count"].(float64); ok {
				c := uint64(count)
				query.MajorRange.Count = &c
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("block query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeQueryPendingHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		query := &api.PendingQuery{
			Range: &api.RangeOptions{},
		}

		if start, ok := args["start"].(float64); ok {
			query.Range.Start = uint64(start)
		}

		count := uint64(20)
		if c, ok := args["count"].(float64); ok {
			count = uint64(c)
		}
		query.Range.Count = &count

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("pending query failed: %w", err)
		}

		return marshalResult(resp)
	}
}

// Search tool handlers

func makeSearchPublicKeyHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		pubKeyStr, ok := args["public_key"].(string)
		if !ok || pubKeyStr == "" {
			return ToolCallResult{}, fmt.Errorf("public_key is required")
		}

		pubKey, err := hex.DecodeString(pubKeyStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid public key hex: %w", err)
		}

		query := &api.PublicKeySearchQuery{
			PublicKey: pubKey,
		}

		if t, ok := args["type"].(string); ok && t != "" {
			sigType, ok := protocol.SignatureTypeByName(t)
			if !ok {
				return ToolCallResult{}, fmt.Errorf("invalid signature type: %s", t)
			}
			query.Type = sigType
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, nil, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("public key search failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeSearchPublicKeyHashHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		hashStr, ok := args["public_key_hash"].(string)
		if !ok || hashStr == "" {
			return ToolCallResult{}, fmt.Errorf("public_key_hash is required")
		}

		hash, err := hex.DecodeString(hashStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid hash hex: %w", err)
		}

		query := &api.PublicKeyHashSearchQuery{
			PublicKeyHash: hash,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, nil, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("public key hash search failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeSearchDelegateHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		delegateStr, ok := args["delegate"].(string)
		if !ok || delegateStr == "" {
			return ToolCallResult{}, fmt.Errorf("delegate is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		delegate, err := url.Parse(delegateStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid delegate URL: %w", err)
		}

		query := &api.DelegateSearchQuery{
			Delegate: delegate,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("delegate search failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeSearchAnchorHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		urlStr, ok := args["url"].(string)
		if !ok || urlStr == "" {
			return ToolCallResult{}, fmt.Errorf("url is required")
		}

		anchorStr, ok := args["anchor"].(string)
		if !ok || anchorStr == "" {
			return ToolCallResult{}, fmt.Errorf("anchor is required")
		}

		u, err := url.Parse(urlStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid URL: %w", err)
		}

		anchor, err := hex.DecodeString(anchorStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid anchor hex: %w", err)
		}

		query := &api.AnchorSearchQuery{
			Anchor: anchor,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, u, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("anchor search failed: %w", err)
		}

		return marshalResult(resp)
	}
}

func makeSearchMessageHashHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		hashStr, ok := args["hash"].(string)
		if !ok || hashStr == "" {
			return ToolCallResult{}, fmt.Errorf("hash is required")
		}

		hash, err := hex.DecodeString(hashStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid hash hex: %w", err)
		}

		var msgHash [32]byte
		copy(msgHash[:], hash)

		query := &api.MessageHashSearchQuery{
			Hash: msgHash,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.Query(ctx, nil, query)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("message hash search failed: %w", err)
		}

		return marshalResult(resp)
	}
}

// Transaction tool handlers

func makeSubmitHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		envelopeStr, ok := args["envelope"].(string)
		if !ok || envelopeStr == "" {
			return ToolCallResult{}, fmt.Errorf("envelope is required")
		}

		envelopeBytes, err := hex.DecodeString(envelopeStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid envelope hex: %w", err)
		}

		var envelope messaging.Envelope
		if err := envelope.UnmarshalBinary(envelopeBytes); err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid envelope: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		submissions, err := client.Submit(ctx, &envelope, api.SubmitOptions{})
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("submit failed: %w", err)
		}

		return marshalResult(submissions)
	}
}

func makeValidateHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		envelopeStr, ok := args["envelope"].(string)
		if !ok || envelopeStr == "" {
			return ToolCallResult{}, fmt.Errorf("envelope is required")
		}

		envelopeBytes, err := hex.DecodeString(envelopeStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid envelope hex: %w", err)
		}

		var envelope messaging.Envelope
		if err := envelope.UnmarshalBinary(envelopeBytes); err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid envelope: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		submissions, err := client.Validate(ctx, &envelope, api.ValidateOptions{})
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("validate failed: %w", err)
		}

		return marshalResult(submissions)
	}
}

func makeFaucetHandler(client *jsonrpc.Client) ToolHandler {
	return func(args map[string]any) (ToolCallResult, error) {
		accountStr, ok := args["account"].(string)
		if !ok || accountStr == "" {
			return ToolCallResult{}, fmt.Errorf("account is required")
		}

		account, err := url.Parse(accountStr)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid account URL: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		submission, err := client.Faucet(ctx, account, api.FaucetOptions{})
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("faucet failed: %w", err)
		}

		return marshalResult(submission)
	}
}

// Helper function to marshal results to JSON
func marshalResult(v any) (ToolCallResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ToolCallResult{}, fmt.Errorf("marshal response: %w", err)
	}
	return NewTextResult(string(data)), nil
}
