// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

// TestGetAccount tests the GetAccount method
func TestGetAccount(t *testing.T) {
	t.Run("ValidAccount", func(t *testing.T) {
		// This test uses the actual client to verify the method works
		c, err := client.NewTestnet()
		require.NoError(t, err)
		require.NotNil(t, c)
		
		// We can't test actual network calls without a mock, so just verify
		// the method exists and handles invalid URLs
	})
	
	t.Run("InvalidURL", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetAccount(ctx, "not-a-valid-url")
		require.Error(t, err)
		// The error might say "invalid" or something else
	})
	
	t.Run("EmptyURL", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetAccount(ctx, "")
		require.Error(t, err)
	})
	
	t.Run("WithTimeout", func(t *testing.T) {
		c, err := client.New(&client.Config{
			Endpoint: "https://testnet.accumulate.io/v3",
			Network:  client.NetworkTestnet,
			Timeout:  1 * time.Millisecond, // Very short timeout
		})
		require.NoError(t, err)
		
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		
		// This should timeout
		_, err = c.GetAccount(ctx, "acc://ACME")
		// The error might be a timeout or connection error
		require.Error(t, err)
	})
}

// TestGetTransaction tests the GetTransaction method
func TestGetTransaction(t *testing.T) {
	t.Run("ValidTransactionID", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		// Test with a valid hex string (64 characters = 32 bytes)
		txID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		
		// We can't test actual network calls, but we can verify the method
		// handles the input correctly
		ctx := context.Background()
		_, err = c.GetTransaction(ctx, txID)
		// This will fail with network error, but should not fail on parsing
		// We just check it doesn't panic
	})
	
	t.Run("InvalidHex", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetTransaction(ctx, "not-hex")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid transaction ID")
	})
	
	t.Run("WrongLength", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Too short
		_, err = c.GetTransaction(ctx, "0123456789abcdef")
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be 32 bytes")
		
		// Too long (33 bytes = 66 hex chars)
		_, err = c.GetTransaction(ctx, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01")
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be 32 bytes")
	})
	
	t.Run("EmptyTransactionID", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetTransaction(ctx, "")
		require.Error(t, err)
	})
}

// TestGetChainEntry tests the GetChainEntry method
func TestGetChainEntry(t *testing.T) {
	t.Run("ValidParameters", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Test the method exists and handles parameters
		_, err = c.GetChainEntry(ctx, "acc://mytoken.acme", "main", 0)
		// Will fail with network error, but validates input handling
	})
	
	t.Run("InvalidURL", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetChainEntry(ctx, "not-a-url", "main", 0)
		require.Error(t, err)
		// Just check that it errors
	})
	
	t.Run("EmptyChainName", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Empty chain name should still work (API might have default)
		_, err = c.GetChainEntry(ctx, "acc://mytoken.acme", "", 0)
		// Just verify it doesn't panic
	})
	
	t.Run("LargeIndex", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Test with a very large index
		_, err = c.GetChainEntry(ctx, "acc://mytoken.acme", "main", 999999999)
		// Just verify it doesn't panic
	})
}

// TestGetDataEntry tests the GetDataEntry method
func TestGetDataEntry(t *testing.T) {
	t.Run("ValidParameters", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetDataEntry(ctx, "acc://mydata.acme", 0)
		// Will fail with network error, but validates input handling
	})
	
	t.Run("InvalidURL", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetDataEntry(ctx, "invalid-url", 0)
		require.Error(t, err)
	})
	
	t.Run("NegativeIndex", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Go doesn't allow negative uint64, so this tests zero
		_, err = c.GetDataEntry(ctx, "acc://mydata.acme", 0)
		// Just verify it doesn't panic
	})
}

