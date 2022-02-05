package client

import (
	"bytes"
	"context"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Query calls the `query` method with the given response type. If resp is nil,
// Query attempts to detect the response type. If that fails, Query unmarshals
// the response as interface{}.
func (c *Client) Query(ctx context.Context, req *api.GeneralQuery, resp interface{}) (interface{}, error) {
	// If the caller provides a response value, unmarshal into it
	if resp != nil {
		err := c.RequestAPIv2(ctx, "query", req, &resp)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// Make the request
	var data json.RawMessage
	err := c.RequestAPIv2(ctx, "query", req, &data)
	if err != nil {
		return nil, err
	}

	// The response must have a type property
	var typ struct {
		Type string `json:"type"`
	}
	err = json.Unmarshal(data, &typ)
	if err != nil {
		// If it doesn't, unmarshal as anything
		var v interface{}
		err = json.Unmarshal(data, &v)
		return v, err
	}

	// If the type is a known account type, attempt to unmarshal as an account
	// response with the given account type
	acntResp, err := unmarshalAsAccount(typ.Type, data)
	if err != nil {
		return nil, err
	} else if acntResp != nil {
		return acntResp, nil
	}

	// If the type is a known transaction type, attempt to unmarshal as a
	// transaction response with the given transaction type
	txnResp, err := unmarshalAsTransaction(typ.Type, data)
	if err != nil {
		return nil, err
	} else if txnResp != nil {
		return txnResp, nil
	}

	// Attempt to unmarshal as a multi response. MultiResponse has a distinctive
	// structure, thus there should be no false positives if we disallow unknown
	// fields.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	multiResp := new(api.MultiResponse)
	if dec.Decode(multiResp) == nil {
		return multiResp, nil
	}

	// Attempt to unmarshal as an account response. If a response is not a
	// transaction, account, or multi response, it is likely something else
	// packaged within the account response type.
	acntResp = new(api.ChainQueryResponse)
	if dec.Decode(acntResp) == nil {
		return acntResp, nil
	}

	// If everything fails, unmarshal as anything
	var v interface{}
	err = json.Unmarshal(data, &v)
	return v, err
}

func unmarshalAsAccount(typStr string, data []byte) (*api.ChainQueryResponse, error) {
	typ, ok := protocol.AccountTypeByName(typStr)
	if !ok {
		return nil, nil
	}

	account, err := protocol.NewAccount(typ)
	if err != nil {
		return nil, nil
	}

	resp := new(api.ChainQueryResponse)
	resp.Data = account
	err = json.Unmarshal(data, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func unmarshalAsTransaction(typStr string, data []byte) (*api.TransactionQueryResponse, error) {
	typ, ok := protocol.TransactionTypeByName(typStr)
	if !ok {
		return nil, nil
	}

	txn, err := protocol.NewTransaction(typ)
	if err != nil {
		return nil, nil
	}

	resp := new(api.TransactionQueryResponse)
	resp.Data = txn
	err = json.Unmarshal(data, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
