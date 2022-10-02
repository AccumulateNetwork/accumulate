// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Metrics returns Metrics for explorer (tps, etc.)
func (m *JrpcMethods) Metrics(_ context.Context, params json.RawMessage) interface{} {
	req := new(MetricsQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	c, err := promapi.NewClient(promapi.Config{
		Address: m.PrometheusServer,
	})
	if err != nil {
		m.logError("Metrics query failed", "error", err, "server", m.PrometheusServer)
		return internalError(err)
	}
	papi := prometheus.NewAPI(c)

	if req.Duration == 0 {
		req.Duration = time.Hour
	}

	res := new(protocol.MetricsResponse)
	switch req.Metric {
	case "tps":
		query := fmt.Sprintf(metricTPS, req.Duration)
		v, _, err := papi.Query(context.Background(), query, time.Now())
		if err != nil {
			return metricsQueryError(fmt.Errorf("query failed: %w", err))
		}
		vec, ok := v.(model.Vector)
		if !ok {
			return ErrMetricsNotAVector
		}
		if len(vec) == 0 {
			return ErrMetricsVectorEmpty
		}
		res.Value = vec[0].Value / model.SampleValue(req.Duration.Seconds())
	default:
		return validatorError(fmt.Errorf("%q is not a valid metric", req.Metric))
	}

	return &ChainQueryResponse{
		Type: "metrics",
		Data: res,
	}
}
