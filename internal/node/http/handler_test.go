// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
)

func TestMetricsEndpoint(t *testing.T) {
	// Create a simple router with just the metrics endpoint
	mux := httprouter.New()
	metricsHandler := promhttp.Handler()
	mux.GET("/metrics", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		metricsHandler.ServeHTTP(w, r)
	})

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	// Serve the request
	mux.ServeHTTP(w, req)

	// Check status code
	require.Equal(t, http.StatusOK, w.Code)

	// Check that we got Prometheus metrics format
	body := w.Body.String()
	require.True(t, strings.Contains(body, "# HELP") || strings.Contains(body, "# TYPE"),
		"response should contain Prometheus metrics format")
}

func TestMetricsEndpointContainsDagBftMetrics(t *testing.T) {
	// Trigger metric registration by accessing a metric
	// (metrics are auto-registered via promauto at package init time)
	metrics.CurrentRound.Set(0)

	// Create a simple router with just the metrics endpoint
	mux := httprouter.New()
	metricsHandler := promhttp.Handler()
	mux.GET("/metrics", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		metricsHandler.ServeHTTP(w, r)
	})

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	// Serve the request
	mux.ServeHTTP(w, req)

	// Check status code
	require.Equal(t, http.StatusOK, w.Code)

	// The metrics are registered via promauto at package init time,
	// so they should appear in the default registry
	body := w.Body.String()

	// Check for the presence of DAG-BFT metrics
	require.Contains(t, body, "accumulate_dagbft_current_round", "should contain DAG-BFT current_round metric")
	require.Contains(t, body, "accumulate_dagbft_certificates_created_total", "should contain DAG-BFT certificates_created_total metric")
}
