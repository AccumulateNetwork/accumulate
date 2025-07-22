package lightclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OperatorsInfo holds information about the network operators
type OperatorsInfo struct {
	KeyBook   *KeyBook
	KeyPages  []*KeyPage
	AllKeys   []string
	Threshold int
}

// GetOperators retrieves the complete operators keybook and all key pages
func (c *Client) GetOperators(ctx context.Context) (*OperatorsInfo, error) {
	operatorsURL := "acc://dn.acme/operators"
	
	keyBook, keyPages, err := c.GetKeyBookWithPages(ctx, operatorsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get operators keybook: %w", err)
	}

	// Collect all keys from all pages
	var allKeys []string
	for _, page := range keyPages {
		allKeys = append(allKeys, page.Keys...)
	}

	return &OperatorsInfo{
		KeyBook:   keyBook,
		KeyPages:  keyPages,
		AllKeys:   allKeys,
		Threshold: keyBook.Threshold,
	}, nil
}

// GetStakingRegistry retrieves the staking registry by querying data entries
func (c *Client) GetStakingRegistry(ctx context.Context) ([]string, error) {
	// Query data entries from the staking registry
	query := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "query",
		"id":      1,
		"params": map[string]interface{}{
			"scope": "staking.acme/registered",
			"query": map[string]interface{}{
				"queryType": "data",
				"range": map[string]interface{}{
					"start": 0,
					"count": 100, // Get up to 100 entries
				},
			},
		},
	}

	// Use the raw JSON-RPC query method from the client
	reqBody, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	resp, err := http.Post(c.serverURL+"/v3", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to send query: %w", err)
	}
	defer resp.Body.Close()

	// Read and return the raw JSON response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Add debug logging to show the raw response
	fmt.Printf("Raw response: %s\n", string(respBody))

	return []string{string(respBody)}, nil
}

// GetStakingAccounts retrieves the staking registry and all registered staking accounts
func (c *Client) GetStakingAccounts(ctx context.Context) ([]string, []*AccountInfo, error) {
	// Get the raw JSON response from the registry
	jsonResponses, err := c.GetStakingRegistry(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get staking registry: %w", err)
	}

	// Parse the JSON response
	var stakingAccounts []*AccountInfo
	for _, jsonResp := range jsonResponses {
		// Parse the JSON response to extract URLs
		var registryResp struct {
			Result struct {
				Entries []struct {
					Value struct {
						Entry struct {
							Data []string `json:"data"`
						} `json:"entry"`
					} `json:"value"`
				} `json:"entries"`
			} `json:"result"`
		}
		
		if err := json.Unmarshal([]byte(jsonResp), &registryResp); err != nil {
			fmt.Printf("Warning: failed to parse JSON response: %v\n", err)
			continue
		}

		// Process each entry in the response
		for _, entry := range registryResp.Result.Entries {
			for _, dataItem := range entry.Value.Entry.Data {
				// Validate and correct the URL
				validatedURL, issues := validateAndCorrectURL(dataItem)
				if len(issues) > 0 {
					fmt.Printf("Warning: URL '%s' has issues:\n", dataItem)
					for _, issue := range issues {
						fmt.Printf("  - %s\n", issue)
					}
					fmt.Printf("  - Using corrected URL: %s\n", validatedURL)
				}

				// Get the account info for this URL
				account, err := c.GetTokenAccount(ctx, validatedURL)
				if err != nil {
					fmt.Printf("Warning: failed to get account %s: %v\n", validatedURL, err)
					continue
				}

				// Create account info
				accountInfo := &AccountInfo{
					URL:        validatedURL,
					Type:       "token",
					Balance:    account.Balance,
					TokenURL:   validatedURL,
					Authorities: []string{},
				}
				
				stakingAccounts = append(stakingAccounts, accountInfo)
			}
		}
	}

	// Return the JSON response and parsed accounts
	return jsonResponses, stakingAccounts, nil
}

// GetStakingAccountsWithTotal retrieves all staking accounts and calculates total staked
func (c *Client) GetStakingAccountsWithTotal(ctx context.Context) ([]*AccountInfo, int64, error) {
	_, stakingAccounts, err := c.GetStakingAccounts(ctx)
	if err != nil {
		return nil, 0, err
	}

	var totalStaked int64
	var accountInfos []*AccountInfo
	for _, account := range stakingAccounts {
		totalStaked += account.Balance
		accountInfo := &AccountInfo{
			URL:    account.URL,
			Type:   "token",
			Balance: account.Balance,
		}
		accountInfos = append(accountInfos, accountInfo)
	}

	return accountInfos, totalStaked, nil
}

