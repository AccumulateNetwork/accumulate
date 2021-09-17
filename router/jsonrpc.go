package router

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	//"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

type API struct {
	port      int
	validate  *validator.Validate
	client    proto.ApiServiceClient
	query     *Query
	txBouncer *networks.Bouncer
}

// StartAPI starts new JSON-RPC server
func StartAPI(port int, q *Query, txBouncer *networks.Bouncer) *API {

	// fmt.Printf("Starting JSON-RPC API at http://localhost:%d\n", port)

	api := &API{}
	api.port = port
	api.validate = validator.New()
	api.query = q

	methods := jsonrpc2.MethodMap{
		// URL
		"get": api.getData,

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
		"faucet":               api.faucet,
	}

	apiHandler := jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))

	apiRouter := mux.NewRouter().StrictSlash(true)
	apiRouter.HandleFunc("/v1", apiHandler)

	proxyRouter := mux.NewRouter().StrictSlash(true)
	proxyRouter.HandleFunc(`/{url:[a-zA-Z0-9=\.\-\_\~\!\$\&\'\(\)\*\+\,\;\=\:\@\/]+}`, proxyHandler)

	// start JSON RPC API
	go func() {
		log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), apiRouter))
	}()

	// start REST proxy for JSON RPC API
	go func() {
		log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port+1), proxyRouter))
	}()

	return api

}

