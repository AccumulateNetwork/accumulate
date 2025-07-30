// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	v2api "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AccountHandler is responsible for retrieving and processing account data.
// It handles account type detection, data retrieval, and type-specific processing.
type AccountHandler struct {
	client *LiteClient
}

// NewAccountHandler creates a new account handler with the given lite client.
func NewAccountHandler(client *LiteClient) *AccountHandler {
	return &AccountHandler{
		client: client,
	}
}

// GetAccountData retrieves account data for the specified account URL.
// It checks the cache first and falls back to network queries if needed.
func (ah *AccountHandler) GetAccountData(ctx context.Context, accountURL string) (*AccountData, error) {
	// Validate account URL
	if err := ah.client.validateAccountURL(accountURL); err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Check cache first
	if cachedData, found := ah.client.unifiedCache.GetAccountData(accountURL); found {
		// Check if data is fresh enough
		if time.Since(cachedData.LastUpdated) < ah.client.unifiedCache.defaultTTL {
			return cachedData, nil
		}
	}

	// Fetch from network
	return ah.getAccountDataFromNetwork(ctx, accountURL)
}

// getAccountDataFromNetwork retrieves account data from the network (internal helper)
func (ah *AccountHandler) getAccountDataFromNetwork(ctx context.Context, accountUrl string) (*AccountData, error) {
	fmt.Printf("Getting account data for %s using universal API\n", accountUrl)

	// Check cache first
	if cached, found := ah.client.unifiedCache.GetAccountData(accountUrl); found {
		fmt.Printf("Retrieved %s account from cache: %s\n", cached.TypeName, accountUrl)
		return cached, nil
	}

	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use v2 API GeneralQuery to get account data
	query := &v2api.GeneralQuery{
		UrlQuery: v2api.UrlQuery{Url: u},
	}

	// Query the account using the standard v2 API
	resp, err := ah.client.v2.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	// The v2 API returns a map[string]interface{} that we need to parse
	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}

	// Extract account type from response
	accountTypeName, ok := respMap["type"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid account type in response")
	}

	accountType, ok := protocol.AccountTypeByName(accountTypeName)
	if !ok {
		return nil, fmt.Errorf("unknown account type '%s'", accountTypeName)
	}

	// Extract the data field which contains the account information
	dataField, ok := respMap["data"]
	if !ok {
		return nil, fmt.Errorf("missing data field in response")
	}

	// Create AccountData structure
	accountData := &AccountData{
		URL:         accountUrl,
		Type:        accountType,
		TypeName:    accountTypeName,
		Data:        dataField,
		LastUpdated: time.Now(),
		RawResponse: respMap,
	}

	// Cache the result
	ah.client.unifiedCache.StoreAccountData(accountUrl, accountData)

	return accountData, nil
}

// GetTokenBalance retrieves the balance for a token account.
func (ah *AccountHandler) GetTokenBalance(ctx context.Context, accountURL string) (*TokenBalanceInfo, error) {
	// Check cache first
	if cached, found := ah.client.unifiedCache.GetBalance(accountURL); found {
		return cached, nil
	}

	accountData, err := ah.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	if !accountData.IsTokenAccount() {
		return nil, fmt.Errorf("account %s is not a token account", accountURL)
	}

	var balanceInfo *TokenBalanceInfo

	switch accountData.Type {
	case protocol.AccountTypeLiteTokenAccount:
		liteToken, err := accountData.AsLiteTokenAccount()
		if err != nil {
			return nil, err
		}
		balanceInfo = &TokenBalanceInfo{
			AccountURL:    accountURL,
			AccountType:   "lite_token",
			Balance:       liteToken.Balance.String(),
			TokenURL:      liteToken.TokenUrl.String(),
		}

	case protocol.AccountTypeTokenAccount:
		token, err := accountData.AsTokenAccount()
		if err != nil {
			return nil, err
		}
		balanceInfo = &TokenBalanceInfo{
			AccountURL:  accountURL,
			AccountType: "token",
			Balance:     token.Balance.String(),
			TokenURL:    token.TokenUrl.String(),
		}

	default:
		return nil, fmt.Errorf("unsupported token account type: %s", accountData.TypeName)
	}

	// Store in cache
	ah.client.unifiedCache.StoreBalance(accountURL, balanceInfo)
	return balanceInfo, nil
}

// GetIdentityInfo retrieves identity information for an ADI.
func (ah *AccountHandler) GetIdentityInfo(ctx context.Context, accountURL string) (*IdentityInfo, error) {
	// Check cache first
	if cached, found := ah.client.unifiedCache.GetIdentityInfo(accountURL); found {
		return cached, nil
	}

	accountData, err := ah.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	if !accountData.IsIdentityAccount() {
		return nil, fmt.Errorf("account %s is not an identity account", accountURL)
	}

	adi, err := accountData.AsADI()
	if err != nil {
		return nil, err
	}

	identityInfo := &IdentityInfo{
		AccountURL:  accountURL,
		IdentityURL: adi.Url.String(),
		KeyBook:     adi.KeyBook().String(),
	}

	// Store in cache
	ah.client.unifiedCache.StoreIdentityInfo(accountURL, identityInfo)
	return identityInfo, nil
}