// AccountInfo provides a generic interface for account information
type AccountInfo struct {
	URL         string
	Type        string
	Balance     int64
	TokenURL    string
	Authorities []string
	Data        map[string]interface{}
}

// validateAndCorrectURL validates and corrects an Accumulate URL
// Returns the corrected URL and any validation issues
func validateAndCorrectURL(url string) (string, []string) {
	var issues []string
	
	// Ensure URL starts with "acc://"
	if !strings.HasPrefix(url, "acc://") {
		issues = append(issues, "URL missing acc:// prefix")
		url = "acc://" + url
	}
	
	// Ensure URL has at least one dot after the prefix
	if !strings.Contains(url[6:], ".") {
		issues = append(issues, "URL missing domain component")
	}
	
	// Ensure URL doesn't end with a slash
	if strings.HasSuffix(url, "/") {
		issues = append(issues, "URL ends with trailing slash")
		url = strings.TrimSuffix(url, "/")
	}
	
	// Validate URL components
	urlComponents := strings.Split(url[6:], "/")
	if len(urlComponents) < 2 {
		issues = append(issues, "URL missing required components")
	}
	
	// Ensure domain name is valid
	if len(urlComponents) > 0 {
		domain := urlComponents[0]
		if len(domain) < 1 {
			issues = append(issues, "Empty domain name")
		}
		if !strings.Contains(domain, ".") {
			issues = append(issues, "Domain name missing dot separator")
		}
	}
	
	return url, issues
}

// GetAccountInfo retrieves basic information about any account
func (c *Client) GetAccountInfo(ctx context.Context, accountURL string) (*AccountInfo, error) {
	resp, err := c.Query(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query account %s: %w", accountURL, err)
	}

	accountType, err := resp.GetType()
	if err != nil {
		return nil, fmt.Errorf("failed to get account type: %w", err)
	}

	data, err := resp.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	info := &AccountInfo{
		URL:  accountURL,
		Type: accountType,
		Data: data,
	}

	// Extract balance if available
	if balance, ok := data["balance"]; ok {
		if balanceFloat, ok := balance.(float64); ok {
			info.Balance = int64(balanceFloat)
		}
	}

	// Extract authorities if available
	if authorities, ok := data["authorities"]; ok {
		if authSlice, ok := authorities.([]interface{}); ok {
			for _, auth := range authSlice {
				if authStr, ok := auth.(string); ok {
					info.Authorities = append(info.Authorities, authStr)
				}
			}
		}
	}

	return info, nil
}

// BatchGetAccounts retrieves information for multiple accounts
func (c *Client) BatchGetAccounts(ctx context.Context, accountURLs []string) ([]*AccountInfo, error) {
	var accounts []*AccountInfo
	
	for _, accountURL := range accountURLs {
		account, err := c.GetAccountInfo(ctx, accountURL)
		if err != nil {
			// Log error but continue with other accounts
			fmt.Printf("Warning: failed to get account %s: %v\n", accountURL, err)
			continue
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

// SearchAccounts searches for accounts by pattern (this would need to be implemented based on available API endpoints)
func (c *Client) SearchAccounts(ctx context.Context, pattern string) ([]string, error) {
	// This is a placeholder - actual implementation would depend on available search endpoints
	return nil, fmt.Errorf("search functionality not yet implemented")
}

// parseStakingRegistryEntries parses the JSON response from the staking registry query
// and extracts the staking account URLs from the data entries
func (c *Client) parseStakingRegistryEntries(resp []byte) ([]string, error) {
	type DataEntry struct {
		Data [][]byte `json:"data"`
	}
	
	type ChainEntryRecord struct {
		Value struct {
			Entry struct {
				Data [][]byte `json:"data"`
			} `json:"entry"`
		} `json:"value"`
	}
	
	type Response struct {
		Result struct {
			Entries []*ChainEntryRecord `json:"entries"`
		} `json:"result"`
	}

	var result Response
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var stakingURLs []string
	for _, entry := range result.Result.Entries {
		// Extract URLs from data entries
		if len(entry.Value.Entry.Data) > 0 {
			for _, dataItem := range entry.Value.Entry.Data {
				if len(dataItem) >= 6 && string(dataItem[:6]) == "acc://" {
					stakingURLs = append(stakingURLs, string(dataItem))
				}
			}
		}
	}

	return stakingURLs, nil
}

// ... (rest of the code remains the same)
