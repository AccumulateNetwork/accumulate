// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

func setupTestHandler(t *testing.T) (*httprouter.Router, *database.Database) {
	t.Helper()
	db := database.OpenInMemory(nil)
	handler := NewExecutorConfigHandler(db)
	router := httprouter.New()
	handler.Register(router)
	return router, db
}

func TestGetExecutorConfig_Default(t *testing.T) {
	router, _ := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/executor/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ExecutorConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint64(database.DefaultExecutorShardCount), resp.ShardCount)
	assert.Equal(t, uint64(4), resp.MinShards)
	assert.Equal(t, uint64(256), resp.MaxShards)
	assert.Len(t, resp.ValidRange, 7)
}

func TestPostExecutorConfig_Valid(t *testing.T) {
	router, _ := setupTestHandler(t)

	for _, count := range []uint64{4, 8, 16, 32, 64, 128, 256} {
		body, _ := json.Marshal(ExecutorConfigRequest{ShardCount: count})
		req := httptest.NewRequest(http.MethodPost, "/executor/config", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "count=%d", count)

		var resp ExecutorConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, count, resp.ShardCount)
	}
}

func TestPostExecutorConfig_InvalidValues(t *testing.T) {
	router, _ := setupTestHandler(t)

	for _, count := range []uint64{0, 1, 3, 5, 10, 48, 512} {
		body, _ := json.Marshal(ExecutorConfigRequest{ShardCount: count})
		req := httptest.NewRequest(http.MethodPost, "/executor/config", bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "count=%d should be rejected", count)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["error"])
	}
}

func TestPostExecutorConfig_InvalidJSON(t *testing.T) {
	router, _ := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/executor/config", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPostThenGetExecutorConfig(t *testing.T) {
	router, _ := setupTestHandler(t)

	// Set to 32
	body, _ := json.Marshal(ExecutorConfigRequest{ShardCount: 32})
	req := httptest.NewRequest(http.MethodPost, "/executor/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Read back
	req = httptest.NewRequest(http.MethodGet, "/executor/config", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ExecutorConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint64(32), resp.ShardCount)
	assert.Equal(t, 5, resp.Depth) // log2(32) = 5
}

func TestGetExecutorConfig_ResponseFormat(t *testing.T) {
	router, _ := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/executor/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

	// Verify all expected fields exist
	assert.Contains(t, raw, "shardCount")
	assert.Contains(t, raw, "depth")
	assert.Contains(t, raw, "minShards")
	assert.Contains(t, raw, "maxShards")
	assert.Contains(t, raw, "validRange")
}

func TestFormatShardCountInfo(t *testing.T) {
	info := FormatShardCountInfo(64)
	assert.Contains(t, info, "64")
	assert.Contains(t, info, "depth: 6")
}
