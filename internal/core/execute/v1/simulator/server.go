// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Simulator) NewServer() http.Handler {
	return jsonrpc2.HTTPRequestHandler(jsonrpc2.MethodMap{
		"describe": infoServerMethod(s, (*client.Client).Describe),
		"status":   infoServerMethod(s, (*client.Client).Status),
		"version":  infoServerMethod(s, (*client.Client).Version),

		"query":              routedServerMethod(s, (*client.Client).Query, func(r *api.GeneralQuery) *url.URL { return r.Url }),
		"query-data":         routedServerMethod(s, (*client.Client).QueryData, func(r *api.DataEntryQuery) *url.URL { return r.Url }),
		"query-data-set":     routedServerMethod(s, (*client.Client).QueryDataSet, func(r *api.DataEntrySetQuery) *url.URL { return r.Url }),
		"query-directory":    routedServerMethod(s, (*client.Client).QueryDirectory, func(r *api.DirectoryQuery) *url.URL { return r.Url }),
		"query-key-index":    routedServerMethod(s, (*client.Client).QueryKeyPageIndex, func(r *api.KeyPageIndexQuery) *url.URL { return r.Url }),
		"query-major-blocks": routedServerMethod(s, (*client.Client).QueryMajorBlocks, func(r *api.MajorBlocksQuery) *url.URL { return r.Url }),
		"query-minor-blocks": routedServerMethod(s, (*client.Client).QueryMinorBlocks, func(r *api.MinorBlocksQuery) *url.URL { return r.Url }),
		"query-synth":        routedServerMethod(s, (*client.Client).QuerySynth, func(r *api.SyntheticTransactionRequest) *url.URL { return r.Source }),
		"query-tx":           routedServerMethod(s, (*client.Client).QueryTx, nil),
		"query-tx-history":   routedServerMethod(s, (*client.Client).QueryTxHistory, func(r *api.TxHistoryQuery) *url.URL { return r.Url }),

		"execute-direct": routedServerMethod(s, (*client.Client).ExecuteDirect, func(r *api.ExecuteRequest) *url.URL { return r.Envelope.Signatures[0].GetSigner() }),
		"faucet":         routedServerMethod(s, (*client.Client).Faucet, func(*protocol.AcmeFaucet) *url.URL { return protocol.FaucetUrl }),
	}, log.New(os.Stdout, "", 0))
}

func infoServerMethod[Out any](s *Simulator, handler func(*client.Client, context.Context) (Out, error)) jsonrpc2.MethodFunc {
	return func(ctx context.Context, _ json.RawMessage) interface{} {
		resp, err := handler(s.Executors[protocol.Directory].API, ctx)
		if err != nil {
			return accumulateError(err)
		}
		return resp
	}
}

func routedServerMethod[In, Out any](s *Simulator, handler func(*client.Client, context.Context, *In) (Out, error), getUrl func(*In) *url.URL) jsonrpc2.MethodFunc {
	return func(ctx context.Context, params json.RawMessage) interface{} {
		req := new(In)
		err := json.Unmarshal(params, req)
		if err != nil {
			return validatorError(err)
		}

		if getUrl != nil {
			resp, err := handler(s.PartitionFor(getUrl(req)).API, ctx, req)
			if err != nil {
				return accumulateError(err)
			}
			return resp
		}

		for _, x := range s.Executors {
			resp, err := handler(x.API, ctx, req)
			switch {
			case err == nil:
				return resp
			case errors.Is(err, errors.NotFound):
				continue
			default:
				return accumulateError(err)
			}
		}
		return errors.NotFound
	}
}

func validatorError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(api.ErrCodeValidation, "Validation Error", err)
}

func accumulateError(err error) jsonrpc2.Error {
	if errors.Is(err, errors.NotFound) {
		return jsonrpc2.NewError(api.ErrCodeNotFound, "Accumulate Error", "Not Found")
	}

	var perr *errors.Error
	if errors.As(err, &perr) {
		return jsonrpc2.NewError(api.ErrCodeProtocolBase-jsonrpc2.ErrorCode(perr.Code), "Accumulate Error", perr.Message)
	}

	var jerr jsonrpc2.Error
	if errors.As(err, &jerr) {
		return jerr
	}

	return jsonrpc2.NewError(api.ErrCodeAccumulate, "Accumulate Error", err)
}
