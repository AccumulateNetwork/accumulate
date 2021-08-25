package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
)

type API struct {
	port     int
	validate *validator.Validate
}

// StartAPI starts new JSON-RPC server
func StartAPI(port int) *API {

	fmt.Printf("Starting JSON-RPC API at http://localhost:%d\n", port)

	api := &API{}
	api.port = port
	api.validate = validator.New()

	methods := jsonrpc2.MethodMap{
		// ADI
		"adi":        api.getADI,
		"adi-create": api.createADI,

		// token
		"token":                api.getToken,
		"token-create":         api.createToken,
		"token-account":        api.getTokenAccount,
		"token-account-create": api.createTokenAccount,
		"token-tx":             api.getTokenTx,
		"token-tx-create":      api.createTokenTx,
	}

	apiHandler := jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))
	http.HandleFunc("/v1", apiHandler)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), nil))

	return api

}

// getADI returns ADI info
func (api *API) getADI(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &ADI{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate only ADI.URL
	if err = api.validate.StructPartial(req, "URL"); err != nil {
		return NewValidatorError(err)
	}

	resp := &ADI{}
	resp.URL = req.URL

	// Tendermint integration here

	return resp
}

// createADI creates ADI
func (api *API) createADI(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &APIRequest{}
	data := &ADI{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	resp := &ADI{}

	// Tendermint integration here

	return resp
}

// getToken returns Token info
func (api *API) getToken(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &Token{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate only Token.URL
	if err = api.validate.StructPartial(req, "URL"); err != nil {
		return NewValidatorError(err)
	}

	resp := &Token{}
	resp.URL = req.URL

	// Tendermint integration here

	return resp

}

// createToken creates Token
func (api *API) createToken(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &APIRequest{}
	data := &Token{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	resp := &Token{}

	// Tendermint integration here

	return resp

}

// getTokenAccount returns Token Account info
func (api *API) getTokenAccount(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &TokenAccount{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate only TokenAddress.URL
	if err = api.validate.StructPartial(req, "URL"); err != nil {
		return NewValidatorError(err)
	}

	resp := &TokenAccount{}
	resp.URL = req.URL

	// Tendermint integration here

	return resp

}

// createTokenAccount creates Token Account
func (api *API) createTokenAccount(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &APIRequest{}
	data := &TokenAccount{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	resp := &TokenAccount{}

	// Tendermint integration here

	return resp

}

// getTokenTx returns Token Tx info
func (api *API) getTokenTx(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &TokenTx{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate only TokenTx.Hash
	if err = api.validate.StructPartial(req, "Hash"); err != nil {
		return NewValidatorError(err)
	}

	resp := &TokenTx{}

	// Tendermint integration here

	return resp
}

// createTokenTx creates Token Tx
func (api *API) createTokenTx(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &APIRequest{}
	data := &TokenTx{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	resp := &TokenTx{}

	// Tendermint integration here

	return resp

}
