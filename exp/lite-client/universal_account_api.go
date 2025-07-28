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

	v2api "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
// It stores the raw data and implements the basic Account interface
type GenericAccount struct {
	AccountType protocol.AccountType
	RawData     map[string]interface{}
}

// Implement basic Account interface methods for GenericAccount
func (g *GenericAccount) Type() protocol.AccountType {
	return g.AccountType
}

func (g *GenericAccount) GetUrl() *url.URL {
	// Try to extract URL from raw data
	if urlStr, ok := g.RawData["url"].(string); ok {
		if u, err := url.Parse(urlStr); err == nil {
			return u
		}
	}
	return nil
}

// UniversalAccountAPI provides methods for querying any type of account data in the lite client.
// This replaces the limited TokenAccountAPI with support for all Accumulate account types.
type UniversalAccountAPI interface {
	// GetAccountData retrieves and interprets account data for any account type.
	// Returns the typed account struct and account type information.
	GetAccountData(ctx context.Context, accountUrl string) (*AccountData, error)

	// GetAccountType determines the type of an account without retrieving full data.
	GetAccountType(ctx context.Context, accountUrl string) (protocol.AccountType, error)
}

// AccountData holds the interpreted account data with type information.
type AccountData struct {
	URL         string                    `json:"url"`
	Type        protocol.AccountType      `json:"type"`
	TypeName    string                    `json:"typeName"`
	Data        interface{}               `json:"data"`        // The actual account struct (protocol.Account or raw data for unknown types)
	RawResponse *v2api.ChainQueryResponse `json:"rawResponse"` // Original API response
}

// Implementation of UniversalAccountAPI on LiteClient

// GetAccountData retrieves and interprets account data for any account type.
// Uses the existing v2 API with proper account type interpretation and caching.
func (c *LiteClient) GetAccountData(ctx context.Context, accountUrl string) (*AccountData, error) {
	fmt.Printf("Getting account data for %s using universal API\n", accountUrl)

	// Check cache first
	if cached, found := c.unifiedCache.GetAccountData(accountUrl); found {
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
	resp, err := c.v2.Query(ctx, query)
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

	// Convert data field to account struct based on account type
	var account interface{}
	dataMap, ok := dataField.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data field is not a map: %T", dataField)
	}

	// Create the appropriate account struct based on type
	switch accountType {
	case protocol.AccountTypeLiteTokenAccount:
		liteToken := &protocol.LiteTokenAccount{}
		if err := mapToStruct(dataMap, liteToken); err != nil {
			return nil, fmt.Errorf("failed to parse LiteTokenAccount: %w", err)
		}
		account = liteToken
	case protocol.AccountTypeTokenAccount:
		tokenAccount := &protocol.TokenAccount{}
		if err := mapToStruct(dataMap, tokenAccount); err != nil {
			return nil, fmt.Errorf("failed to parse TokenAccount: %w", err)
		}
		account = tokenAccount
	case protocol.AccountTypeDataAccount:
		dataAccount := &protocol.DataAccount{}
		if err := mapToStruct(dataMap, dataAccount); err != nil {
			return nil, fmt.Errorf("failed to parse DataAccount: %w", err)
		}
		account = dataAccount
	case protocol.AccountTypeLiteDataAccount:
		liteDataAccount := &protocol.LiteDataAccount{}
		if err := mapToStruct(dataMap, liteDataAccount); err != nil {
			return nil, fmt.Errorf("failed to parse LiteDataAccount: %w", err)
		}
		account = liteDataAccount
	case protocol.AccountTypeIdentity:
		adi := &protocol.ADI{}
		if err := mapToStruct(dataMap, adi); err != nil {
			return nil, fmt.Errorf("failed to parse ADI: %w", err)
		}
		account = adi
	case protocol.AccountTypeKeyBook:
		keyBook := &protocol.KeyBook{}
		if err := mapToStruct(dataMap, keyBook); err != nil {
			return nil, fmt.Errorf("failed to parse KeyBook: %w", err)
		}
		account = keyBook
	case protocol.AccountTypeKeyPage:
		keyPage := &protocol.KeyPage{}
		if err := mapToStruct(dataMap, keyPage); err != nil {
			return nil, fmt.Errorf("failed to parse KeyPage: %w", err)
		}
		account = keyPage
	case protocol.AccountTypeTokenIssuer:
		tokenIssuer := &protocol.TokenIssuer{}
		if err := mapToStruct(dataMap, tokenIssuer); err != nil {
			return nil, fmt.Errorf("failed to parse TokenIssuer: %w", err)
		}
		account = tokenIssuer
	case protocol.AccountTypeSystemLedger:
		systemLedger := &protocol.SystemLedger{}
		if err := mapToStruct(dataMap, systemLedger); err != nil {
			return nil, fmt.Errorf("failed to parse SystemLedger: %w", err)
		}
		account = systemLedger
	case protocol.AccountTypeSyntheticLedger:
		syntheticLedger := &protocol.SyntheticLedger{}
		if err := mapToStruct(dataMap, syntheticLedger); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticLedger: %w", err)
		}
		account = syntheticLedger
	default:
		// For unknown account types, we'll store the raw data directly
		// This handles special cases like anchorLedger that might not have dedicated structs
		// We'll create a simple wrapper that can hold any data
		genericData := make(map[string]interface{})
		for k, v := range dataMap {
			genericData[k] = v
		}
		// Store as a generic interface{} - the AccountData will handle type checking
		account = genericData
	}

	// Create a ChainQueryResponse-like structure for RawResponse
	rawResponse := &v2api.ChainQueryResponse{
		Type: accountTypeName,
		Data: account,
	}

	// Create the unified account data response
	accountData := &AccountData{
		URL:         accountUrl,
		Type:        accountType,
		TypeName:    accountTypeName,
		Data:        account,
		RawResponse: rawResponse,
	}

	// Store in cache for future requests
	c.unifiedCache.StoreAccountData(accountUrl, accountData)

	fmt.Printf("Successfully retrieved %s account: %s\n", accountData.TypeName, accountUrl)
	return accountData, nil
}

