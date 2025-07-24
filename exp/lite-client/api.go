package liteclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TokenAccountAPI defines methods for querying token account data in the lite client.
type TokenAccountAPI interface {
	// GetBalance returns the balance for a token account.
	GetBalance(ctx context.Context, accountUrl string) (BalanceResult, error)
	// GetTransactions returns up to 'limit' transactions for a token account (all if limit<=0).
	GetTransactions(ctx context.Context, accountUrl string, limit int) ([]Transaction, error)
	// GetBalanceAndTransactions returns both the balance and up to 'limit' transactions.
	GetBalanceAndTransactions(ctx context.Context, accountUrl string, limit int) (BalanceResult, []Transaction, error)

	// PullAllTransactions fetches and caches all transactions for an account.
	PullAllTransactions(ctx context.Context, accountUrl string) error
	// GetStoredTransactions retrieves cached transactions for an account.
	GetStoredTransactions(accountUrl string) []Transaction
	// GetStoredBalance retrieves cached balance for an account.
	GetStoredBalance(accountUrl string) (BalanceResult, bool)
	// StoreBalance caches balance data for an account.
	StoreBalance(accountUrl string, balance BalanceResult)
}

// BalanceResult holds token balance and metadata.
type BalanceResult struct {
	AccountUrl string
	Balance    string
	Token      string
	Height     int64
}

// Implementation of TokenAccountAPI on LiteClient

