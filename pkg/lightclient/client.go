package lightclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides access to Accumulate network data with cryptographic proofs
type Client struct {
	serverURL  string
	httpClient *http.Client
}

// NewClient creates a new light client instance
func NewClient(server string) (*Client, error) {
	switch server {
	case "local":
		return &Client{
			serverURL: "http://localhost:26657",
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		}, nil
	case "testnet":
		return &Client{
			serverURL: "https://testnet.accumulate.network/v3",
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		}, nil
	case "mainnet":
		return &Client{
			serverURL: "https://mainnet.accumulate.network/v3",
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		}, nil
	default:
		// Add default protocol if not specified
		if !strings.HasPrefix(server, "http") {
			server = "https://" + server
		}
		// Ensure v3 endpoint
		if !strings.HasSuffix(server, "/v3") {
			if strings.HasSuffix(server, "/") {
				server += "v3"
			} else {
				server += "/v3"
			}
		}
		return &Client{
			serverURL: server,
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		}, nil
	}
}

// Query sends a JSON-RPC 2.0 query request to the Accumulate API
func (c *Client) Query(ctx context.Context, accountURL string) (*QueryResponse, error) {
	// Create JSON-RPC 2.0 request
	jsonRPCRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "query",
		"params": map[string]interface{}{
			"scope": accountURL,
		},
		"id": 1,
	}

	// Marshal the query to JSON
	queryBytes, err := json.Marshal(jsonRPCRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.serverURL, bytes.NewBuffer(queryBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the JSON response
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Check for JSON-RPC error
	if errField, ok := jsonResp["error"]; ok {
		return nil, fmt.Errorf("API error: %v", errField)
	}

	// Extract result
	result, ok := jsonResp["result"]
	if !ok {
		return nil, fmt.Errorf("no result field in response")
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("result is not a map")
	}

	return &QueryResponse{
		URL:    accountURL,
		Result: resultMap,
	}, nil
}

// QueryResponse represents the response from a query
type QueryResponse struct {
	URL    string
	Result map[string]interface{}
}

// GetData returns the data field from the query response
func (qr *QueryResponse) GetData() (map[string]interface{}, error) {
	// Check for account field first (v3 API format)
	if account, ok := qr.Result["account"]; ok {
		if accountMap, ok := account.(map[string]interface{}); ok {
			return accountMap, nil
		}
	}

	// Fallback to data field (older API format)
	data, ok := qr.Result["data"]
	if !ok {
		return nil, fmt.Errorf("no account or data field in response")
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data is not a map")
	}

	return dataMap, nil
}

// GetType returns the account type from the response
func (r *QueryResponse) GetType() (string, error) {
	// Check for account field first (v3 API format)
	if account, ok := r.Result["account"]; ok {
		if accountMap, ok := account.(map[string]interface{}); ok {
			if accountType, ok := accountMap["type"]; ok {
				if typeStr, ok := accountType.(string); ok {
					return typeStr, nil
				}
			}
		}
	}

	// Fallback to data field (older API format)
	data, ok := r.Result["data"]
	if !ok {
		return "", fmt.Errorf("no account or data field in response")
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("data is not a map")
	}

	accountType, ok := dataMap["type"]
	if !ok {
		return "", fmt.Errorf("no type field in data")
	}

	typeStr, ok := accountType.(string)
	if !ok {
		return "", fmt.Errorf("type is not a string")
	}

	return typeStr, nil
}
