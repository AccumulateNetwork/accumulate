package goaccumulate

import (
	"context"
	"encoding/json"

	"github.com/AccumulateNetwork/accumulate"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
)

func GetVersion() (string, error) {
	res := new(api2.QueryResponse)

	res.Type = "version"
	res.Data = map[string]interface{}{
		"version": accumulate.Version,
		"commit":  accumulate.Commit,
	}

	if err := Client.Request(context.Background(), "version", nil, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	return string(str), nil
}

/*

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	res := new(QueryResponse)
	res.Type = "version"
	res.Data = map[string]interface{}{
		"version":        accumulate.Version,
		"commit":         accumulate.Commit,
		"versionIsKnown": accumulate.IsVersionKnown(),
	}
	return res
}
*/
