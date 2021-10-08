package api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/go-playground/validator/v10"
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
	return &API{randomRouterPorts(t), validator.New(), q}
}

func randomRouterPorts(t *testing.T) *cfg.API {
	t.Helper()
	port := GetFreePort(t)
	return &cfg.API{
		JSONListenAddress: fmt.Sprintf("localhost:%d", port),
		RESTListenAddress: fmt.Sprintf("localhost:%d", port+1),
	}
}

func (api *API) GetData(ctx context.Context, params json.RawMessage) interface{} {
	return api.getData(ctx, params)
}

func (api *API) CreateADI(ctx context.Context, params json.RawMessage) interface{} {
	return api.createADI(ctx, params)
}