// getData returns Accumulate Object by URL
func (api *API) getData(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURL{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	resp, err := api.query.GetChainState(req.URL.AsString())

	if err != nil {
		return NewAccumulateError(err)
	}

	return resp
}

// getADI returns ADI info
func (api *API) getADI(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURL{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	resp, err := api.query.GetAdi(req.URL.AsString())

	if err != nil {
		return NewAccumulateError(err)
	}

	return resp
}

// createADI creates ADI
func (api *API) createADI(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &ADI{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	if err = json.Unmarshal(*req.Tx.Data, &data); err != nil {
		return NewValidatorError(err)
	}

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	submission, err := proto.Builder().
		AdiUrl(*req.Tx.Signer.URL.AsString()).
		Instruction(proto.AccInstruction_Identity_Creation).
		Type(types.ChainTypeAdi[:]). //The type is only needed for chain create messages
		Data(*req.Tx.Data).
		PubKey(req.Tx.Signer.PublicKey.Bytes()).
		Signature(req.Sig.Bytes()).
		Timestamp(req.Tx.Timestamp).
		Build()

	if err != nil {
		return NewSubmissionError(err)
	}

	//This client connects us to the router, the router will send the message to the correct BVC network
	resp, err := api.client.ProcessTx(context.Background(), submission)
	if err != nil {
		return NewAccumulateError(err)
	}

	return resp
}

// getToken returns Token info
func (api *API) getToken(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURL{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	//query tendermint
	resp, err := api.query.GetToken(req.URL.AsString())

	if err != nil {
		return NewAccumulateError(err)
	}

	return resp

}

// createToken creates Token
func (api *API) createToken(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &Token{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	if err = json.Unmarshal(*req.Tx.Data, &data); err != nil {
		return NewValidatorError(err)
	}

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	submission, err := proto.Builder().
		AdiUrl(*req.Tx.Signer.URL.AsString()).
		ChainUrl(*data.URL.AsString()). //this chain shouldn't exist yet
		Instruction(proto.AccInstruction_Token_Issue).
		Type(types.ChainTypeToken[:]). //Needed since this is a chain create messages
		Data(*req.Tx.Data).
		PubKey(req.Tx.Signer.PublicKey.Bytes()).
		Signature(req.Sig.Bytes()).
		Timestamp(req.Tx.Timestamp).
		Build()

	if err != nil {
		return NewSubmissionError(err)
	}

	//This client connects us to the router, the router will send the message to the correct BVC network
	//this uses grpc to route but that isn't necessary (and slow), we should develop a simple dispatcher
	//that contains a client connection to each BVC
	resp, err := api.client.ProcessTx(context.Background(), submission)
	if err != nil {
		return NewAccumulateError(err)
	}

	//Need to decide what kind of response is desired.
	return resp

}

// getTokenAccount returns Token Account info
func (api *API) getTokenAccount(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURL{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	taResp, err := api.query.GetTokenAccount(req.URL.AsString())
	if err != nil {
		return NewValidatorError(err)
	}

	return taResp

}

// createTokenAccount creates Token Account
func (api *API) createTokenAccount(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &TokenAccount{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	if err = json.Unmarshal(*req.Tx.Data, &data); err != nil {
		return NewValidatorError(err)
	}

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	submission, err := proto.Builder().
		AdiUrl(*req.Tx.Signer.URL.AsString()).
		ChainUrl(*data.URL.AsString()). // This chain shouldn't exist yet.
		Instruction(proto.AccInstruction_Token_URL_Creation).
		Type(types.ChainTypeToken[:]). // The type is only needed for chain create messages
		Data(*req.Tx.Data).
		PubKey(req.Tx.Signer.PublicKey.Bytes()).
		Signature(req.Sig.Bytes()).
		Timestamp(req.Tx.Timestamp).
		Build()

	if err != nil {
		return NewSubmissionError(err)
	}

	//This client connects us to the router, the router will send the message to the correct BVC network
	resp, err := api.client.ProcessTx(context.Background(), submission)
	if err != nil {
		return NewAccumulateError(err)
	}

	//Need to decide what the appropriate response should be.
	return resp
}

// getTokenTx returns Token Tx info
func (api *API) getTokenTx(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &TokenTx{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate only TokenTx.Hash (Assuming the hash is the txid)
	if err = api.validate.StructPartial(req, "Hash", "From"); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint's integration here
	resp, err := api.query.GetTokenTx(req.From.AsString(), req.Hash[:])
	if err != nil {
		return NewValidatorError(err)
	}

	return resp
}

// createTokenTx creates Token Tx
func (api *API) createTokenTx(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &TokenTx{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	// parse req.tx.data
	if err = json.Unmarshal(*req.Tx.Data, &data); err != nil {
		return NewValidatorError(err)
	}

	// validate request data
	if err = api.validate.Struct(data); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	submission, err := proto.Builder().
		AdiUrl(*req.Tx.Signer.URL.AsString()).
		ChainUrl(*data.From.AsString()). // This chain shouldn't exist yet.
		Instruction(proto.AccInstruction_Token_Transaction).
		Type(types.ChainTypeToken[:]). // The type is only needed for chain create messages
		Data(*req.Tx.Data).
		PubKey(req.Tx.Signer.PublicKey.Bytes()).
		Signature(req.Sig.Bytes()).
		Timestamp(req.Tx.Timestamp).
		Build()

	if err != nil {
		return NewSubmissionError(err)
	}

	//This client connects us to the router, the router will send the message to the correct BVC network
	resp, err := api.client.ProcessTx(context.Background(), submission)
	if err != nil {
		return NewAccumulateError(err)
	}

	//Need to decide what the appropriate response should be.
	return resp
}

// createTokenTx creates Token Tx
func (api *API) faucet(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURL{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return NewValidatorError(err)
	}

	adi, _, _ := types.ParseIdentityChainPath(req.URL.AsString())

	if err = anon.IsAcmeAddress(adi); err != nil {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("Invalid Anonymous ACME address %s: ", adi), err)
	}
	var destAccount types.String
	destAccount = types.String(adi)

	wallet := NewWalletEntry()
	fromAccount := types.String(wallet.Addr)

	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)

	txid := sha256.Sum256([]byte("fake txid"))

	tokenUrl := types.String("dc/ACME")

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(txid[:], &fromAccount, &destAccount)
	amtToDeposit := int64(10)                                //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * 100000000) // assume 8 decimal places
	deposit.TokenUrl = tokenUrl

	depData, err := deposit.MarshalBinary()
	gtx := new(proto.GenTransaction)
	gtx.Transaction = depData
	if err := gtx.SetRoutingChainID(*destAccount.AsString()); err != nil {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("bad url generated %s: ", adi), err)
	}
	dataToSign, err := gtx.MarshalBinary()
	if err != nil {
		return NewSubmissionError(err)
	}

	ed := new(proto.ED25519Sig)
	ed.Nonce = wallet.Nonce
	ed.PublicKey = wallet.Public()
	err = ed.Sign(wallet.PrivateKey, dataToSign)
	if err != nil {
		return NewSubmissionError(err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	_, err = api.txBouncer.SendTx(gtx)

	if err != nil {
		return NewAccumulateError(err)
	}

	data, err := gtx.Marshal()
	if err != nil {
		return NewAccumulateError(err)
	}

	txId := sha256.Sum256(data)
	return fmt.Sprintf("{\"txid\":\"%x\"}", txId)
}
