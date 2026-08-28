// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func controlEnv(tps float64) *env {
	e := new(env)
	e.rateBits.Store(math.Float64bits(tps))
	return e
}

func postControl(t *testing.T, e *env, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/control", strings.NewReader(body))
	w := httptest.NewRecorder()
	e.handleControl(w, req)
	return w
}

func TestControl_SetRateTakesEffectLive(t *testing.T) {
	e := controlEnv(2)
	require.Equal(t, 2.0, e.currentTPS())

	w := postControl(t, e, `{"tps": 10}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 10.0, e.currentTPS(), "the generate loop reads this every iteration")
}

func TestControl_RejectsNonsenseRates(t *testing.T) {
	e := controlEnv(2)
	for _, body := range []string{`{"tps": 0}`, `{"tps": -3}`, `{"tps": 99999}`} {
		w := postControl(t, e, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
	}
	assert.Equal(t, 2.0, e.currentTPS(), "a rejected request must change nothing")
}

func TestControl_MixOverridesAndZeroDisables(t *testing.T) {
	e := controlEnv(2)
	w := postControl(t, e, `{"mix": {"burn-tokens": 0, "send-tokens-lite": 50}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	for _, a := range menu {
		switch a.name {
		case "burn-tokens":
			assert.Equal(t, 0, e.weightOf(a), "weight 0 disables the action")
		case "send-tokens-lite":
			assert.Equal(t, 50, e.weightOf(a))
		default:
			assert.Equal(t, a.weight, e.weightOf(a), "%s must keep its compiled-in weight", a.name)
		}
	}
	assert.Contains(t, w.Body.String(), `"disabled":["burn-tokens"]`)
}

func TestControl_UnknownActionRejectedWholesale(t *testing.T) {
	e := controlEnv(2)
	// One valid name, one typo: NOTHING may be applied.
	w := postControl(t, e, `{"tps": 7, "mix": {"send-tokens-lite": 50, "sned-tokens": 1}}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "sned-tokens")
	assert.Equal(t, 2.0, e.currentTPS(), "tps must not change when the mix half of the request is invalid")
	for _, a := range menu {
		assert.Equal(t, a.weight, e.weightOf(a), "%s must be untouched", a.name)
	}
}

func TestControl_DeleteClearsOverrides(t *testing.T) {
	e := controlEnv(2)
	require.NoError(t, e.setMix(map[string]int{"burn-tokens": 0}))

	req := httptest.NewRequest(http.MethodDelete, "/control/mix", nil)
	w := httptest.NewRecorder()
	e.handleControlMix(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	for _, a := range menu {
		assert.Equal(t, a.weight, e.weightOf(a))
	}
}

func TestControl_PickNeverDrawsADisabledAction(t *testing.T) {
	e := controlEnv(2)
	e.u = newUniverse(rand.New(rand.NewSource(42)))

	// Disable everything except one action; pick must always return it.
	// (No identities exist in this universe, so needsIdentity actions are
	// already out of the draw; disable the rest explicitly.)
	weights := map[string]int{}
	for _, a := range menu {
		if a.name != "send-tokens-lite" {
			weights[a.name] = 0
		}
	}
	require.NoError(t, e.setMix(weights))

	for i := 0; i < 200; i++ {
		assert.Equal(t, "send-tokens-lite", e.pick().name)
	}
}

func TestControl_GetReportsState(t *testing.T) {
	e := controlEnv(4)
	req := httptest.NewRequest(http.MethodGet, "/control", nil)
	w := httptest.NewRecorder()
	e.handleControl(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"tps":4`)
	assert.Contains(t, w.Body.String(), `"send-tokens-lite"`)
}
