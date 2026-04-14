// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

func TestCLIValidate_ValidCounts(t *testing.T) {
	for _, count := range []string{"4", "8", "16", "32", "64", "128", "256"} {
		out, err := runExecutorConfigValidate(nil, []string{count})
		require.NoError(t, err, "count=%s", count)
		assert.Contains(t, out, "Valid")
		assert.NotContains(t, out, "Invalid")
	}
}

func TestCLIValidate_InvalidCounts(t *testing.T) {
	for _, count := range []string{"0", "1", "3", "5", "10", "48", "512"} {
		out, err := runExecutorConfigValidate(nil, []string{count})
		require.NoError(t, err, "count=%s", count)
		assert.Contains(t, out, "Invalid")
	}
}

func TestCLIValidate_BadInput(t *testing.T) {
	_, err := runExecutorConfigValidate(nil, []string{"abc"})
	assert.Error(t, err)
}

func TestCLIValidate_JSON(t *testing.T) {
	flagExecutorConfig.JSON = true
	defer func() { flagExecutorConfig.JSON = false }()

	out, err := runExecutorConfigValidate(nil, []string{"64"})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, true, result["valid"])
	assert.Equal(t, float64(64), result["shardCount"])
	assert.Equal(t, float64(6), result["depth"])
}

func TestCLIValidate_JSON_Invalid(t *testing.T) {
	flagExecutorConfig.JSON = true
	defer func() { flagExecutorConfig.JSON = false }()

	out, err := runExecutorConfigValidate(nil, []string{"5"})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, false, result["valid"])
	assert.NotEmpty(t, result["error"])
}

// TestCLIGetAndSet tests the CLI get and set commands against a mock API server.
func TestCLIGetAndSet(t *testing.T) {
	// Set up a real API handler backed by an in-memory database.
	db := database.OpenInMemory(nil)
	handler := api.NewExecutorConfigHandler(db)
	router := httprouter.New()
	handler.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()

	// Point the CLI at our test server.
	flagExecutorConfig.Endpoint = server.URL

	// GET default
	out, err := runExecutorConfigGet(&cobra.Command{}, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "64") // default shard count

	// SET to 32
	out, err = runExecutorConfigSet(&cobra.Command{}, []string{"32"})
	require.NoError(t, err)
	assert.Contains(t, out, "32")

	// GET after set
	out, err = runExecutorConfigGet(&cobra.Command{}, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "32")

	// SET invalid
	_, err = runExecutorConfigSet(&cobra.Command{}, []string{"5"})
	assert.Error(t, err)
}

// TestCLIGet_JSON tests JSON output mode for get command.
func TestCLIGet_JSON(t *testing.T) {
	db := database.OpenInMemory(nil)
	handler := api.NewExecutorConfigHandler(db)
	router := httprouter.New()
	handler.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()

	flagExecutorConfig.Endpoint = server.URL
	flagExecutorConfig.JSON = true
	defer func() { flagExecutorConfig.JSON = false }()

	out, err := runExecutorConfigGet(&cobra.Command{}, nil)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(64), result["shardCount"])
}

// TestCLISet_JSON tests JSON output mode for set command.
func TestCLISet_JSON(t *testing.T) {
	db := database.OpenInMemory(nil)
	handler := api.NewExecutorConfigHandler(db)
	router := httprouter.New()
	handler.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()

	flagExecutorConfig.Endpoint = server.URL
	flagExecutorConfig.JSON = true
	defer func() { flagExecutorConfig.JSON = false }()

	out, err := runExecutorConfigSet(&cobra.Command{}, []string{"128"})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(128), result["shardCount"])
	assert.Equal(t, "updated", result["status"])
}

// TestCLISet_ConnectionError tests error handling when node is unreachable.
func TestCLISet_ConnectionError(t *testing.T) {
	flagExecutorConfig.Endpoint = "http://localhost:1" // unlikely to be running
	_, err := runExecutorConfigSet(&cobra.Command{}, []string{"32"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

// TestCLIGet_ConnectionError tests error handling when node is unreachable.
func TestCLIGet_ConnectionError(t *testing.T) {
	flagExecutorConfig.Endpoint = "http://localhost:1"
	_, err := runExecutorConfigGet(&cobra.Command{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

// TestCLISet_ServerError tests handling of non-200 responses.
func TestCLISet_ServerError(t *testing.T) {
	// Create a server that always returns 500.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	flagExecutorConfig.Endpoint = server.URL
	_, err := runExecutorConfigSet(&cobra.Command{}, []string{"32"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node returned error")
}
