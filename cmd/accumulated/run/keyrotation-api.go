// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/keyrotation"
)

// KeyRotationAPI handles HTTP API endpoints for key rotation operations.
type KeyRotationAPI struct {
	managers map[string]*keyrotation.Manager
	logger   *slog.Logger
}

// NewKeyRotationAPI creates a new key rotation API handler.
func NewKeyRotationAPI(inst *Instance) *KeyRotationAPI {
	api := &KeyRotationAPI{
		managers: make(map[string]*keyrotation.Manager),
		logger:   inst.logger,
	}

	// Collect all key rotation managers from services
	for _, svc := range inst.services {
		if kr, ok := svc.(*KeyRotationService); ok {
			if kr.manager != nil {
				api.managers[kr.Partition] = kr.manager
			}
		}
	}

	return api
}

// ServeHTTP implements the http.Handler interface.
func (api *KeyRotationAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/validator/key/"

	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}

	operation := strings.TrimPrefix(r.URL.Path, prefix)
	partition := r.URL.Query().Get("partition")
	if partition == "" {
		// Default to Directory partition if not specified
		partition = "Directory"
	}

	manager, ok := api.managers[partition]
	if !ok {
		http.Error(w, "key rotation not enabled for partition", http.StatusNotFound)
		return
	}

	switch operation {
	case "status":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		api.handleStatus(w, r, manager)
	case "rotate":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		api.handleRotate(w, r, manager)
	case "revoke":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		api.handleRevoke(w, r, manager)
	default:
		http.NotFound(w, r)
	}
}

// KeyStatusResponse is the response for GET /api/v1/validator/key/status
type KeyStatusResponse struct {
	KeyID               string        `json:"keyId"`
	Status              string        `json:"status"`
	PublicKey           string        `json:"publicKey"`
	ActivatedAt         time.Time     `json:"activatedAt"`
	ExpiresAt           time.Time     `json:"expiresAt"`
	GraceEndsAt         time.Time     `json:"graceEndsAt"`
	DaysUntilExpiration int           `json:"daysUntilExpiration"`
	GraceKey            *GraceKeyInfo `json:"graceKey,omitempty"`
}

// GraceKeyInfo contains information about a key in grace period.
type GraceKeyInfo struct {
	KeyID       string    `json:"keyId"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expiresAt"`
	GraceEndsAt time.Time `json:"graceEndsAt"`
}

// RotateRequest is the request body for POST /api/v1/validator/key/rotate
type RotateRequest struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

// RevokeRequest is the request body for POST /api/v1/validator/key/revoke
type RevokeRequest struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

// handleStatus handles GET /api/v1/validator/key/status
func (api *KeyRotationAPI) handleStatus(w http.ResponseWriter, r *http.Request, manager *keyrotation.Manager) {
	activeKey := manager.GetActiveKey()
	if activeKey == nil {
		http.Error(w, "no active key", http.StatusNotFound)
		return
	}

	daysUntilExp := int(time.Until(activeKey.ExpiresAt).Hours() / 24)

	response := &KeyStatusResponse{
		KeyID:               activeKey.KeyID,
		Status:              activeKey.Status.String(),
		PublicKey:           keyrotation.EncodePublicKey(activeKey.PublicKey),
		ActivatedAt:         activeKey.ActivatedAt,
		ExpiresAt:           activeKey.ExpiresAt,
		GraceEndsAt:         activeKey.GraceEndsAt,
		DaysUntilExpiration: daysUntilExp,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRotate handles POST /api/v1/validator/key/rotate
func (api *KeyRotationAPI) handleRotate(w http.ResponseWriter, r *http.Request, manager *keyrotation.Manager) {
	var req RotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Operator == "" {
		http.Error(w, "operator is required", http.StatusBadRequest)
		return
	}

	if req.Reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	err := manager.RotateKey(req.Operator, req.Reason)
	if err != nil {
		api.logger.Error("Key rotation failed", "error", err, "operator", req.Operator)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the new key status
	api.handleStatus(w, r, manager)
}

// handleRevoke handles POST /api/v1/validator/key/revoke
func (api *KeyRotationAPI) handleRevoke(w http.ResponseWriter, r *http.Request, manager *keyrotation.Manager) {
	var req RevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Operator == "" {
		http.Error(w, "operator is required", http.StatusBadRequest)
		return
	}

	if req.Reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	err := manager.RevokeKey(req.Operator, req.Reason)
	if err != nil {
		api.logger.Error("Key revocation failed", "error", err, "operator", req.Operator)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the new key status
	api.handleStatus(w, r, manager)
}