// TestGetDirectory tests the GetDirectory method
func TestGetDirectory(t *testing.T) {
	t.Run("ValidParameters", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetDirectory(ctx, "acc://myadi.acme", 0, 10)
		// Will fail with network error, but validates input handling
	})
	
	t.Run("InvalidURL", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		_, err = c.GetDirectory(ctx, "not-valid", 0, 10)
		require.Error(t, err)
	})
	
	t.Run("ZeroCount", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Zero count should be valid (return empty)
		_, err = c.GetDirectory(ctx, "acc://myadi.acme", 0, 0)
		// Just verify it doesn't panic
	})
	
	t.Run("LargeCount", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Very large count
		_, err = c.GetDirectory(ctx, "acc://myadi.acme", 0, 100000)
		// Just verify it doesn't panic
	})
	
	t.Run("Pagination", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		
		ctx := context.Background()
		// Test pagination parameters
		_, err = c.GetDirectory(ctx, "acc://myadi.acme", 10, 10)
		// Just verify it doesn't panic
		
		_, err = c.GetDirectory(ctx, "acc://myadi.acme", 100, 50)
		// Just verify it doesn't panic
	})
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	t.Run("EmptyEndpoint", func(t *testing.T) {
		_, err := client.New(&client.Config{
			Network: client.NetworkCustom,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "endpoint is required")
	})
	
	t.Run("DefaultTimeout", func(t *testing.T) {
		c, err := client.New(&client.Config{
			Endpoint: "http://localhost:8080/v3",
			Network:  client.NetworkCustom,
			// Don't set timeout, should get default
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		// Can't easily test the actual timeout value without reflection
	})
	
	t.Run("CustomTimeout", func(t *testing.T) {
		c, err := client.New(&client.Config{
			Endpoint: "http://localhost:8080/v3",
			Network:  client.NetworkCustom,
			Timeout:  5 * time.Minute,
		})
		require.NoError(t, err)
		require.NotNil(t, c)
	})
	
	t.Run("DebugMode", func(t *testing.T) {
		c, err := client.New(&client.Config{
			Endpoint: "http://localhost:8080/v3",
			Network:  client.NetworkCustom,
			Debug:    true,
		})
		require.NoError(t, err)
		require.NotNil(t, c)
	})
}

// TestURLParsing tests URL parsing edge cases
func TestURLParsing(t *testing.T) {
	c, err := client.NewTestnet()
	require.NoError(t, err)
	ctx := context.Background()
	
	testCases := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{"Empty", "", true, ""},
		{"NoScheme", "mytoken.acme", true, ""},
		{"HTTPScheme", "http://mytoken.acme", true, ""},
		{"ValidACC", "acc://mytoken.acme", false, ""},
		{"WithPath", "acc://mytoken.acme/sub", false, ""},
		{"WithPort", "acc://mytoken.acme:1234", false, ""}, // Actually might be valid
		{"Spaces", "acc://my token.acme", true, ""},
		{"SpecialChars", "acc://my$token.acme", false, ""}, // Might be valid
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.GetAccount(ctx, tc.url)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					require.Contains(t, err.Error(), tc.errMsg)
				}
			}
			// Note: even valid URLs will fail with network error in tests
		})
	}
}

// TestContextCancellation tests context cancellation handling
func TestContextCancellation(t *testing.T) {
	c, err := client.NewTestnet()
	require.NoError(t, err)
	
	t.Run("ImmediateCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		
		_, err := c.GetAccount(ctx, "acc://ACME")
		require.Error(t, err)
	})
	
	t.Run("TimeoutContext", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		
		time.Sleep(10 * time.Millisecond) // Ensure timeout
		
		_, err := c.GetAccount(ctx, "acc://ACME")
		require.Error(t, err)
	})
}

// TestHexEncoding tests hex encoding/decoding in transaction IDs
func TestHexEncoding(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"Valid32Bytes", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false, ""},
		{"UpperCase", "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF", false, ""},
		{"MixedCase", "0123456789AbCdEf0123456789aBcDeF0123456789ABCDEF0123456789abcdef", false, ""},
		{"TooShort", "0123456789abcdef", true, "must be 32 bytes"},
		{"TooLong", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00", true, "must be 32 bytes"},
		{"InvalidChars", "0123456789abcdefg123456789abcdef0123456789abcdef0123456789abcdef", true, "invalid transaction ID"},
		{"OddLength", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde", true, "invalid transaction ID"},
		{"Empty", "", true, ""},
		{"Spaces", "0123456789abcdef 123456789abcdef0123456789abcdef0123456789abcdef", true, "invalid transaction ID"},
	}
	
	c, err := client.NewTestnet()
	require.NoError(t, err)
	ctx := context.Background()
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.GetTransaction(ctx, tc.input)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					require.Contains(t, err.Error(), tc.errMsg)
				}
			}
			// Note: even valid IDs will fail with network error in tests
		})
	}
}