// GetBalance returns the balance for a token account using v2 API (most stable).
func (c *LiteClient) GetBalance(ctx context.Context, accountUrl string) (BalanceResult, error) {
	fmt.Printf("Getting balance for account %s using v2 API\n", accountUrl)

	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return BalanceResult{}, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use v2 API directly since it's more stable
	if c.v2 == nil {
		return BalanceResult{}, fmt.Errorf("v2 client not available")
	}

	v2req := &v2.UrlQuery{Url: u}
	v2resp := new(v2.ChainQueryResponse)
	err = c.v2.RequestAPIv2(ctx, "query", v2req, v2resp)
	if err != nil {
		return BalanceResult{}, fmt.Errorf("v2 API query failed: %w", err)
	}

	if v2resp.Data == nil {
		return BalanceResult{}, fmt.Errorf("v2 API returned nil data")
	}

	// For debugging, print the type of the response data
	fmt.Printf("Response data type: %T\n", v2resp.Data)

	// Get chain height if available
	var height int64
	if v2resp.MainChain != nil {
		height = int64(v2resp.MainChain.Height)
	}

	// Try to handle as map first (most common response format)
	switch data := v2resp.Data.(type) {
	case map[string]interface{}:
		// Debug the map contents
		fmt.Printf("Map data keys: %v\n", getMapKeys(data))

		// Try to extract balance and token URL
		balance, hasBalance := data["balance"]
		tokenUrl, hasToken := data["tokenUrl"]

		// Also check for alternate key names that might be used
		if !hasBalance {
			balance, hasBalance = data["Balance"]
		}
		if !hasToken {
			tokenUrl, hasToken = data["TokenUrl"]
			if !hasToken {
				tokenUrl, hasToken = data["tokenURL"]
			}
			if !hasToken {
				tokenUrl, hasToken = data["url"]
			}
		}

		// If we have both balance and token URL, create the result
		if hasBalance && hasToken {
			// Convert balance from interface{} to string
			balanceStr := ""
			switch b := balance.(type) {
			case float64:
				// Convert from credits to ACME (divide by 1e8)
				balanceStr = fmt.Sprintf("%.8f", b/1e8)
			case int64:
				balanceStr = fmt.Sprintf("%.8f", float64(b)/1e8)
			case string:
				// Try to parse as float if it's a string
				if val, err := strconv.ParseFloat(b, 64); err == nil {
					balanceStr = fmt.Sprintf("%.8f", val/1e8)
				} else {
					balanceStr = b
				}
			default:
				balanceStr = fmt.Sprintf("%v", b)
			}

			result := BalanceResult{
				AccountUrl: accountUrl,
				Balance:    balanceStr,
				Token:      fmt.Sprintf("%v", tokenUrl),
				Height:     height,
			}
			fmt.Printf("Successfully retrieved balance: %s %s (height %d)\n", result.Balance, result.Token, result.Height)
			return result, nil
		}

		// If we have a data field, try to extract from there
		if nestedData, ok := data["data"].(map[string]interface{}); ok {
			fmt.Printf("Found nested data field with keys: %v\n", getMapKeys(nestedData))

			balance, hasBalance := nestedData["balance"]
			tokenUrl, hasToken := nestedData["tokenUrl"]

			// Check alternate keys
			if !hasBalance {
				balance, hasBalance = nestedData["Balance"]
			}
			if !hasToken {
				tokenUrl, hasToken = nestedData["TokenUrl"]
				if !hasToken {
					tokenUrl, hasToken = nestedData["tokenURL"]
				}
				if !hasToken {
					tokenUrl, hasToken = nestedData["url"]
				}
			}

			if hasBalance && hasToken {
				// Convert balance from interface{} to string
				balanceStr := ""
				switch b := balance.(type) {
				case float64:
					balanceStr = fmt.Sprintf("%.8f", b/1e8)
				case int64:
					balanceStr = fmt.Sprintf("%.8f", float64(b)/1e8)
				case string:
					// Try to parse as float if it's a string
					if val, err := strconv.ParseFloat(b, 64); err == nil {
						balanceStr = fmt.Sprintf("%.8f", val/1e8)
					} else {
						balanceStr = b
					}
				default:
					balanceStr = fmt.Sprintf("%v", b)
				}

				result := BalanceResult{
					AccountUrl: accountUrl,
					Balance:    balanceStr,
					Token:      fmt.Sprintf("%v", tokenUrl),
					Height:     height,
				}
				fmt.Printf("Successfully retrieved balance: %s %s (height %d)\n", result.Balance, result.Token, result.Height)
				return result, nil
			}
		}

	case *protocol.TokenAccount:
		// Handle as TokenAccount
		result := BalanceResult{
			AccountUrl: accountUrl,
			Balance:    data.Balance.String(),
			Token:      data.TokenUrl.String(),
			Height:     height,
		}
		fmt.Printf("Successfully retrieved balance from TokenAccount: %s %s (height %d)\n", result.Balance, result.Token, result.Height)
		return result, nil

	default:
		// Try to marshal and unmarshal to extract data
		fmt.Printf("Attempting to extract data via JSON marshaling for type %T\n", v2resp.Data)
		dataBytes, err := json.Marshal(v2resp.Data)
		if err == nil {
			// Print the raw JSON for debugging
			fmt.Printf("Raw JSON data: %s\n", string(dataBytes))

			// Try to unmarshal into a map
			var dataMap map[string]interface{}
			if err := json.Unmarshal(dataBytes, &dataMap); err == nil {
				fmt.Printf("Unmarshaled JSON keys: %v\n", getMapKeys(dataMap))

				// Look for balance and token
				balance, hasBalance := dataMap["balance"]
				tokenUrl, hasToken := dataMap["tokenUrl"]

				// Check alternate keys
				if !hasBalance {
					balance, hasBalance = dataMap["Balance"]
				}
				if !hasToken {
					tokenUrl, hasToken = dataMap["TokenUrl"]
					if !hasToken {
						tokenUrl, hasToken = dataMap["tokenURL"]
					}
					if !hasToken {
						tokenUrl, hasToken = dataMap["url"]
					}
				}

				if hasBalance && hasToken {
					// Convert balance from interface{} to string
					balanceStr := ""
					switch b := balance.(type) {
					case float64:
						balanceStr = fmt.Sprintf("%.8f", b/1e8)
					case int64:
						balanceStr = fmt.Sprintf("%.8f", float64(b)/1e8)
					case string:
						// Try to parse as float if it's a string
						if val, err := strconv.ParseFloat(b, 64); err == nil {
							balanceStr = fmt.Sprintf("%.8f", val/1e8)
						} else {
							balanceStr = b
						}
					default:
						balanceStr = fmt.Sprintf("%v", b)
					}

					result := BalanceResult{
						AccountUrl: accountUrl,
						Balance:    balanceStr,
						Token:      fmt.Sprintf("%v", tokenUrl),
						Height:     height,
					}
					fmt.Printf("Successfully retrieved balance via JSON: %s %s (height %d)\n", result.Balance, result.Token, result.Height)
					return result, nil
				}
			}
		}
	}

	// If we got here, we couldn't parse the data
	// Print detailed debug information about the response
	dataJSON, _ := json.MarshalIndent(v2resp.Data, "", "  ")
	fmt.Printf("Failed to parse balance data. Raw response data:\n%s\n", string(dataJSON))

	return BalanceResult{}, fmt.Errorf("unable to parse balance data from v2 API response")
}

