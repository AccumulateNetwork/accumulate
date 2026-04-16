// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
)

// ExecutorConfigResponse is the JSON response for GET /executor/config.
type ExecutorConfigResponse struct {
	ShardCount uint64   `json:"shardCount"`
	Depth      int      `json:"depth"`
	MinShards  uint64   `json:"minShards"`
	MaxShards  uint64   `json:"maxShards"`
	ValidRange []uint64 `json:"validRange"`
}

// ExecutorConfigRequest is the JSON request body for POST /executor/config.
type ExecutorConfigRequest struct {
	ShardCount uint64 `json:"shardCount"`
}

// ExecutorConfigHandler provides HTTP handlers for executor shard configuration.
type ExecutorConfigHandler struct {
	db *database.Database
}

// NewExecutorConfigHandler creates a new handler backed by the given database.
func NewExecutorConfigHandler(db *database.Database) *ExecutorConfigHandler {
	return &ExecutorConfigHandler{db: db}
}

// Register adds the executor config endpoints to the router.
func (h *ExecutorConfigHandler) Register(r *httprouter.Router) {
	r.GET("/executor/config", h.handleGet)
	r.POST("/executor/config", h.handlePost)
}

func (h *ExecutorConfigHandler) handleGet(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	batch := h.db.Begin(false)
	defer batch.Discard()

	cfg, err := database.GetExecutorConfig(batch)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read config: "+err.Error())
		return
	}

	resp := ExecutorConfigResponse{
		ShardCount: cfg.ExecutorShardCount,
		Depth:      bits.TrailingZeros64(cfg.ExecutorShardCount),
		MinShards:  4,
		MaxShards:  256,
		ValidRange: []uint64{4, 8, 16, 32, 64, 128, 256},
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *ExecutorConfigHandler) handlePost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req ExecutorConfigRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := database.ValidateShardCount(req.ShardCount); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	batch := h.db.Begin(true)
	defer batch.Discard()

	if err := database.PutExecutorConfig(batch, &database.ExecutorConfig{
		ExecutorShardCount: req.ShardCount,
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to store config: "+err.Error())
		return
	}

	if err := batch.Commit(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to commit: "+err.Error())
		return
	}

	// Update Prometheus metric.
	metrics.ExecutorShardCount.Set(float64(req.ShardCount))

	resp := ExecutorConfigResponse{
		ShardCount: req.ShardCount,
		Depth:      bits.TrailingZeros64(req.ShardCount),
		MinShards:  4,
		MaxShards:  256,
		ValidRange: []uint64{4, 8, 16, 32, 64, 128, 256},
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// FormatShardCountInfo returns a human-readable summary for the given shard count.
func FormatShardCountInfo(count uint64) string {
	depth := bits.TrailingZeros64(count)
	return fmt.Sprintf("Shard count: %d (depth: %d, range: 4-256, must be power of 2)", count, depth)
}