// GetAccountType determines the type of an account without retrieving full data.
// This is more efficient when you only need to know the account type.
func (c *LiteClient) GetAccountType(ctx context.Context, accountUrl string) (protocol.AccountType, error) {
	fmt.Printf("Getting account type for %s\n", accountUrl)

	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return protocol.AccountTypeUnknown, fmt.Errorf("invalid account URL: %w", err)
	}

	// Use a lightweight query to get just the type information
	query := &v2api.GeneralQuery{
		UrlQuery: v2api.UrlQuery{Url: u},
		// We could add QueryOptions here to limit the response if needed
	}

	resp, err := c.v2.Query(ctx, query)
	if err != nil {
		return protocol.AccountTypeUnknown, fmt.Errorf("failed to query account: %w", err)
	}

	// The v2 API returns a map[string]interface{} that we need to parse
	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return protocol.AccountTypeUnknown, fmt.Errorf("unexpected response type: %T", resp)
	}

	// Extract account type from response
	accountTypeName, ok := respMap["type"].(string)
	if !ok {
		return protocol.AccountTypeUnknown, fmt.Errorf("missing or invalid account type in response")
	}

	accountType, ok := protocol.AccountTypeByName(accountTypeName)
	if !ok {
		return protocol.AccountTypeUnknown, fmt.Errorf("unknown account type '%s'", accountTypeName)
	}

	fmt.Printf("Account %s is type: %s (%d)\n", accountUrl, accountTypeName, accountType)
	return accountType, nil
}

// Helper methods for working with AccountData

// IsTokenAccount returns true if this is any type of token account.
func (ad *AccountData) IsTokenAccount() bool {
	return ad.Type == protocol.AccountTypeLiteTokenAccount ||
		ad.Type == protocol.AccountTypeTokenAccount
}