// Helper function to get map keys for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// GetTransactions returns up to 'limit' transactions for a token account using v2 API (all if limit<=0).
func (c *LiteClient) GetTransactions(ctx context.Context, accountUrl string, limit int) ([]Transaction, error) {
	fmt.Printf("Getting transactions for account %s using v2 API (limit: %d)\n", accountUrl, limit)

	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use v2 API directly since it's more stable
	if c.v2 == nil {
		return nil, fmt.Errorf("v2 client not available")
	}

	// Query transaction history using v2 API
	v2req := &v2.TxHistoryQuery{
		UrlQuery: v2.UrlQuery{
			Url: u,
		},
		QueryPagination: v2.QueryPagination{
			Count: uint64(limit),
		},
	}

	v2resp := new(v2.MultiResponse)
	err = c.v2.RequestAPIv2(ctx, "query-tx-history", v2req, v2resp)
	if err != nil {
		return nil, fmt.Errorf("v2 API transaction query failed: %w", err)
	}

	var results []Transaction
	if v2resp.Items != nil {
		for _, item := range v2resp.Items {
			if item == nil {
				continue
			}

			// Parse transaction data from v2 response
			if txData, ok := item.(map[string]interface{}); ok {
				txResult := Transaction{
					Status:  "delivered", // Default status
					Account: accountUrl,  // Set the account URL
					Height:  0,           // Default height
				}

				if txid, ok := txData["txid"]; ok {
					txResult.TxID = fmt.Sprintf("%v", txid)
				}
				if txType, ok := txData["type"]; ok {
					txResult.Type = fmt.Sprintf("%v", txType)
				}
				if timestamp, ok := txData["timestamp"]; ok {
					if ts, ok := timestamp.(float64); ok {
						txResult.Timestamp = int64(ts)
					}
				}
				if status, ok := txData["status"]; ok {
					txResult.Status = fmt.Sprintf("%v", status)
				}
				if amount, ok := txData["amount"]; ok {
					txResult.Amount = fmt.Sprintf("%v", amount)
				}
				if from, ok := txData["from"]; ok {
					txResult.From = fmt.Sprintf("%v", from)
				}
				if to, ok := txData["to"]; ok {
					txResult.To = fmt.Sprintf("%v", to)
				}

				results = append(results, txResult)
			}

			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	fmt.Printf("Successfully retrieved %d transactions for %s\n", len(results), accountUrl)
	return results, nil
}

// GetBalanceAndTransactions returns both the balance and up to 'limit' transactions.
func (c *LiteClient) GetBalanceAndTransactions(ctx context.Context, accountUrl string, limit int) (BalanceResult, []Transaction, error) {
	bal, err := c.GetBalance(ctx, accountUrl)
	if err != nil {
		return bal, nil, err
	}
	txs, err := c.GetTransactions(ctx, accountUrl, limit)
	return bal, txs, err
}

// GetStoredBalance retrieves cached balance for an account.
func (c *LiteClient) GetStoredBalance(accountUrl string) (BalanceResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	balance, exists := c.balances[accountUrl]
	return balance, exists
}

// StoreBalance caches balance data for an account.
func (c *LiteClient) StoreBalance(accountUrl string, balance BalanceResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.balances[accountUrl] = balance
	fmt.Printf("Stored balance for account %s: %s %s\n", accountUrl, balance.Balance, balance.Token)
}

// StoreTransaction stores a single transaction in the local cache.
// It validates the transaction before storing and ensures thread safety.
func (c *LiteClient) StoreTransaction(txRecord Transaction) error {
	if err := c.validateTransaction(txRecord); err != nil {
		return fmt.Errorf("invalid transaction: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.transactions[txRecord.Account] = append(c.transactions[txRecord.Account], txRecord)
	fmt.Printf("Stored transaction %s for account %s\n", txRecord.TxID, txRecord.Account)
	return nil
}

// GetStoredTransactions retrieves all cached transactions for the specified account.
// Returns an empty slice if no transactions are found for the account.
func (c *LiteClient) GetStoredTransactions(accountUrl string) ([]Transaction, error) {
	if err := c.validateAccountURL(accountUrl); err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	txs, exists := c.transactions[accountUrl]
	if !exists {
		return []Transaction{}, nil
	}
	return txs, nil
}

// PullAllTransactions fetches all transactions for an account and stores them in the local cache.
// This operation replaces any existing cached transactions for the account.
func (c *LiteClient) PullAllTransactions(ctx context.Context, accountUrl string) error {
	if err := c.validateAccountURL(accountUrl); err != nil {
		return fmt.Errorf("invalid account URL: %w", err)
	}

	fmt.Printf("Pulling all transactions for account %s using v2 API\n", accountUrl)

	// Fetch all transactions
	txs, err := c.GetTransactions(ctx, accountUrl, 0) // 0 means get all
	if err != nil {
		return fmt.Errorf("failed to fetch transactions: %w", err)
	}

	// Store transactions in bulk
	c.mu.Lock()
	c.transactions[accountUrl] = txs
	c.mu.Unlock()

	fmt.Printf("Successfully pulled and stored %d transactions for %s\n", len(txs), accountUrl)
	return nil
}
