// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// The control API adjusts a running generator without restarting it — a
// restart re-bootstraps the whole account universe, so "turn the rate up"
// must not cost a fresh network.
//
//	GET  /control        -> current rate, effective weights, overrides
//	POST /control        -> {"tps": 10}                       set the rate
//	                        {"mix": {"burn-tokens": 0}}       merge weight overrides
//	                        {"tps": 10, "mix": {...}}         both at once
//	DELETE /control/mix  -> drop every override (compiled-in weights again)
//
// A mix override of 0 disables an action; unknown action names are rejected
// wholesale so a typo cannot silently change nothing. Changes are logged to
// the run log, so the soak record shows when the knobs moved.

// controlRequest is the POST body. Absent fields are left unchanged.
type controlRequest struct {
	TPS *float64       `json:"tps,omitempty"`
	Mix map[string]int `json:"mix,omitempty"`
}

// controlState is the GET response.
type controlState struct {
	TPS       float64        `json:"tps"`
	Weights   map[string]int `json:"weights"`             // effective weight per action
	Overrides map[string]int `json:"overrides,omitempty"` // runtime overrides only
	Disabled  []string       `json:"disabled,omitempty"`  // actions with effective weight 0
}

func (e *env) controlState() controlState {
	s := controlState{TPS: e.currentTPS(), Weights: map[string]int{}}
	for _, a := range menu {
		w := e.weightOf(a)
		s.Weights[a.name] = w
		if w == 0 {
			s.Disabled = append(s.Disabled, a.name)
		}
	}
	if m := e.mixOverride.Load(); m != nil && len(*m) > 0 {
		s.Overrides = map[string]int{}
		for k, v := range *m {
			s.Overrides[k] = v
		}
	}
	return s
}

// serveControl runs the control API until the context ends. Failure to bind is
// fatal: a soak that thinks it can steer the generator but cannot is worse
// than one that stops at launch.
func serveControl(ctx context.Context, e *env, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/control", e.handleControl)
	mux.HandleFunc("/control/mix", e.handleControlMix)

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Printf("control API listening on http://%s/control", addr)
	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		fatalIf(err, "control API")
	}
}

func (e *env) handleControl(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, e.controlState())

	case http.MethodPost, http.MethodPut:
		var req controlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("bad request body: %v", err)})
			return
		}
		if req.TPS == nil && req.Mix == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": `nothing to do: set "tps", "mix", or both`})
			return
		}
		// Validate everything before applying anything, so a request either
		// takes effect whole or not at all.
		if req.Mix != nil {
			if err := validateMix(req.Mix); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		if req.TPS != nil {
			if err := e.setTPS(*req.TPS); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			log.Printf("control: rate set to %v tps", *req.TPS)
		}
		if req.Mix != nil {
			if err := e.setMix(req.Mix); err != nil {
				// Unreachable after validateMix, but never apply silently.
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			log.Printf("control: mix overrides merged: %v", req.Mix)
		}
		writeJSON(w, http.StatusOK, e.controlState())

	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use GET to read, POST to change"})
	}
}

func (e *env) handleControlMix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "DELETE /control/mix clears all overrides; use POST /control to set them"})
		return
	}
	e.clearMix()
	log.Printf("control: mix overrides cleared")
	writeJSON(w, http.StatusOK, e.controlState())
}

// validateMix is setMix's validation without the apply, for all-or-nothing
// request handling.
func validateMix(weights map[string]int) error {
	for name, w := range weights {
		if w < 0 {
			return fmt.Errorf("%s: weight must be >= 0, got %d", name, w)
		}
		found := false
		for _, a := range menu {
			if a.name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown action %q; valid actions: %v", name, actionNames())
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
