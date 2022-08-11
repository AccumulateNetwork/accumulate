package walletd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/jsonrpc2/v15"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run ../../../tools/cmd/gen-types --package api --out api/types_gen.go api/types.yml
//go:generate go run ../../../tools/cmd/gen-api --package walletd api/methods.yml
//go:generate go run ../../../tools/cmd/gen-enum --out api/enums_gen.go --package api api/enums.yml

func (m *JrpcMethods) Version(_ context.Context, params json.RawMessage) interface{} {
	resp := api.VersionResponse{Version: "hello", Commit: "1234abcd"}
	return resp
}

func (m *JrpcMethods) Decode(_ context.Context, params json.RawMessage) interface{} {
	req := api.DecodeRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	//unmarshal as account
	ac, err := protocol.UnmarshalAccount(req.DataBinary)
	if err == nil {
		resp := api.DecodeResponse{}
		data, err := json.Marshal(&ac)
		if err != nil {
			return accumulateError(err)
		}
		resp.DataJson = string(data)
		return resp
	}

	//unmarshal as a transaction
	tx := protocol.Transaction{}
	err = tx.UnmarshalBinary(req.DataBinary)
	if err == nil {
		resp := api.DecodeResponse{}
		data, err := tx.MarshalJSON()
		if err != nil {
			return accumulateError(err)
		}
		resp.DataJson = string(data)
		return resp
	}

	//unmarshal as a transaction body
	tb, err := protocol.UnmarshalTransactionBody(req.DataBinary)
	if err == nil {
		resp := api.DecodeResponse{}
		data, err := json.Marshal(&tb)
		if err != nil {
			return accumulateError(err)
		}
		resp.DataJson = string(data)
		return resp
	}

	//unmarshal as a transaction body
	th := protocol.TransactionHeader{}
	err = th.UnmarshalBinary(req.DataBinary)
	if err == nil {
		resp := api.DecodeResponse{}
		data, err := json.Marshal(&th)
		if err != nil {
			return accumulateError(err)
		}
		resp.DataJson = string(data)
		return resp
	}

	return accumulateError(fmt.Errorf("cannot decode binary"))
}

func (m *JrpcMethods) Encode(_ context.Context, params json.RawMessage) interface{} {
	req := api.EncodeRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	//process as account
	ac, err := protocol.UnmarshalAccountJSON([]byte(req.DataJson))
	if err == nil {
		resp := api.EncodeAccountResponse{}
		resp.AccountBinary, err = ac.MarshalBinary()
		if err != nil {
			return accumulateError(err)
		}
		return resp
	}

	//process as full transaction
	tx := protocol.Transaction{}
	err = tx.UnmarshalJSON([]byte(req.DataJson))
	if err == nil {
		resp := api.EncodeTransactionResponse{}
		resp.TransactionBinary, err = tx.MarshalBinary()
		if err != nil {
			return accumulateError(err)
		}
		resp.TransactionHash = tx.GetHash()
		return resp
	}

	//process as transaction header only
	th := protocol.TransactionHeader{}
	err = th.UnmarshalJSON([]byte(req.DataJson))
	if err == nil {
		resp := api.EncodeTransactionHeaderResponse{}
		resp.TransactionHeaderBinary, err = th.MarshalBinary()
		if err != nil {
			return accumulateError(err)
		}
		return resp
	}

	//process as transaction body
	tb, err := protocol.UnmarshalTransactionBodyJSON([]byte(req.DataJson))
	if err == nil {
		resp := api.EncodeTransactionBodyResponse{}
		resp.TransactionBodyBinary, err = tb.MarshalBinary()
		if err != nil {
			return accumulateError(err)
		}
		return resp
	}

	return accumulateError(fmt.Errorf("malformed encoding request"))
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

func (m *JrpcMethods) CreateEnvelope(_ context.Context, params json.RawMessage) interface{} {
	resp := api.KeyListResponse{}
	return resp
}

func (m *JrpcMethods) CreateTransaction(_ context.Context, params json.RawMessage) interface{} {
	resp := api.KeyListResponse{}
	return resp
}

func (m *JrpcMethods) KeyList(_ context.Context, params json.RawMessage) interface{} {
	resp := api.KeyListResponse{}
	var err error
	resp.KeyList, err = GetKeyList()
	if err != nil {
		return jsonrpc2.NewError(api.ErrorCodeGeneralError.Code(), "key list error", err)
	}
	return resp
}

func (m *JrpcMethods) ResolveKey(_ context.Context, params json.RawMessage) interface{} {
	resp := api.ResolveKeyResponse{}

	req := api.ResolveKeyRequest{}
	err := json.Unmarshal(params, &req)

	if err != nil {
		return jsonrpc2.NewError(api.ErrorCodeGeneralError.Code(), "resolve key error", err)
	}

	label, isLite := LabelForLiteIdentity(req.KeyNameOrLiteAddress)
	var k *Key
	if isLite {
		k, err = LookupByLiteIdentityUrl(label)
	} else {
		label, isLite = LabelForLiteTokenAccount(req.KeyNameOrLiteAddress)
		if isLite {
			k, err = LookupByLiteTokenUrl(label)
		} else {
			k, err = LookupByLabel(label)
		}
	}

	if err != nil {
		gen := api.GeneralResponse{}
		gen.Code = api.ErrorCodeNotFound
		gen.Error = err.Error()
		return gen
	}

	resp.KeyData.PublicKey = k.PublicKey
	resp.KeyData.Derivation = k.KeyInfo.Derivation
	resp.KeyData.KeyType = k.KeyInfo.Type
	return resp
}

func (m *JrpcMethods) AdiList(_ context.Context, params json.RawMessage) interface{} {
	resp := api.AdiListResponse{}
	var err error
	adis, err := getAdiList()
	if err != nil {
		return jsonrpc2.NewError(api.ErrorCodeGeneralError.Code(), "adi list error", err)
	}

	for _, v := range adis {
		resp.Urls = append(resp.Urls, v.String())
	}
	return resp
}
