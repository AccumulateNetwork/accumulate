// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRequestBodySizeLimit verifies that oversized requests are rejected
func TestRequestBodySizeLimit(t *testing.T) {
	// Create a JrpcMethods instance
	jrpc, err := NewJrpc(Options{})
	require.NoError(t, err)

	// Create a test router
	router := jrpc.NewMux()

	t.Run("Normal request succeeds", func(t *testing.T) {
		// Create a normal-sized request
		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "version",
			"id":      1,
		}
		reqBytes, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/v2", bytes.NewReader(reqBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should succeed (200 or similar, not 413)
		require.NotEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	})

	t.Run("Oversized request rejected", func(t *testing.T) {
		// Create an oversized request (larger than MaxRequestBodySize)
		largeBody := bytes.Repeat([]byte("x"), MaxRequestBodySize+1)

		req := httptest.NewRequest("POST", "/v2", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// The jsonrpc2 library may return 200 with an error in the response body
		// Check for either 413 or an error in the response
		if w.Code != http.StatusRequestEntityTooLarge {
			// Check if the response contains an error about size
			body := w.Body.String()
			require.Contains(t, body, "too large", "Expected error about request size")
		}
	})

	t.Run("Oversized request rejected on GET endpoints", func(t *testing.T) {
		// Test that GET endpoints also enforce size limits
		largeBody := bytes.Repeat([]byte("x"), MaxRequestBodySize+1)

		req := httptest.NewRequest("GET", "/status", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should return 413 Request Entity Too Large
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

		// Should return JSON error
		var errorResp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &errorResp)
		require.NoError(t, err)
		require.Contains(t, errorResp, "error")
	})

	t.Run("Large but acceptable request succeeds", func(t *testing.T) {
		// Create a large request that's just under the limit
		largeButOkBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "version",
			"id":      1,
			"params": map[string]string{
				"data": strings.Repeat("x", MaxRequestBodySize-1000),
			},
		}
		reqBytes, err := json.Marshal(largeButOkBody)
		require.NoError(t, err)

		// Ensure we're under the limit
		require.Less(t, len(reqBytes), MaxRequestBodySize)

		req := httptest.NewRequest("POST", "/v2", bytes.NewReader(reqBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should succeed (not 413)
		require.NotEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	})
}

// TestMaxRequestBodySize verifies the constant is set to a reasonable value
func TestMaxRequestBodySize(t *testing.T) {
	// Verify the constant is 10MB
	require.Equal(t, int64(10*1024*1024), int64(MaxRequestBodySize))
}

// TestRequestBodySizeLimitErrorMessage verifies error messages are appropriate
func TestRequestBodySizeLimitErrorMessage(t *testing.T) {
	jrpc, err := NewJrpc(Options{})
	require.NoError(t, err)

	router := jrpc.NewMux()

	// Create an oversized request
	largeBody := bytes.Repeat([]byte("x"), MaxRequestBodySize+1)

	req := httptest.NewRequest("GET", "/status", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify the response
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	// Verify error message format
	body, err := io.ReadAll(w.Body)
	require.NoError(t, err)

	var errorResp map[string]interface{}
	err = json.Unmarshal(body, &errorResp)
	require.NoError(t, err)

	// Check error structure
	require.Contains(t, errorResp, "error")
	errorObj := errorResp["error"].(map[string]interface{})
	require.Contains(t, errorObj, "code")
	require.Contains(t, errorObj, "message")

	// Verify error message mentions size limit
	message := errorObj["message"].(string)
	require.Contains(t, message, "Request body too large")
	require.Contains(t, message, "10485760") // 10MB in bytes
}
