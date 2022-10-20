package walletd

// GENERATED BY go run ./tools/cmd/gen-api. DO NOT EDIT.

import (
	"encoding/json"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

func (m *JrpcMethods) populateMethodTable() jsonrpc2.MethodMap {
	if m.methods == nil {
		m.methods = make(jsonrpc2.MethodMap, 13)
	}

	m.methods["add-output"] = m.AddSendTokensOutput
	m.methods["adi-list"] = m.AdiList
	m.methods["create-envelope"] = m.CreateEnvelope
	m.methods["create-transaction"] = m.CreateTransaction
	m.methods["decode"] = m.Decode
	m.methods["delete-transaction"] = m.DeleteSendTokensTransaction
	m.methods["encode"] = m.Encode
	m.methods["key-list"] = m.KeyList
	m.methods["new-transaction"] = m.NewSendTokensTransaction
	m.methods["resolve-key"] = m.ResolveKey
	m.methods["sign"] = m.Sign
	m.methods["sign-transaction"] = m.SignSendTokensTransaction
	m.methods["version"] = m.Version

	return m.methods
}

func (m *JrpcMethods) parse(params json.RawMessage, target interface{}, validateFields ...string) error {
	err := json.Unmarshal(params, target)
	if err != nil {
		return validatorError(err)
	}

	// validate fields
	if len(validateFields) == 0 {
		if err = m.validate.Struct(target); err != nil {
			return validatorError(err)
		}
	} else {
		if err = m.validate.StructPartial(target, validateFields...); err != nil {
			return validatorError(err)
		}
	}

	return nil
}

func jrpcFormatResponse(res interface{}, err error) interface{} {
	if err != nil {
		return accumulateError(err)
	}

	return res
}
