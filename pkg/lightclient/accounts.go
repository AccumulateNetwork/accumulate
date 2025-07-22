package lightclient

import (
	"context"
	"fmt"
)

// ADI represents an Accumulate Digital Identity
type ADI struct {
	URL         string
	Type        string
	KeyBook     string
	Authorities []string
	Data        map[string]interface{}
}

// TokenAccount represents a token account
type TokenAccount struct {
	URL         string
	Type        string
	TokenURL    string
	Balance     int64
	Authorities []string
	Data        map[string]interface{}
}

// DataAccount represents a data account
type DataAccount struct {
	URL         string
	Type        string
	Entries     []DataEntry
	Authorities []string
	Data        map[string]interface{}
}

// DataEntry represents an entry in a data account
type DataEntry struct {
	Hash  string
	Data  []byte
	Index int64
}

// KeyBook represents a key book
type KeyBook struct {
	URL       string
	Type      string
	Pages     []string
	Threshold int
	Data      map[string]interface{}
}

// KeyPage represents a key page
type KeyPage struct {
	URL       string
	Type      string
	Keys      []string
	Threshold int
	Data      map[string]interface{}
}

// GetADI retrieves and parses an ADI account
func (c *Client) GetADI(ctx context.Context, adiURL string) (*ADI, error) {
	resp, err := c.Query(ctx, adiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query ADI %s: %w", adiURL, err)
	}

	accountType, err := resp.GetType()
	if err != nil {
		return nil, fmt.Errorf("failed to get account type: %w", err)
	}

	if accountType != "identity" {
		return nil, fmt.Errorf("account %s is not an ADI, got type: %s", adiURL, accountType)
	}

	data, err := resp.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	adi := &ADI{
		URL:  adiURL,
		Type: accountType,
		Data: data,
	}

	// Extract key book URL
	if keyBook, ok := data["keyBook"]; ok {
		if keyBookStr, ok := keyBook.(string); ok {
			adi.KeyBook = keyBookStr
		}
	}

	// Extract authorities
	if authorities, ok := data["authorities"]; ok {
		if authSlice, ok := authorities.([]interface{}); ok {
			for _, auth := range authSlice {
				if authStr, ok := auth.(string); ok {
					adi.Authorities = append(adi.Authorities, authStr)
				}
			}
		}
	}

	return adi, nil
}

// GetTokenAccount retrieves and parses a token account
func (c *Client) GetTokenAccount(ctx context.Context, tokenURL string) (*TokenAccount, error) {
	resp, err := c.Query(ctx, tokenURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query token account %s: %w", tokenURL, err)
	}

	accountType, err := resp.GetType()
	if err != nil {
		return nil, fmt.Errorf("failed to get account type: %w", err)
	}

	if accountType != "tokenAccount" {
		return nil, fmt.Errorf("account %s is not a token account, got type: %s", tokenURL, accountType)
	}

	data, err := resp.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	tokenAccount := &TokenAccount{
		URL:  tokenURL,
		Type: accountType,
		Data: data,
	}

	// Extract token URL
	if tokenURLField, ok := data["tokenUrl"]; ok {
		if tokenURLStr, ok := tokenURLField.(string); ok {
			tokenAccount.TokenURL = tokenURLStr
		}
	}

	// Extract balance
	if balance, ok := data["balance"]; ok {
		if balanceFloat, ok := balance.(float64); ok {
			tokenAccount.Balance = int64(balanceFloat)
		}
	}

	// Extract authorities
	if authorities, ok := data["authorities"]; ok {
		if authSlice, ok := authorities.([]interface{}); ok {
			for _, auth := range authSlice {
				if authStr, ok := auth.(string); ok {
					tokenAccount.Authorities = append(tokenAccount.Authorities, authStr)
				}
			}
		}
	}

	return tokenAccount, nil
}

// GetDataAccount retrieves and parses a data account
func (c *Client) GetDataAccount(ctx context.Context, dataURL string) (*DataAccount, error) {
	resp, err := c.Query(ctx, dataURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query data account %s: %w", dataURL, err)
	}

	accountType, err := resp.GetType()
	if err != nil {
		return nil, fmt.Errorf("failed to get account type: %w", err)
	}

	if accountType != "dataAccount" {
		return nil, fmt.Errorf("account %s is not a data account, got type: %s", dataURL, accountType)
	}

	data, err := resp.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	dataAccount := &DataAccount{
		URL:  dataURL,
		Type: accountType,
		Data: data,
	}

	// Extract entries (this would need to be expanded based on actual data structure)
	if entries, ok := data["entries"]; ok {
		if entriesSlice, ok := entries.([]interface{}); ok {
			for i, entry := range entriesSlice {
				if entryMap, ok := entry.(map[string]interface{}); ok {
					dataEntry := DataEntry{
						Index: int64(i),
					}
					
					if hash, ok := entryMap["hash"]; ok {
						if hashStr, ok := hash.(string); ok {
							dataEntry.Hash = hashStr
						}
					}
					
					if entryData, ok := entryMap["data"]; ok {
						if dataBytes, ok := entryData.([]byte); ok {
							dataEntry.Data = dataBytes
						}
					}
					
					dataAccount.Entries = append(dataAccount.Entries, dataEntry)
				}
			}
		}
	}

	// Extract authorities
	if authorities, ok := data["authorities"]; ok {
		if authSlice, ok := authorities.([]interface{}); ok {
			for _, auth := range authSlice {
				if authStr, ok := auth.(string); ok {
					dataAccount.Authorities = append(dataAccount.Authorities, authStr)
				}
			}
		}
	}

	return dataAccount, nil
}

