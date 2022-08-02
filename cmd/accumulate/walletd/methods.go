package walletd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../../tools/cmd/gen-types --package api --out api/types_gen.go api/types.yml
//go:generate go run ../../../tools/cmd/gen-api --package walletd api/methods.yml

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	resp := api.VersionResponse{Version: "hello", Commit: "1234abcd"}
	return resp
}

func (m *JrpcMethods) DecodeTransaction(_ context.Context, params json.RawMessage) interface{} {
	req := api.DecodeTransactionRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	resp := api.DecodeTransactionResponse{}
	return resp
}

func encodeWithValue[T encoding.BinaryValue](v T) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		check(json.Unmarshal([]byte(args[0]), v))

		data, err := v.MarshalBinary()
		check(err)
		if encodeFlag.Hash {
			if u, ok := any(v).(interface{ GetHash() []byte }); ok {
				// Use custom hashing for transactions and certain transaction
				// bodies
				data = u.GetHash()
			} else {
				data = doSha256(data)
			}
		}

		fmt.Printf("%x\n", data)
	}
}

func encodeWithFunc[T encoding.BinaryValue](fn func([]byte) (T, error)) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		v, err := fn([]byte(args[0]))
		check(err)

		data, err := v.MarshalBinary()
		check(err)
		if encodeFlag.Hash {
			data = doSha256(data)
		}

		fmt.Printf("%x\n", data)
	}
}

func (m *JrpcMethods) EncodeTransaction(_ context.Context, params json.RawMessage) interface{} {
	req := api.EncodeTransactionRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}
	protocol.TransactionBody{}
	protocol.Transa()

	resp := api.EncodeTransactionRespones{}
	return resp
}

func (m *JrpcMethods) Sign(_ context.Context, params json.RawMessage) interface{} {
	req := api.SignRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	resp := api.SignResponse{}
	return resp
}

func (m *JrpcMethods) KeyList(_ context.Context, _ json.RawMessage) interface{} {
	resp := api.KeyListResponse{}
	return resp
}
