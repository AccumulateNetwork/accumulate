package server

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

// Tier 1: Core Query Tools

func (s *Server) queryChain(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Build query parameters
	params := make(map[string]interface{})
	if name, ok := args["chain_name"].(string); ok {
		params["name"] = name
	}
	if index, ok := args["index"]; ok {
		params["index"] = index
	}
	if entryHash, ok := args["entry_hash"].(string); ok {
		params["entry"] = entryHash
	}
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}
	if expand, ok := args["expand"].(bool); ok {
		params["expand"] = expand
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryChain(ctx, url, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query chain: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) queryData(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if index, ok := args["index"]; ok {
		params["index"] = index
	}
	if entryHash, ok := args["entry_hash"].(string); ok {
		params["entry"] = entryHash
	}
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryData(ctx, url, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query data: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) queryDirectory(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}
	if expand, ok := args["expand"].(bool); ok {
		params["expand"] = expand
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryDirectory(ctx, url, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query directory: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) queryPending(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryPending(ctx, url, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) queryMinorBlock(args map[string]interface{}) (map[string]interface{}, error) {
	partition, ok := args["partition"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: partition")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	var index interface{}
	if idx, ok := args["index"]; ok {
		index = idx
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryMinorBlock(ctx, partition, index)
	if err != nil {
		return nil, fmt.Errorf("failed to query minor block: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) queryMajorBlock(args map[string]interface{}) (map[string]interface{}, error) {
	partition, ok := args["partition"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: partition")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	var index interface{}
	if idx, ok := args["index"]; ok {
		index = idx
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryMajorBlock(ctx, partition, index)
	if err != nil {
		return nil, fmt.Errorf("failed to query major block: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

// Tier 3: Network Status Tools

func (s *Server) nodeInfo(args map[string]interface{}) (map[string]interface{}, error) {
	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	params := make(map[string]interface{})
	record, err := c.NodeInfo(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) networkStatus(args map[string]interface{}) (map[string]interface{}, error) {
	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	params := make(map[string]interface{})
	record, err := c.NetworkStatus(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) consensusStatus(args map[string]interface{}) (map[string]interface{}, error) {
	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if p, ok := args["partition"].(string); ok {
		params["partition"] = p
	}
	if nodeID, ok := args["nodeID"].(string); ok {
		params["nodeID"] = nodeID
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.ConsensusStatus(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get consensus status: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) metrics(args map[string]interface{}) (map[string]interface{}, error) {
	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if p, ok := args["partition"].(string); ok {
		params["partition"] = p
	}
	if metric, ok := args["metric"].(string); ok {
		params["metric"] = metric
	}
	if span, ok := args["span"].(string); ok {
		params["span"] = span
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.Metrics(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) faucet(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "testnet" // Faucet only works on testnet
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if t, ok := args["token"].(string); ok {
		params["token"] = t
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.Faucet(ctx, url, params)
	if err != nil {
		return nil, fmt.Errorf("failed to request from faucet: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

// Tier 4: Advanced Search Tools

func (s *Server) searchPublicKey(args map[string]interface{}) (map[string]interface{}, error) {
	publicKey, ok := args["public_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: public_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.SearchPublicKey(ctx, publicKey, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search public key: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) searchPublicKeyHash(args map[string]interface{}) (map[string]interface{}, error) {
	publicKeyHash, ok := args["public_key_hash"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: public_key_hash")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.SearchPublicKeyHash(ctx, publicKeyHash, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search public key hash: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) searchAnchor(args map[string]interface{}) (map[string]interface{}, error) {
	anchor, ok := args["anchor"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: anchor")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	params := make(map[string]interface{})
	if start, ok := args["start"]; ok {
		params["start"] = start
	}
	if count, ok := args["count"]; ok {
		params["count"] = count
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.SearchAnchor(ctx, anchor, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search anchor: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

// Phase 1: KeyBook and KeyPage Query Tools

func (s *Server) queryKeyBook(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryKeyBook(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to query KeyBook: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

func (s *Server) queryKeyPage(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	record, err := c.QueryKeyPage(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to query KeyPage: %w", err)
	}

	// Convert record to map[string]interface{}
	// The MCP protocol wrapping is done by handleCallTool, so just return the raw data
	var result map[string]interface{}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := json.Unmarshal(recordBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	return result, nil
}

// Phase 1.5: ADI Management Tools

func (s *Server) generateKey(args map[string]interface{}) (map[string]interface{}, error) {
	publicKey, privateKey, liteAccountURL, err := client.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	result := map[string]interface{}{
		"publicKey":      publicKey,
		"privateKey":     privateKey,
		"liteAccountURL": liteAccountURL,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) addCredits(args map[string]interface{}) (map[string]interface{}, error) {
	recipient, ok := args["recipient"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: recipient")
	}

	payer, ok := args["payer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: payer")
	}

	amountStr, ok := args["amount"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: amount")
	}

	privateKey, ok := args["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Parse amount
	var amount int64
	_, err := fmt.Sscanf(amountStr, "%d", &amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.AddCredits(ctx, recipient, payer, amount, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to add credits: %w", err)
	}

	result := map[string]interface{}{
		"txHash":    fmt.Sprintf("%x", txHash),
		"recipient": recipient,
		"payer":     payer,
		"amount":    amount,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) createADI(args map[string]interface{}) (map[string]interface{}, error) {
	adiURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	publicKey, ok := args["public_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: public_key")
	}

	sponsor, ok := args["sponsor"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor")
	}

	sponsorPrivateKey, ok := args["sponsor_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.CreateIdentity(ctx, adiURL, publicKey, sponsor, sponsorPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create ADI: %w", err)
	}

	result := map[string]interface{}{
		"txHash":     fmt.Sprintf("%x", txHash),
		"adiURL":     adiURL,
		"keyBookURL": adiURL + "/book",
		"keyPageURL": adiURL + "/book/1",
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

// Phase 2: Data & Token Account Operations

func (s *Server) createDataAccount(args map[string]interface{}) (map[string]interface{}, error) {
	accountURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	sponsor, ok := args["sponsor"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor")
	}

	sponsorPrivateKey, ok := args["sponsor_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Parse optional authorities
	var authorities []string
	if auths, ok := args["authorities"].([]interface{}); ok {
		authorities = make([]string, len(auths))
		for i, auth := range auths {
			if authStr, ok := auth.(string); ok {
				authorities[i] = authStr
			}
		}
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.CreateDataAccount(ctx, accountURL, sponsor, sponsorPrivateKey, authorities)
	if err != nil {
		return nil, fmt.Errorf("failed to create data account: %w", err)
	}

	result := map[string]interface{}{
		"txHash":     fmt.Sprintf("%x", txHash),
		"accountURL": accountURL,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) writeData(args map[string]interface{}) (map[string]interface{}, error) {
	accountURL, ok := args["account_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: account_url")
	}

	dataStr, ok := args["data"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: data")
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	encoding := "utf8"
	if enc, ok := args["encoding"].(string); ok {
		encoding = enc
	}

	writeToState := false
	if wts, ok := args["write_to_state"].(bool); ok {
		writeToState = wts
	}

	// Encode data
	data, err := client.EncodeData(dataStr, encoding)
	if err != nil {
		return nil, fmt.Errorf("failed to encode data: %w", err)
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.WriteData(ctx, accountURL, signer, signerPrivateKey, data, writeToState)
	if err != nil {
		return nil, fmt.Errorf("failed to write data: %w", err)
	}

	result := map[string]interface{}{
		"txHash":     fmt.Sprintf("%x", txHash),
		"accountURL": accountURL,
		"dataSize":   len(data),
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) createTokenAccount(args map[string]interface{}) (map[string]interface{}, error) {
	accountURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	sponsor, ok := args["sponsor"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor")
	}

	sponsorPrivateKey, ok := args["sponsor_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	tokenURL := "acc://ACME"
	if t, ok := args["token_url"].(string); ok {
		tokenURL = t
	}

	// Parse optional authorities
	var authorities []string
	if auths, ok := args["authorities"].([]interface{}); ok {
		authorities = make([]string, len(auths))
		for i, auth := range auths {
			if authStr, ok := auth.(string); ok {
				authorities[i] = authStr
			}
		}
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.CreateTokenAccount(ctx, accountURL, tokenURL, sponsor, sponsorPrivateKey, authorities)
	if err != nil {
		return nil, fmt.Errorf("failed to create token account: %w", err)
	}

	result := map[string]interface{}{
		"txHash":     fmt.Sprintf("%x", txHash),
		"accountURL": accountURL,
		"tokenURL":   tokenURL,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

// Phase 3: Key Management Operations

func (s *Server) createKeyPage(args map[string]interface{}) (map[string]interface{}, error) {
	keybookURL, ok := args["keybook_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: keybook_url")
	}

	keysInterface, ok := args["keys"].([]interface{})
	if !ok || len(keysInterface) == 0 {
		return nil, fmt.Errorf("missing or empty required parameter: keys")
	}

	// Convert keys to string slice
	keys := make([]string, len(keysInterface))
	for i, key := range keysInterface {
		keyStr, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("invalid key at index %d: must be string", i)
		}
		keys[i] = keyStr
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.CreateKeyPage(ctx, keybookURL, signer, signerPrivateKey, keys)
	if err != nil {
		return nil, fmt.Errorf("failed to create keypage: %w", err)
	}

	result := map[string]interface{}{
		"txHash":      fmt.Sprintf("%x", txHash),
		"keybookURL":  keybookURL,
		"keysAdded":   len(keys),
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) updateKeyPage(args map[string]interface{}) (map[string]interface{}, error) {
	keypageURL, ok := args["keypage_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: keypage_url")
	}

	operation, ok := args["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: operation")
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Get optional parameters based on operation
	key := ""
	if k, ok := args["key"].(string); ok {
		key = k
	}

	var threshold uint64
	if t, ok := args["threshold"].(float64); ok {
		threshold = uint64(t)
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.UpdateKeyPage(ctx, keypageURL, signer, signerPrivateKey, operation, key, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to update keypage: %w", err)
	}

	result := map[string]interface{}{
		"txHash":      fmt.Sprintf("%x", txHash),
		"keypageURL":  keypageURL,
		"operation":   operation,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) createKeyBook(args map[string]interface{}) (map[string]interface{}, error) {
	keybookURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	publicKeyHash, ok := args["public_key_hash"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: public_key_hash")
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.CreateKeyBook(ctx, keybookURL, signer, signerPrivateKey, publicKeyHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create keybook: %w", err)
	}

	result := map[string]interface{}{
		"txHash":      fmt.Sprintf("%x", txHash),
		"keybookURL":  keybookURL,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) updateAccountAuth(args map[string]interface{}) (map[string]interface{}, error) {
	accountURL, ok := args["account_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: account_url")
	}

	operationsInterface, ok := args["operations"].([]interface{})
	if !ok || len(operationsInterface) == 0 {
		return nil, fmt.Errorf("missing or empty required parameter: operations")
	}

	// Convert operations to proper format
	operations := make([]map[string]interface{}, len(operationsInterface))
	for i, op := range operationsInterface {
		opMap, ok := op.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid operation at index %d: must be object", i)
		}
		operations[i] = opMap
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.UpdateAccountAuth(ctx, accountURL, signer, signerPrivateKey, operations)
	if err != nil {
		return nil, fmt.Errorf("failed to update account auth: %w", err)
	}

	result := map[string]interface{}{
		"txHash":           fmt.Sprintf("%x", txHash),
		"accountURL":       accountURL,
		"operationsCount":  len(operations),
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

// Phase 4: Token Issuance Operations

func (s *Server) createToken(args map[string]interface{}) (map[string]interface{}, error) {
	tokenURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	symbol, ok := args["symbol"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: symbol")
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Default precision
	var precision uint64 = 8
	if p, ok := args["precision"].(float64); ok {
		precision = uint64(p)
	}

	// Optional properties
	properties := make(map[string]interface{})
	if supplyLimitStr, ok := args["supply_limit"].(string); ok {
		var supplyLimit int64
		_, err := fmt.Sscanf(supplyLimitStr, "%d", &supplyLimit)
		if err == nil && supplyLimit > 0 {
			properties["supply_limit"] = supplyLimit
		}
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.CreateToken(ctx, tokenURL, signer, signerPrivateKey, symbol, precision, properties)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	result := map[string]interface{}{
		"txHash":    fmt.Sprintf("%x", txHash),
		"tokenURL":  tokenURL,
		"symbol":    symbol,
		"precision": precision,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) issueTokens(args map[string]interface{}) (map[string]interface{}, error) {
	tokenURL, ok := args["token_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: token_url")
	}

	recipient, ok := args["recipient"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: recipient")
	}

	amountStr, ok := args["amount"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: amount")
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Parse amount
	var amount int64
	_, err := fmt.Sscanf(amountStr, "%d", &amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.IssueTokens(ctx, tokenURL, recipient, signer, signerPrivateKey, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to issue tokens: %w", err)
	}

	result := map[string]interface{}{
		"txHash":    fmt.Sprintf("%x", txHash),
		"tokenURL":  tokenURL,
		"recipient": recipient,
		"amount":    amount,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}

func (s *Server) burnTokens(args map[string]interface{}) (map[string]interface{}, error) {
	accountURL, ok := args["account_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: account_url")
	}

	amountStr, ok := args["amount"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: amount")
	}

	signer, ok := args["signer"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer")
	}

	signerPrivateKey, ok := args["signer_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: signer_private_key")
	}

	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Parse amount
	var amount int64
	_, err := fmt.Sscanf(amountStr, "%d", &amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx := context.Background()
	txHash, err := c.BurnTokens(ctx, accountURL, signer, signerPrivateKey, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to burn tokens: %w", err)
	}

	result := map[string]interface{}{
		"txHash":     fmt.Sprintf("%x", txHash),
		"accountURL": accountURL,
		"amount":     amount,
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format result: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultData),
			},
		},
	}, nil
}