// IsDataAccount returns true if this is any type of data account.
func (ad *AccountData) IsDataAccount() bool {
	return ad.Type == protocol.AccountTypeDataAccount ||
		ad.Type == protocol.AccountTypeLiteDataAccount
}

// IsIdentityAccount returns true if this is an ADI (Identity) account.
func (ad *AccountData) IsIdentityAccount() bool {
	return ad.Type == protocol.AccountTypeIdentity
}

// IsKeyAccount returns true if this is a key management account.
func (ad *AccountData) IsKeyAccount() bool {
	return ad.Type == protocol.AccountTypeKeyPage ||
		ad.Type == protocol.AccountTypeKeyBook
}

// AsLiteTokenAccount returns the account data as a LiteTokenAccount if applicable.
func (ad *AccountData) AsLiteTokenAccount() (*protocol.LiteTokenAccount, error) {
	if ad.Type != protocol.AccountTypeLiteTokenAccount {
		return nil, fmt.Errorf("account is not a LiteTokenAccount, it is %s", ad.TypeName)
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
		return nil, fmt.Errorf("account is not a TokenAccount, it is %s", ad.TypeName)
	}

	tokenAccount, ok := ad.Data.(*protocol.TokenAccount)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to TokenAccount")
	}

	return tokenAccount, nil
}

// AsKeyPage returns the account data as a KeyPage if applicable.
func (ad *AccountData) AsKeyPage() (*protocol.KeyPage, error) {
	if ad.Type != protocol.AccountTypeKeyPage {
		return nil, fmt.Errorf("account is not a KeyPage, it is %s", ad.TypeName)
	}

	keyPage, ok := ad.Data.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to KeyPage")
	}

	return keyPage, nil
}

// AsDataAccount returns the account data as a DataAccount if applicable.
func (ad *AccountData) AsDataAccount() (*protocol.DataAccount, error) {
	if ad.Type != protocol.AccountTypeDataAccount {
		return nil, fmt.Errorf("account is not a DataAccount, it is %s", ad.TypeName)
	}

	dataAccount, ok := ad.Data.(*protocol.DataAccount)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to DataAccount")
	}

	return dataAccount, nil
}

// AsADI returns the account data as an ADI (Identity) if applicable.
func (ad *AccountData) AsADI() (*protocol.ADI, error) {
	if ad.Type != protocol.AccountTypeIdentity {
		return nil, fmt.Errorf("account is not an ADI, it is %s", ad.TypeName)
	}

	adi, ok := ad.Data.(*protocol.ADI)
	if !ok {
		return nil, fmt.Errorf("failed to cast account data to ADI")
	}

	return adi, nil
}

// Phase 2: Account Type Detection and Routing

// AccountHandler defines the interface for handling different account types
type AccountHandler interface {
	CanHandle(accountType protocol.AccountType) bool
	GetBalance(ctx context.Context, accountData *AccountData) (interface{}, error)
	GetTransactions(ctx context.Context, accountData *AccountData, limit int) ([]interface{}, error)
	GetSpecificData(ctx context.Context, accountData *AccountData) (interface{}, error)
}

// TokenAccountHandler handles token account operations
type TokenAccountHandler struct {
	client *LiteClient
}

func (h *TokenAccountHandler) CanHandle(accountType protocol.AccountType) bool {
	return accountType == protocol.AccountTypeLiteTokenAccount || accountType == protocol.AccountTypeTokenAccount
}

func (h *TokenAccountHandler) GetBalance(ctx context.Context, accountData *AccountData) (interface{}, error) {
	if accountData.IsTokenAccount() {
		if liteToken, ok := accountData.Data.(*protocol.LiteTokenAccount); ok {
			return map[string]interface{}{
				"balance":  liteToken.Balance.String(),
				"tokenUrl": liteToken.TokenUrl.String(),
				"type":     "lite_token",
			}, nil
		}
		if tokenAccount, ok := accountData.Data.(*protocol.TokenAccount); ok {
			return map[string]interface{}{
				"balance":  tokenAccount.Balance.String(),
				"tokenUrl": tokenAccount.TokenUrl.String(),
				"type":     "token",
			}, nil
		}
	}
	return nil, fmt.Errorf("account is not a token account")
}

