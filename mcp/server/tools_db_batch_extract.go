package server

import (
	"fmt"
)

// dbExtractAccountsBatch extracts multiple accounts in a single call
func (s *Server) dbExtractAccountsBatch(args map[string]interface{}) (map[string]interface{}, error) {
	// Parse accounts array
	accountsRaw, ok := args["accounts"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("accounts parameter required and must be an array")
	}

	if len(accountsRaw) == 0 {
		return nil, fmt.Errorf("accounts array cannot be empty")
	}

	// Parse options
	includeChains := false
	if ic, ok := args["include_chains"].(bool); ok {
		includeChains = ic
	}

	includeTransactions := false
	if it, ok := args["include_transactions"].(bool); ok {
		includeTransactions = it
	}

	database := ""
	if db, ok := args["database"].(string); ok {
		database = db
	}

	// Convert accounts to strings
	var accountURLs []string
	for _, acct := range accountsRaw {
		if acctStr, ok := acct.(string); ok {
			accountURLs = append(accountURLs, acctStr)
		}
	}

	// Extract each account
	var extractedAccounts []interface{}
	var failed []map[string]interface{}
	extractedCount := 0

	for _, url := range accountURLs {
		// Query account
		queryArgs := map[string]interface{}{
			"url": url,
		}
		if database != "" {
			queryArgs["database"] = database
		}

		accountData, err := s.dbQueryAccount(queryArgs)
		if err != nil {
			// Add to failed list
			failed = append(failed, map[string]interface{}{
				"url":   url,
				"error": err.Error(),
			})
			continue
		}

		// Build account result
		acctResult := map[string]interface{}{
			"url": url,
		}

		// Extract account data from content
		if content, ok := accountData["content"].([]map[string]interface{}); ok && len(content) > 0 {
			if text, ok := content[0]["text"].(string); ok {
				acctResult["data"] = text

				// Try to parse type if available
				// This is a simplified approach - in production you'd parse the JSON
				acctResult["type"] = "account"
			}
		}

		// Include chains if requested
		if includeChains {
			chains, err := s.getAccountChains(url, database)
			if err == nil && chains != nil {
				acctResult["chains"] = chains
			}
		}

		// Include transactions if requested
		if includeTransactions {
			transactions, err := s.getAccountTransactions(url, database)
			if err == nil && transactions != nil {
				acctResult["transactions"] = transactions
			}
		}

		extractedAccounts = append(extractedAccounts, acctResult)
		extractedCount++
	}

	// Return result
	result := map[string]interface{}{
		"accounts":        extractedAccounts,
		"extracted_count": extractedCount,
		"failed":          failed,
	}

	return result, nil
}

// getAccountChains retrieves chains for an account (helper method)
func (s *Server) getAccountChains(url, database string) ([]interface{}, error) {
	// Query main chain
	chainArgs := map[string]interface{}{
		"url":        url,
		"chain_name": "main",
		"count":      float64(100),
	}
	if database != "" {
		chainArgs["database"] = database
	}

	chainResult, err := s.dbQueryChain(chainArgs)
	if err != nil {
		return nil, err
	}

	// Parse chain entries
	var chains []interface{}
	if content, ok := chainResult["content"].([]map[string]interface{}); ok && len(content) > 0 {
		if text, ok := content[0]["text"].(string); ok {
			chains = append(chains, map[string]interface{}{
				"name": "main",
				"data": text,
			})
		}
	}

	return chains, nil
}

// getAccountTransactions retrieves transactions for an account (helper method)
func (s *Server) getAccountTransactions(url, database string) ([]interface{}, error) {
	// Query chain to get transaction hashes
	chainArgs := map[string]interface{}{
		"url":        url,
		"chain_name": "main",
		"count":      float64(100),
	}
	if database != "" {
		chainArgs["database"] = database
	}

	chainResult, err := s.dbQueryChain(chainArgs)
	if err != nil {
		return nil, err
	}

	// Parse transactions (simplified - would need full implementation)
	var transactions []interface{}
	if content, ok := chainResult["content"].([]map[string]interface{}); ok && len(content) > 0 {
		if text, ok := content[0]["text"].(string); ok {
			transactions = append(transactions, map[string]interface{}{
				"data": text,
			})
		}
	}

	return transactions, nil
}
