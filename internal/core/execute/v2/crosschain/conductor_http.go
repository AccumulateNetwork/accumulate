// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build testnet
// +build testnet

package crosschain

import (
	"encoding/json"
	"net/http"
)

// RegisterPauseEndpoints registers HTTP endpoints for pause control
// Only available when compiled with -tags testnet
func (cc *CrossChainConductor) RegisterPauseEndpoints(mux *http.ServeMux) {
	// Pause endpoint
	mux.HandleFunc("/debug/ccc/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		cc.Pause()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "paused",
		})
	})
	
	// Resume endpoint
	mux.HandleFunc("/debug/ccc/resume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		cc.Resume()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "resumed",
		})
	})
	
	// Status endpoint
	mux.HandleFunc("/debug/ccc/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"paused": cc.IsPaused(),
		})
	})
	
	cc.logger.Info("⚠️  TESTNET: CCC pause endpoints registered",
		"endpoints", []string{
			"/debug/ccc/pause",
			"/debug/ccc/resume",
			"/debug/ccc/status",
		})
}