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
		// identity
		"identity":        api.getIdentity,
		"identity-create": api.createIdentity,

		// token
		"token":                api.getToken,
		"token-create":         api.createToken,
		"token-address":        api.getTokenAddress,
		"token-address-create": api.createTokenAddress,
		"token-tx-create":      api.createTokenTx,
	}

	apiHandler := jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))
	http.HandleFunc("/v1", apiHandler)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), nil))

	return api

}

// getIdentity returns Identity info
func (api *API) getIdentity(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &Identity{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate only Identity.URL
	if err = api.validate.StructPartial(req, "URL"); err != nil {
		return NewValidatorError(err)
	}

	resp := &Identity{}
	resp.URL = req.URL

	// Tendermint integration here

	return resp
}

// createIdentity creates Identity
func (api *API) createIdentity(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &CreateIdentityRequest{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	resp := &Identity{}
	resp.URL = req.Identity.URL
	resp.PublicKeyHash = req.Identity.PublicKeyHash

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
	req := &CreateTokenRequest{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	resp := &Token{}
	resp.URL = req.Token.URL
	resp.Precision = req.Token.Precision
	resp.Symbol = req.Token.Symbol

	// Tendermint integration here

	return resp

}

// getTokenAddress returns Token Address info
func (api *API) getTokenAddress(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &TokenAddress{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate only TokenAddress.URL
	if err = api.validate.StructPartial(req, "URL"); err != nil {
		return NewValidatorError(err)
	}

	resp := &TokenAddress{}
	resp.URL = req.URL

	// Tendermint integration here

	return resp

}

// createTokenAddress creates Token Address
func (api *API) createTokenAddress(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &CreateTokenAddressRequest{}

	if err = json.Unmarshal(params, &req); err != nil {
		return ErrorInvalidRequest
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	resp := &TokenAddress{}
	resp.URL = req.TokenAddress.URL
	resp.TokenURL = req.TokenAddress.TokenURL

	// Tendermint integration here

	return resp

}

// createTokenTx creates Token Tx
func (api *API) createTokenTx(_ context.Context, params json.RawMessage) interface{} {

	return nil

}
