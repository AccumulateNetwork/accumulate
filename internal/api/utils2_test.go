package api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/protocol"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/require"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

func GetFreePort(t *testing.T) int {
	t.Helper()
	port, err := tmnet.GetFreePort()
	require.NoError(t, err)
	return port
}

func NewTest(t *testing.T, q *Query) *API {
	t.Helper()
	port := GetFreePort(t)
	config := &cfg.API{
		JSONListenAddress: fmt.Sprintf("localhost:%d", port),
		RESTListenAddress: fmt.Sprintf("localhost:%d", port+1),
	}
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

func (api *API) BroadcastTx(wait bool, tx *transactions.GenTransaction) *acmeapi.APIDataResponse {
	return api.broadcastTx(wait, tx)
}
