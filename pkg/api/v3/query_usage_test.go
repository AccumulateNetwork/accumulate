// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MockQuerier implements the api.Querier interface for testing
type MockQuerier struct {
	responses map[string]api.Record
}

func NewMockQuerier() *MockQuerier {
	return &MockQuerier{
		responses: make(map[string]api.Record),
	}
}

func (m *MockQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	// Use scope URL as key for mock responses
	key := scope.String()
	if record, exists := m.responses[key]; exists {
		return record, nil
	}
	
	// Return a default error record if no mock response is set
	return &api.ErrorRecord{
		Value: &errors.Error{
			Code:    errors.NotFound,
			Message: "account not found",
		},
	}, nil
}

func (m *MockQuerier) SetResponse(scope string, record api.Record) {
	m.responses[scope] = record
}

// TestQueryACMETokenIssuedValues demonstrates how to query the ACME token issuer
// to retrieve the total issued token amount using the API v3 query system.
func TestQueryACMETokenIssuedValues(t *testing.T) {
	// Create a mock querier for testing
	querier := NewMockQuerier()
	
	// Set up the ACME token issuer scope URL
	// The ACME token issuer is typically located at acc://ACME
	acmeTokenURL, err := url.Parse("acc://ACME")
	require.NoError(t, err, "Failed to parse ACME token URL")
	
	// Create a mock ACME token issuer account with issued values
	issuedAmount := big.NewInt(500000000) // 500 million ACME tokens (with 8 decimal precision)
	issuedAmount.Mul(issuedAmount, big.NewInt(100000000)) // 500,000,000.00000000 ACME
	
	mockTokenIssuer := &protocol.TokenIssuer{
		Url:       acmeTokenURL,
		Symbol:    "ACME",
		Precision: 8,
		Issued:    *issuedAmount,
	}
	
	// Create an AccountRecord containing the TokenIssuer
	mockAccountRecord := &api.AccountRecord{
		Account: mockTokenIssuer,
	}
	
	// Set up the mock response for the ACME token URL
	querier.SetResponse(acmeTokenURL.String(), mockAccountRecord)
	
	// Create a DefaultQuery to retrieve the account state
	query := &api.DefaultQuery{
		// IncludeReceipt can be set to get additional verification data
		IncludeReceipt: nil, // Not needed for basic account queries
	}
	
	// Execute the query using the scope and query
	ctx := context.Background()
	record, err := querier.Query(ctx, acmeTokenURL, query)
	require.NoError(t, err, "Failed to execute query for ACME token issuer")
	
	// Verify we got an AccountRecord back
	accountRecord, ok := record.(*api.AccountRecord)
	require.True(t, ok, "Expected AccountRecord, got %T", record)
	require.NotNil(t, accountRecord.Account, "Account should not be nil")
	
	// Verify the account is a TokenIssuer
	tokenIssuer, ok := accountRecord.Account.(*protocol.TokenIssuer)
	require.True(t, ok, "Expected TokenIssuer account, got %T", accountRecord.Account)
	
	// Verify the token issuer properties
	require.Equal(t, "ACME", tokenIssuer.Symbol, "Token symbol should be ACME")
	require.Equal(t, uint64(8), tokenIssuer.Precision, "ACME token should have 8 decimal places")
	require.Equal(t, acmeTokenURL.String(), tokenIssuer.Url.String(), "Token issuer URL should match")
	
	// Verify the issued amount - this is the key value we're querying for
	expectedIssued := big.NewInt(500000000)
	expectedIssued.Mul(expectedIssued, big.NewInt(100000000)) // 500,000,000.00000000 ACME
	require.Equal(t, expectedIssued.String(), tokenIssuer.Issued.String(), 
		"Issued amount should be 500,000,000.00000000 ACME tokens")
	
	// Demonstrate how to work with the issued amount
	// Convert to human-readable format considering precision
	precision := big.NewInt(1)
	for i := uint64(0); i < tokenIssuer.Precision; i++ {
		precision.Mul(precision, big.NewInt(10))
	}
	
	// Calculate the human-readable amount (issued / 10^precision)
	humanReadable := new(big.Int).Div(&tokenIssuer.Issued, precision)
	require.Equal(t, int64(500000000), humanReadable.Int64(), 
		"Human-readable issued amount should be 500,000,000 ACME")
	
	t.Logf("Successfully queried ACME token issuer:")
	t.Logf("  URL: %s", tokenIssuer.Url)
	t.Logf("  Symbol: %s", tokenIssuer.Symbol)
	t.Logf("  Precision: %d", tokenIssuer.Precision)
	t.Logf("  Total Issued (raw): %s", tokenIssuer.Issued.String())
	t.Logf("  Total Issued (human): %s %s", humanReadable.String(), tokenIssuer.Symbol)
}

