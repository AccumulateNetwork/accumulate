package api

import (
	"context"
	"encoding/json"
	"fmt"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/go-playground/validator/v10"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

func NewTest(q *Query) *API {
	return &API{randomRouterPorts(), validator.New(), q}
}

func randomRouterPorts() *cfg.API {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
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