// GetKeyBook retrieves and parses a key book
func (c *Client) GetKeyBook(ctx context.Context, keyBookURL string) (*KeyBook, error) {
	resp, err := c.Query(ctx, keyBookURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query key book %s: %w", keyBookURL, err)
	}

	accountType, err := resp.GetType()
	if err != nil {
		return nil, fmt.Errorf("failed to get account type: %w", err)
	}

	if accountType != "keyBook" {
		return nil, fmt.Errorf("account %s is not a key book, got type: %s", keyBookURL, accountType)
	}

	data, err := resp.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	keyBook := &KeyBook{
		URL:  keyBookURL,
		Type: accountType,
		Data: data,
	}

	// Extract pages from directory field (v3 API format)
	if directory, ok := resp.Result["directory"]; ok {
		if dirMap, ok := directory.(map[string]interface{}); ok {
			if records, ok := dirMap["records"]; ok {
				if recordsSlice, ok := records.([]interface{}); ok {
					for _, record := range recordsSlice {
						if recordMap, ok := record.(map[string]interface{}); ok {
							if value, ok := recordMap["value"]; ok {
								if pageURL, ok := value.(string); ok {
									keyBook.Pages = append(keyBook.Pages, pageURL)
								}
							}
						}
					}
				}
			}
		}
	}

	// Fallback: Extract pages from data field (older API format)
	if len(keyBook.Pages) == 0 {
		if pages, ok := data["pages"]; ok {
			if pagesSlice, ok := pages.([]interface{}); ok {
				for _, page := range pagesSlice {
					if pageStr, ok := page.(string); ok {
						keyBook.Pages = append(keyBook.Pages, pageStr)
					}
				}
			}
		}
	}

	// Extract threshold - try multiple locations
	if threshold, ok := data["threshold"]; ok {
		if thresholdFloat, ok := threshold.(float64); ok {
			keyBook.Threshold = int(thresholdFloat)
		}
	}
	// Default threshold for operators is 1 if not specified
	if keyBook.Threshold == 0 && len(keyBook.Pages) > 0 {
		keyBook.Threshold = 1
	}

	return keyBook, nil
}

// GetKeyPage retrieves and parses a key page
func (c *Client) GetKeyPage(ctx context.Context, keyPageURL string) (*KeyPage, error) {
	resp, err := c.Query(ctx, keyPageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query key page %s: %w", keyPageURL, err)
	}

	accountType, err := resp.GetType()
	if err != nil {
		return nil, fmt.Errorf("failed to get account type: %w", err)
	}

	if accountType != "keyPage" {
		return nil, fmt.Errorf("account %s is not a key page, got type: %s", keyPageURL, accountType)
	}

	data, err := resp.GetData()
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	keyPage := &KeyPage{
		URL:  keyPageURL,
		Type: accountType,
		Data: data,
	}

	// Extract keys
	if keys, ok := data["keys"]; ok {
		if keysSlice, ok := keys.([]interface{}); ok {
			for _, key := range keysSlice {
				if keyStr, ok := key.(string); ok {
					keyPage.Keys = append(keyPage.Keys, keyStr)
				}
			}
		}
	}

	// Extract threshold
	if threshold, ok := data["threshold"]; ok {
		if thresholdFloat, ok := threshold.(float64); ok {
			keyPage.Threshold = int(thresholdFloat)
		}
	}

	return keyPage, nil
}

// GetKeyBookWithPages retrieves a key book and all its key pages
func (c *Client) GetKeyBookWithPages(ctx context.Context, keyBookURL string) (*KeyBook, []*KeyPage, error) {
	keyBook, err := c.GetKeyBook(ctx, keyBookURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get key book: %w", err)
	}

	var keyPages []*KeyPage
	for _, pageURL := range keyBook.Pages {
		keyPage, err := c.GetKeyPage(ctx, pageURL)
		if err != nil {
			return keyBook, keyPages, fmt.Errorf("failed to get key page %s: %w", pageURL, err)
		}
		keyPages = append(keyPages, keyPage)
	}

	return keyBook, keyPages, nil
}