func (h *TokenAccountHandler) GetTransactions(ctx context.Context, accountData *AccountData, limit int) ([]interface{}, error) {
	// TODO: Implement transaction retrieval for token accounts
	// For now, return empty slice as transaction retrieval needs to be implemented
	// in the universal API architecture
	return []interface{}{}, nil
}

func (h *TokenAccountHandler) GetSpecificData(ctx context.Context, accountData *AccountData) (interface{}, error) {
	if accountData.IsTokenAccount() {
		return map[string]interface{}{
			"category":    "token",
			"accountType": accountData.Type.String(),
			"typeName":    accountData.TypeName,
			"data":        accountData.Data,
		}, nil
	}
	return nil, fmt.Errorf("account is not a token account")
}

// DataAccountHandler handles data account operations
type DataAccountHandler struct {
	client *LiteClient
}

func (h *DataAccountHandler) CanHandle(accountType protocol.AccountType) bool {
	return accountType == protocol.AccountTypeDataAccount || accountType == protocol.AccountTypeLiteDataAccount
}

func (h *DataAccountHandler) GetBalance(ctx context.Context, accountData *AccountData) (interface{}, error) {
	return map[string]interface{}{
		"message":     "Data accounts do not have balances",
		"type":        "data",
		"accountType": accountData.Type.String(),
	}, nil
}

func (h *DataAccountHandler) GetTransactions(ctx context.Context, accountData *AccountData, limit int) ([]interface{}, error) {
	// TODO: Implement transaction retrieval for data accounts
	// For now, return empty slice as transaction retrieval needs to be implemented
	// in the universal API architecture
	return []interface{}{}, nil
}

func (h *DataAccountHandler) GetSpecificData(ctx context.Context, accountData *AccountData) (interface{}, error) {
	if accountData.IsDataAccount() {
		if dataAccount, ok := accountData.Data.(*protocol.DataAccount); ok {
			return map[string]interface{}{
				"category":    "data",
				"accountType": accountData.Type.String(),
				"url":         dataAccount.Url.String(),
				"data":        dataAccount,
			}, nil
		}
		if liteDataAccount, ok := accountData.Data.(*protocol.LiteDataAccount); ok {
			return map[string]interface{}{
				"category":    "lite_data",
				"accountType": accountData.Type.String(),
				"data":        liteDataAccount,
			}, nil
		}
	}
	return nil, fmt.Errorf("account is not a data account")
}

// IdentityAccountHandler handles identity (ADI) account operations
type IdentityAccountHandler struct {
	client *LiteClient
}

func (h *IdentityAccountHandler) CanHandle(accountType protocol.AccountType) bool {
	return accountType == protocol.AccountTypeIdentity
}

func (h *IdentityAccountHandler) GetBalance(ctx context.Context, accountData *AccountData) (interface{}, error) {
	return map[string]interface{}{
		"message":     "Identity accounts do not have balances",
		"type":        "identity",
		"accountType": accountData.Type.String(),
	}, nil
}

func (h *IdentityAccountHandler) GetTransactions(ctx context.Context, accountData *AccountData, limit int) ([]interface{}, error) {
	// TODO: Implement transaction retrieval for identity accounts
	// For now, return empty slice as transaction retrieval needs to be implemented
	// in the universal API architecture
	return []interface{}{}, nil
}

