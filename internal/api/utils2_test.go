package api

import (
	"context"
	"encoding/json"
	"testing"

	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/protocol"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/require"
)

func NewTest(t *testing.T, config *cfg.API, q *Query) *API {
	t.Helper()
	v, err := protocol.NewValidator()
	require.NoError(t, err)
	return &API{config, v, q}
}

func (api *API) GetData(ctx context.Context, params json.RawMessage) interface{} {
	return api.getData(ctx, params)
}

func (api *API) CreateADI(ctx context.Context, params json.RawMessage) interface{} {
	return api.createADI(ctx, params)
}

func (api *API) GetTokenAccount(ctx context.Context, params json.RawMessage) interface{} {
	return api.getTokenAccount(ctx, params)
}

func (api *API) GetADI(ctx context.Context, params json.RawMessage) interface{} {
	return api.getADI(ctx, params)
}

func (api *API) BroadcastTx(wait bool, tx *transactions.Envelope) (*acmeapi.APIDataResponse, error) {
	return api.broadcastTx(wait, tx)
}
