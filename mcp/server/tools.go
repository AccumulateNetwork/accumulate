package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

// queryAccount queries an Accumulate account with automatic retry for transient failures
func (s *Server) queryAccount(args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	helper := NewClientHelper(s.state)
	network := helper.GetNetwork(args)

	return helper.WithClientRetry(network, nil, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
		record, err := c.QueryAccount(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("failed to query account: %w", err)
		}

		// Convert record to map[string]interface{}
		var result map[string]interface{}
		recordBytes, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal record: %w", err)
		}
		if err := json.Unmarshal(recordBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal record: %w", err)
		}

		return result, nil
	})
}

// queryTransaction queries a transaction by hash with automatic retry for transient failures
func (s *Server) queryTransaction(args map[string]interface{}) (map[string]interface{}, error) {
	txid, ok := args["txid"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: txid")
	}

	helper := NewClientHelper(s.state)
	network := helper.GetNetwork(args)

	return helper.WithClientRetry(network, nil, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
		record, err := c.QueryTransaction(ctx, txid)
		if err != nil {
			return nil, fmt.Errorf("failed to query transaction: %w", err)
		}

		// Convert record to map[string]interface{}
		var result map[string]interface{}
		recordBytes, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal record: %w", err)
		}
		if err := json.Unmarshal(recordBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal record: %w", err)
		}

		return result, nil
	})
}

// createLiteAccount creates a lite account URL from a public key
func (s *Server) createLiteAccount(args map[string]interface{}) (map[string]interface{}, error) {
	pubKey, ok := args["public_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: public_key")
	}

	liteUrl, err := client.CreateLiteAccountURL(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create lite account URL: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("Lite account URL created: %s\n\nThis is a deterministic URL derived from the public key. The account will be automatically created on first use (when it receives tokens).", liteUrl),
			},
		},
	}, nil
}

// sendTokens prepares and sends a token transfer with automatic retry for transient failures
func (s *Server) sendTokens(args map[string]interface{}) (map[string]interface{}, error) {
	from, ok := args["from"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: from")
	}

	to, ok := args["to"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: to")
	}

	amountStr, ok := args["amount"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: amount")
	}

	privateKey, ok := args["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: private_key")
	}

	// Parse amount and convert to credits (1 ACME = 1e8 credits)
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	amountInCredits := int64(amount * 1e8)

	helper := NewClientHelper(s.state)
	network := helper.GetNetwork(args)

	return helper.WithClientRetry(network, nil, func(ctx context.Context, c *client.Client) (map[string]interface{}, error) {
		txHash, err := c.SendTokens(ctx, from, to, amountInCredits, privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to send tokens: %w", err)
		}

		text := fmt.Sprintf("Transaction submitted successfully!\n\nFrom: %s\nTo: %s\nAmount: %s ACME\nNetwork: %s\nTransaction Hash: %s", from, to, amountStr, network, hex.EncodeToString(txHash))

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		}, nil
	})
}