func (h *IdentityAccountHandler) GetSpecificData(ctx context.Context, accountData *AccountData) (interface{}, error) {
	if accountData.IsIdentityAccount() {
		if identity, ok := accountData.Data.(*protocol.ADI); ok {
			return map[string]interface{}{
				"category":    "identity",
				"accountType": accountData.Type.String(),
				"url":         identity.Url.String(),
				"data":        identity,
			}, nil
		}
	}
	return nil, fmt.Errorf("account is not an identity account")
}

// AccountRouter manages different account handlers
type AccountRouter struct {
	handlers []AccountHandler
}

// NewAccountRouter creates a new account router with default handlers
func (c *LiteClient) NewAccountRouter() *AccountRouter {
	return &AccountRouter{
		handlers: []AccountHandler{
			&TokenAccountHandler{client: c},
			&DataAccountHandler{client: c},
			&IdentityAccountHandler{client: c},
		},
	}
}

// GetHandler returns the appropriate handler for the given account type
func (r *AccountRouter) GetHandler(accountType protocol.AccountType) AccountHandler {
	for _, handler := range r.handlers {
		if handler.CanHandle(accountType) {
			return handler
		}
	}
	return nil
}

// RouteAccountOperation routes an operation to the appropriate handler
func (c *LiteClient) RouteAccountOperation(ctx context.Context, accountURL string, operation string, params map[string]interface{}) (interface{}, error) {
	// Get account data first
	accountData, err := c.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	// Get appropriate handler
	router := c.NewAccountRouter()
	handler := router.GetHandler(accountData.Type)
	if handler == nil {
		return nil, fmt.Errorf("no handler available for account type: %s", accountData.Type.String())
	}

	// Route the operation
	switch operation {
	case "balance":
		return handler.GetBalance(ctx, accountData)
	case "transactions":
		limit := 100 // default
		if l, ok := params["limit"].(int); ok {
			limit = l
		}
		return handler.GetTransactions(ctx, accountData, limit)
	case "specific":
		return handler.GetSpecificData(ctx, accountData)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// Phase 3: Type-specific data access methods

// GetTokenBalance retrieves the balance for any token account type
func (c *LiteClient) GetTokenBalance(ctx context.Context, accountURL string) (*TokenBalanceInfo, error) {
	// Check cache first
	if cached, found := c.unifiedCache.GetBalance(accountURL); found {
		return cached, nil
	}

	accountData, err := c.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	if !accountData.IsTokenAccount() {
		return nil, fmt.Errorf("account is not a token account: %s", accountData.TypeName)
	}

	switch accountData.Type {
	case protocol.AccountTypeLiteTokenAccount:
		liteToken, err := accountData.AsLiteTokenAccount()
		if err != nil {
			return nil, err
		}
		balanceInfo := &TokenBalanceInfo{
			AccountURL:    accountURL,
			AccountType:   "lite_token",
			Balance:       liteToken.Balance.String(),
			TokenURL:      liteToken.TokenUrl.String(),
			CreditBalance: 0, // LiteTokenAccount doesn't have CreditBalance field
		}
		// Store in cache
		c.unifiedCache.StoreBalance(accountURL, balanceInfo)
		return balanceInfo, nil

	case protocol.AccountTypeTokenAccount:
		tokenAccount, err := accountData.AsTokenAccount()
		if err != nil {
			return nil, err
		}
		balanceInfo := &TokenBalanceInfo{
			AccountURL:    accountURL,
			AccountType:   "token",
			Balance:       tokenAccount.Balance.String(),
			TokenURL:      tokenAccount.TokenUrl.String(),
			CreditBalance: 0, // TokenAccount doesn't have CreditBalance field
		}
		// Store in cache
		c.unifiedCache.StoreBalance(accountURL, balanceInfo)
		return balanceInfo, nil

	default:
		return nil, fmt.Errorf("unsupported token account type: %s", accountData.TypeName)
	}
}

// GetIdentityInfo retrieves information about an identity (ADI) account
func (c *LiteClient) GetIdentityInfo(ctx context.Context, accountURL string) (*IdentityInfo, error) {
	// Check cache first
	if cached, found := c.unifiedCache.GetIdentityInfo(accountURL); found {
		return cached, nil
	}

	accountData, err := c.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	if !accountData.IsIdentityAccount() {
		return nil, fmt.Errorf("account is not an identity account: %s", accountData.TypeName)
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
	c.unifiedCache.StoreIdentityInfo(accountURL, identityInfo)
	return identityInfo, nil
}

// GetDataAccountInfo retrieves information about a data account
func (c *LiteClient) GetDataAccountInfo(ctx context.Context, accountURL string) (*DataAccountInfo, error) {
	// Check cache first
	if cached, found := c.unifiedCache.GetDataAccountInfo(accountURL); found {
		return cached, nil
	}

	accountData, err := c.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	if !accountData.IsDataAccount() {
		return nil, fmt.Errorf("account is not a data account: %s", accountData.TypeName)
	}

	switch accountData.Type {
	case protocol.AccountTypeDataAccount:
		dataAccount, err := accountData.AsDataAccount()
		if err != nil {
			return nil, err
		}
		dataInfo := &DataAccountInfo{
			AccountURL:  accountURL,
			AccountType: "data",
			DataURL:     dataAccount.Url.String(),
			KeyBook:     dataAccount.KeyBook().String(),
		}
		// Store in cache
		c.unifiedCache.StoreDataAccountInfo(accountURL, dataInfo)
		return dataInfo, nil

	case protocol.AccountTypeLiteDataAccount:
		// LiteDataAccount doesn't have the same structure, handle differently
		dataInfo := &DataAccountInfo{
			AccountURL:  accountURL,
			AccountType: "lite_data",
			DataURL:     accountURL, // For lite data accounts, the URL is the data URL
			KeyBook:     "",         // Lite data accounts don't have key books
		}
		// Store in cache
		c.unifiedCache.StoreDataAccountInfo(accountURL, dataInfo)
		return dataInfo, nil

	default:
		return nil, fmt.Errorf("unsupported data account type: %s", accountData.TypeName)
	}
}

// GetAccountSummary provides a unified summary for any account type
func (c *LiteClient) GetAccountSummary(ctx context.Context, accountURL string) (*AccountSummary, error) {
	// Check cache first
	if cached, found := c.unifiedCache.GetAccountSummary(accountURL); found {
		return cached, nil
	}

	accountData, err := c.GetAccountData(ctx, accountURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get account data: %w", err)
	}

	summary := &AccountSummary{
		AccountURL:  accountURL,
		AccountType: accountData.TypeName,
		Category:    getAccountCategory(accountData.Type),
	}

	// Add type-specific information
	switch {
	case accountData.IsTokenAccount():
		balanceInfo, err := c.GetTokenBalance(ctx, accountURL)
		if err == nil {
			summary.Balance = balanceInfo.Balance
			summary.TokenURL = balanceInfo.TokenURL
		}

	case accountData.IsIdentityAccount():
		identityInfo, err := c.GetIdentityInfo(ctx, accountURL)
		if err == nil {
			summary.KeyBook = identityInfo.KeyBook
		}

	case accountData.IsDataAccount():
		dataInfo, err := c.GetDataAccountInfo(ctx, accountURL)
		if err == nil {
			summary.KeyBook = dataInfo.KeyBook
		}
	}

	// Store in cache
	c.unifiedCache.StoreAccountSummary(accountURL, summary)
	return summary, nil
}

// Helper function to categorize account types
func getAccountCategory(accountType protocol.AccountType) string {
	switch accountType {
	case protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount:
		return "token"
	case protocol.AccountTypeDataAccount, protocol.AccountTypeLiteDataAccount:
		return "data"
	case protocol.AccountTypeIdentity:
		return "identity"
	case protocol.AccountTypeKeyPage, protocol.AccountTypeKeyBook:
		return "key"
	default:
		return "unknown"
	}
}

// Data structures for Phase 3 methods

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