// DiscoverADIAccounts discovers all accounts associated with an ADI.
func (ah *AccountHandler) DiscoverADIAccounts(ctx context.Context, adiURL string) ([]string, error) {
	// Validate ADI URL
	if err := ah.client.validateAccountURL(adiURL); err != nil {
		return nil, fmt.Errorf("invalid ADI URL: %w", err)
	}

	// Check cache first
	cachedAccounts := ah.client.unifiedCache.GetADIAccounts(adiURL)
	if len(cachedAccounts) > 0 {
		// Extract URLs from cached account data
		urls := make([]string, len(cachedAccounts))
		for i, account := range cachedAccounts {
			urls[i] = account.URL
		}
		return urls, nil
	}

	// Get identity info to discover accounts
	identityInfo, err := ah.GetAccountData(ctx, adiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity info: %w", err)
	}

	// Start with the ADI itself
	accountURLs := []string{adiURL}

	// Add token accounts if available
	if data, ok := identityInfo.Data.(map[string]interface{}); ok {
		if tokens, ok := data["tokenAccounts"].([]string); ok && len(tokens) > 0 {
			accountURLs = append(accountURLs, tokens...)
		}
	}

	// Add key book if available
	if data, ok := identityInfo.Data.(map[string]interface{}); ok {
		if keyBook, ok := data["keyBook"].(string); ok && keyBook != "" {
			accountURLs = append(accountURLs, keyBook)

			// Get key pages from key book
			keyBookData, err := ah.GetAccountData(ctx, keyBook)
			if err == nil {
				if bookData, ok := keyBookData.Data.(map[string]interface{}); ok {
					if keyPages, ok := bookData["keyPages"].([]string); ok && len(keyPages) > 0 {
						accountURLs = append(accountURLs, keyPages...)
					}
				}
			}
		}
	}

	return accountURLs, nil
}

// Helper methods for working with AccountData

// IsTokenAccount returns true if this is any type of token account.
func (ad *AccountData) IsTokenAccount() bool {
	return ad.Type == protocol.AccountTypeLiteTokenAccount || ad.Type == protocol.AccountTypeTokenAccount
}

// IsDataAccount returns true if this is any type of data account.
func (ad *AccountData) IsDataAccount() bool {
	return ad.Type == protocol.AccountTypeDataAccount || ad.Type == protocol.AccountTypeLiteDataAccount
}

// IsIdentityAccount returns true if this is an ADI (Identity) account.
func (ad *AccountData) IsIdentityAccount() bool {
	return ad.Type == protocol.AccountTypeIdentity
}

// IsKeyAccount returns true if this is a key management account.
func (ad *AccountData) IsKeyAccount() bool {
	return ad.Type == protocol.AccountTypeKeyPage || ad.Type == protocol.AccountTypeKeyBook
}

// AsLiteTokenAccount returns the account data as a LiteTokenAccount if applicable.
func (ad *AccountData) AsLiteTokenAccount() (*protocol.LiteTokenAccount, error) {
	if ad.Type != protocol.AccountTypeLiteTokenAccount {
		return nil, fmt.Errorf("account is not a lite token account")
	}

	liteToken, ok := ad.Data.(*protocol.LiteTokenAccount)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to LiteTokenAccount")
	}

	return liteToken, nil
}

// AsTokenAccount returns the account data as a TokenAccount if applicable.
func (ad *AccountData) AsTokenAccount() (*protocol.TokenAccount, error) {
	if ad.Type != protocol.AccountTypeTokenAccount {
		return nil, fmt.Errorf("account is not a token account")
	}

	token, ok := ad.Data.(*protocol.TokenAccount)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to TokenAccount")
	}

	return token, nil
}

// AsADI returns the account data as an ADI (Identity) if applicable.
func (ad *AccountData) AsADI() (*protocol.ADI, error) {
	if ad.Type != protocol.AccountTypeIdentity {
		return nil, fmt.Errorf("account is not an ADI")
	}

	adi, ok := ad.Data.(*protocol.ADI)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to ADI")
	}

	return adi, nil
}

// Data structures for account information

// TokenBalanceInfo contains balance information for token accounts
type TokenBalanceInfo struct {
	AccountURL    string `json:"accountUrl"`
	AccountType   string `json:"accountType"`
	Balance       string `json:"balance"`
	TokenURL      string `json:"tokenUrl"`
	CreditBalance uint64 `json:"creditBalance"`
}

// IdentityInfo contains information about identity accounts
type IdentityInfo struct {
	AccountURL  string `json:"accountUrl"`
	IdentityURL string `json:"identityUrl"`
	KeyBook     string `json:"keyBook"`
}

// DataAccountInfo contains information about data accounts
type DataAccountInfo struct {
	AccountURL  string `json:"accountUrl"`
	AccountType string `json:"accountType"`
	DataURL     string `json:"dataUrl"`
	KeyBook     string `json:"keyBook"`
}

// AccountSummary provides a unified view of any account type
type AccountSummary struct {
	AccountURL  string `json:"accountUrl"`
	AccountType string `json:"accountType"`
	Category    string `json:"category"`
	Balance     string `json:"balance,omitempty"`
	TokenURL    string `json:"tokenUrl,omitempty"`
	KeyBook     string `json:"keyBook,omitempty"`
}

// mapToStruct converts a map[string]interface{} to a struct using JSON marshaling/unmarshaling
func mapToStruct(data map[string]interface{}, target interface{}) error {
	// Convert map to JSON bytes
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal map to JSON: %w", err)
	}

	// Unmarshal JSON bytes into target struct
	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal JSON to struct: %w", err)
	}

	return nil
}

// GenericAccount is a wrapper for unknown or unsupported account types
type GenericAccount struct {
	AccountType protocol.AccountType
	RawData     map[string]interface{}
}

// Type returns the account type
func (g *GenericAccount) Type() protocol.AccountType {
	return g.AccountType
}

// GetUrl returns the account URL
func (g *GenericAccount) GetUrl() *url.URL {
	// Try to extract URL from raw data
	if urlStr, ok := g.RawData["url"].(string); ok {
		if u, err := url.Parse(urlStr); err == nil {
			return u
		}
	}
	return nil
}