// TestQueryScopeRouting demonstrates how different scope URLs route to different partitions
func TestQueryScopeRouting(t *testing.T) {
	querier := NewMockQuerier()
	
	// Test cases for different scope URL patterns and their expected routing behavior
	testCases := []struct {
		name        string
		scopeURL    string
		description string
	}{
		{
			name:        "ACME Token Issuer",
			scopeURL:    "acc://ACME",
			description: "System account - routes to Directory partition",
		},
		{
			name:        "Directory Node",
			scopeURL:    "acc://dn.acme",
			description: "Directory Node partition account - routes to Directory",
		},
		{
			name:        "Staking Account",
			scopeURL:    "acc://staking.acme",
			description: "Staking system account - routes to Directory partition",
		},
		{
			name:        "User Account",
			scopeURL:    "acc://alice.acme",
			description: "User account - routes to BVN partition",
		},
		{
			name:        "User Token Account",
			scopeURL:    "acc://alice.acme/tokens",
			description: "User's token account - routes to BVN partition",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the scope URL
			scopeURL, err := url.Parse(tc.scopeURL)
			require.NoError(t, err, "Failed to parse scope URL: %s", tc.scopeURL)
			
			// Create a DefaultQuery
			query := &api.DefaultQuery{}
			
			// Set up a mock response (in real usage, the router would handle partition selection)
			mockRecord := &api.ErrorRecord{
				Value: &errors.Error{
					Code:    errors.NotFound,
					Message: "mock response for routing test",
				},
			}
			querier.SetResponse(tc.scopeURL, mockRecord)
			
			// Execute the query
			ctx := context.Background()
			record, err := querier.Query(ctx, scopeURL, query)
			require.NoError(t, err, "Query should not fail for scope: %s", tc.scopeURL)
			require.NotNil(t, record, "Record should not be nil")
			
			t.Logf("Scope: %s - %s", tc.scopeURL, tc.description)
		})
	}
}

// TestQueryTypes demonstrates usage of different query types available in API v3
func TestQueryTypes(t *testing.T) {
	querier := NewMockQuerier()
	ctx := context.Background()
	
	// Test scope - using ACME token issuer
	acmeURL, err := url.Parse("acc://ACME")
	require.NoError(t, err)
	
	// Set up a mock account record
	mockRecord := &api.AccountRecord{
		Account: &protocol.TokenIssuer{
			Url:    acmeURL,
			Symbol: "ACME",
			Issued: *big.NewInt(1000000),
		},
	}
	querier.SetResponse(acmeURL.String(), mockRecord)
	
	t.Run("DefaultQuery", func(t *testing.T) {
		// DefaultQuery - Basic account/transaction state queries
		query := &api.DefaultQuery{
			IncludeReceipt: nil, // Optional receipt inclusion
		}
		
		record, err := querier.Query(ctx, acmeURL, query)
		require.NoError(t, err)
		require.NotNil(t, record)
		
		// Verify query type
		require.Equal(t, api.QueryTypeDefault, query.QueryType())
		t.Logf("DefaultQuery executed successfully for %s", acmeURL)
	})
	
	t.Run("DirectoryQuery", func(t *testing.T) {
		// DirectoryQuery - Directory listing queries with range support
		query := &api.DirectoryQuery{
			Range: &api.RangeOptions{
				Start: 0,
				Count: func() *uint64 { v := uint64(10); return &v }(),
			},
		}
		
		// Note: In real usage, this would return directory entries
		record, err := querier.Query(ctx, acmeURL, query)
		require.NoError(t, err)
		require.NotNil(t, record)
		
		require.Equal(t, api.QueryTypeDirectory, query.QueryType())
		t.Logf("DirectoryQuery executed successfully for %s", acmeURL)
	})
	
	t.Run("PendingQuery", func(t *testing.T) {
		// PendingQuery - Pending transaction queries
		query := &api.PendingQuery{
			Range: &api.RangeOptions{
				Start: 0,
				Count: func() *uint64 { v := uint64(5); return &v }(),
			},
		}
		
		record, err := querier.Query(ctx, acmeURL, query)
		require.NoError(t, err)
		require.NotNil(t, record)
		
		require.Equal(t, api.QueryTypePending, query.QueryType())
		t.Logf("PendingQuery executed successfully for %s", acmeURL)
	})
}

// TestQueryValidation demonstrates query validation and error handling
func TestQueryValidation(t *testing.T) {
	t.Run("ValidateDefaultQuery", func(t *testing.T) {
		query := &api.DefaultQuery{}
		
		// DefaultQuery should be valid without any required fields
		err := query.IsValid()
		require.NoError(t, err, "DefaultQuery should be valid")
	})
	
	t.Run("ValidateDirectoryQuery", func(t *testing.T) {
		query := &api.DirectoryQuery{
			Range: &api.RangeOptions{
				Start: 0,
				Count: func() *uint64 { v := uint64(10); return &v }(),
			},
		}
		
		// DirectoryQuery with range should be valid
		err := query.IsValid()
		require.NoError(t, err, "DirectoryQuery with range should be valid")
	})
	
	t.Run("ValidatePendingQuery", func(t *testing.T) {
		// PendingQuery requires a Range field
		query := &api.PendingQuery{
			Range: &api.RangeOptions{
				Start: 0,
				Count: func() *uint64 { v := uint64(5); return &v }(),
			},
		}
		
		err := query.IsValid()
		require.NoError(t, err, "PendingQuery with range should be valid")
	})
}
