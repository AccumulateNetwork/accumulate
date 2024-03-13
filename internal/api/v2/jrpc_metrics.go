// Copyright 2024 The Accumulate Authors
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

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Metrics returns Metrics for explorer (tps, etc.)
func (m *JrpcMethods) Metrics(ctx context.Context, params json.RawMessage) interface{} {
	if m.LocalV3 == nil {
		return accumulateError(fmt.Errorf("service not available"))
	}

	req := new(MetricsQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	if req.Duration == 0 {
		req.Duration = time.Hour
	}

	res := new(protocol.MetricsResponse)
	switch req.Metric {
	case "tps":
		x, err := m.LocalV3.Metrics(ctx, api.MetricsOptions{
			Span: uint64(req.Duration / time.Second),
		})
		if err != nil {
			return accumulateError(err)
		}
		res.Value = x.TPS
	default:
		return validatorError(fmt.Errorf("%q is not a valid metric", req.Metric))
	}

	return &ChainQueryResponse{
		Type: "metrics",
		Data: res,
	}
}
