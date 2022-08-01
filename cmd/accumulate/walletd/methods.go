package walletd

import (
	"context"
	"encoding/json"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
)

//go:generate go run ../../../tools/cmd/gen-types --package api --out api/types_gen.go api/types.yml
//go:generate go run ../../../tools/cmd/gen-api --package walletd api/methods.yml

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	resp := api.Version{Version: "hello", Commit: "1234abcd"}
	return resp
}
