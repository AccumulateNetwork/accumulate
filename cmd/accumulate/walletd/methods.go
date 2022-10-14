package walletd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
		return jsonrpc2.NewError(api.ErrorCodeNotFound.Code(), "resolve key error", err)
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
		return jsonrpc2.NewError(api.ErrorCodeNotFound.Code(), "adi list error", err)
	}

	for _, v := range adis {
		resp.Urls = append(resp.Urls, v.String())
	}
	return resp
}

func (m *JrpcMethods) NewSendTokensTransaction(_ context.Context, params json.RawMessage) interface{} {
	req := api.NewTransactionRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	origin, err := url.Parse(req.Origin)
	if err != nil {
		return validatorError(err)
	}
	txn := protocol.Transaction{}
	txn.Header.Principal = origin
	resp, err := txn.MarshalJSON()
	if err != nil {
		return validatorError(err)
	}
	value, _ := GetWallet().Get(BucketTransactionCache, []byte(req.TxName))
	if value != nil {
		return validatorError(fmt.Errorf("txn already available with the tx name"))
	}
	err = GetWallet().Put(BucketTransactionCache, []byte(req.TxName), resp)
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (m *JrpcMethods) AddSendTokensOutput(_ context.Context, params json.RawMessage) interface{} {
	req := api.AddSendTokensOutputRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	value, err := GetWallet().Get(BucketTransactionCache, []byte(req.TxName))
	if err != nil {
		return validatorError(err)
	}
	txn := protocol.Transaction{}
	err = txn.UnmarshalBinary(value)
	if err != nil {
		return validatorError(err)
	}
	address, err := url.Parse(req.TokenAddress)
	if err != nil {
		return validatorError(err)
	}
	recipient := &protocol.TokenRecipient{
		Url:    address,
		Amount: req.Amount,
	}
	sendToken := protocol.SendTokens{}
	sendToken.To = append(sendToken.To, recipient)
	txn.Body = &sendToken
	resp, err := txn.MarshalBinary()
	if err != nil {
		return validatorError(err)
	}
	err = GetWallet().Put(BucketTransactionCache, []byte(req.TxName), resp)
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (m *JrpcMethods) DeleteSendTokensTransaction(_ context.Context, params json.RawMessage) interface{} {
	req := api.DeleteTransactionRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	value, err := GetWallet().Get(BucketTransactionCache, []byte(req.Name))
	if err != nil {
		return validatorError(err)
	}
	resp := protocol.Transaction{}
	err = resp.UnmarshalBinary(value)
	if err != nil {
		return validatorError(err)
	}
	err = GetWallet().Delete(BucketTransactionCache, []byte(req.Name))
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (m *JrpcMethods) SignSendTokensTransaction(_ context.Context, params json.RawMessage) interface{} {
	req := api.SignTransactionRequest{}
	err := json.Unmarshal(params, &req)
	if err != nil {
		return validatorError(err)
	}

	value, err := GetWallet().Get(BucketTransactionCache, []byte(req.TxName))
	if err != nil {
		return validatorError(err)
	}
	txn := protocol.Transaction{}
	err = txn.UnmarshalBinary(value)
	if err != nil {
		return validatorError(err)
	}
	key, err := LookupByLabel(req.KeyName)
	if err != nil {
		return validatorError(err)
	}
	signer := new(signing.Builder)
	signer.Url = txn.Header.Principal
	signer.Type = key.KeyInfo.Type
	signer.Version = uint64(req.SignerVersion)
	if req.Timestamp == 0 {
		signer.Timestamp = signer.SetTimestampToNow().Timestamp
	} else {
		signer.Timestamp = signing.TimestampFromValue(req.Timestamp)
	}
	signer.SetPrivateKey(key.PrivateKey)
	sig, err := signer.Initiate(&txn)
	if err != nil {
		return validatorError(err)
	}
	resp, err := sig.MarshalBinary()
	if err != nil {
		return validatorError(err)
	}
	return resp
}
