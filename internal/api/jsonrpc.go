package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/AccumulateNetwork/accumulated/config"
	accurl "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/ybbus/jsonrpc/v2"
)

type API struct {
	config   *config.API
	validate *validator.Validate
	query    *Query
	jsonrpc  jsonrpc.RPCClient
}

// StartAPI starts new JSON-RPC server
func StartAPI(config *config.API, q *Query) (*API, error) {

	// fmt.Printf("Starting JSON-RPC API at http://localhost:%d\n", port)

	api := &API{}
	api.config = config
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
	proxyRouter.HandleFunc(`/{url:[a-zA-Z0-9=\.\-\_\~\!\$\&\'\(\)\*\+\,\;\=\:\@\/]+}`, api.proxyHandler)

	rpcUrl, err := url.Parse(config.JSONListenAddress)
	if err != nil {
		return nil, err
	}
	rpcUrl.Scheme = "http"
	rpcUrl.Path = "/v1"
	api.jsonrpc = jsonrpc.NewClient(rpcUrl.String())

	// start JSON RPC API
	go listenAndServe("JSONRPC API", config.JSONListenAddress, apiRouter)

	// start REST proxy for JSON RPC API
	go listenAndServe("REST API", config.RESTListenAddress, proxyRouter)

	return api, nil
}

// proxyHandler makes JSON-RPC API request
func (api *API) proxyHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("Content-Type", "application/json")

	// make "get" request to JSON RPC API
	fmt.Printf("=============== proxyHandler Is going to send : %s ===========\n\n\n", r.URL)
	params := &acmeapi.APIRequestURL{URL: types.String(r.URL.String()[1:])}

	result, err := api.jsonrpc.Call("get", params)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
	}

	response, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
	}

	fmt.Fprintf(w, "%s", response)
}

func listenAndServe(label, address string, handler http.Handler) {
	if address == "" {
		log.Fatalf("Address for %s is empty", label)
	}

	u, err := url.Parse(address)
	if err != nil {
		log.Fatal(err)
	}
	if u.Scheme == "" {
		log.Fatalf("Address for %s is missing a scheme: %q", label, address)
	}
	if u.Scheme != "tcp" {
		log.Fatalf("Failed to start HTTP server for %s: unsupported scheme %q", label, address)
	}

	err = http.ListenAndServe(u.Host, handler)
	if err != nil {
		log.Fatal(err)
	}
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
	resp, err := api.query.GetChainStateByUrl(string(req.URL))

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
	resp, err := api.query.GetAdi(*req.URL.AsString())

	if err != nil {
		return NewAccumulateError(err)
	}

	return resp
}

// createADI creates ADI
func (api *API) createADI(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &acmeapi.ADI{}

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
	var payload types.Bytes
	if payload, err = data.MarshalBinary(); err != nil {
		return NewValidatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "tokenTx"
	return ret
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
	resp, err := api.query.GetToken(*req.URL.AsString())

	if err != nil {
		return NewAccumulateError(err)
	}

	return resp

}

// createToken creates Token
func (api *API) createToken(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &acmeapi.Token{}

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
	var payload types.Bytes
	if payload, err = data.MarshalBinary(); err != nil {
		return NewValidatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "token"
	return ret
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
	taResp, err := api.query.GetTokenAccount(*req.URL.AsString())
	if err != nil {
		return NewValidatorError(err)
	}

	return taResp
}

func (api *API) sendTx(req *acmeapi.APIRequestRaw, payload []byte) *acmeapi.APIDataResponse {
	genTx, err := acmeapi.NewAPIRequest(&req.Sig, req.Tx.Signer, uint64(req.Tx.Timestamp), payload)

	ret := &acmeapi.APIDataResponse{}
	var msg json.RawMessage

	if err != nil {
		msg = []byte(fmt.Sprintf("{\"error\":\"%v\"}", err))
		ret.Data = &msg
		return ret
	}
	txInfo, err := api.query.BroadcastTx(genTx)
	if err != nil {
		msg = []byte(fmt.Sprintf("{\"error\":\"%v\"}", err))
		ret.Data = &msg
		return ret
	}

	stat := api.query.BatchSend()
	resp := <-stat

	resolved, err := resp.ResolveTransactionResponse(txInfo)
	if err != nil {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"error\":\"%v\"}", genTx.TransactionHash(), err))
		ret.Data = &msg
		return ret
	}

	if resolved.Code != 0 || len(resolved.MempoolError) != 0 {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"log\":\"%s\",\"hash\":\"%x\",\"code\":\"%d\",\"mempool\":\"%s\",\"codespace\":\"%s\"}", genTx.TransactionHash(), resolved.Log, resolved.Hash, resolved.Code, resolved.MempoolError, resolved.Codespace))
	} else {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"hash\":\"%x\",\"codespace\":\"%s\"}", genTx.TransactionHash(), resolved.Hash, resolved.Codespace))
	}
	ret.Data = &msg
	return ret
}

// createTokenAccount creates Token Account
func (api *API) createTokenAccount(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &acmeapi.TokenAccount{}

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
	var payload types.Bytes
	if payload, err = data.MarshalBinary(); err != nil {
		return NewValidatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "tokenAccount"
	return ret
}

// getTokenTx returns Token Tx info
func (api *API) getTokenTx(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.TokenTxRequest{}

	if err = json.Unmarshal(params, &req); err != nil {
		return NewValidatorError(err)
	}

	// validate only TokenTx.Hash (Assuming the hash is the txid)
	if err = api.validate.StructPartial(req, "Hash"); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint's integration here
	resp, err := api.query.GetTransaction(req.Hash[:])
	if err != nil {
		return NewValidatorError(err)
	}

	if resp.Type != "tokenTx" && resp.Type != "syntheticTokenDeposit" {
		return NewValidatorError(fmt.Errorf("transaction type is %s and not a token transaction", resp.Type))
	}

	return resp
}

// createTokenTx creates Token Tx
func (api *API) createTokenTx(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestRaw{}
	data := &acmeapi.TokenTx{}

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
	if err = api.validate.StructPartial(data, "From", "To"); err != nil {
		return NewValidatorError(err)
	}

	// Tendermint integration here
	var payload types.Bytes
	if payload, err = data.MarshalBinary(); err != nil {
		return NewValidatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "tokenTx"
	return ret
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

	u, err := accurl.Parse(*req.URL.AsString())
	if err != nil {
		return NewValidatorError(err)
	}

	destAccount := types.String(u.String())
	addr, tok, err := protocol.ParseAnonymousAddress(u)
	if err != nil {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("Invalid Anonymous ACME address %s: ", destAccount), err)
	} else if addr == nil {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("Invalid Anonymous ACME address %s: ", destAccount), errors.New("not an anonymous account URL"))
	} else if !protocol.AcmeUrl().Equal(tok) {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("Invalid Anonymous ACME address %s: ", destAccount), errors.New("wrong token URL"))
	}

	wallet := NewWalletEntry()
	fromAccount := types.String(wallet.Addr)

	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	txid := sha256.Sum256([]byte("faucet"))

	tokenUrl := types.String(protocol.AcmeUrl().String())

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(txid[:], &fromAccount, &destAccount)
	amtToDeposit := int64(10)                                //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * 100000000) // assume 8 decimal places
	deposit.TokenUrl = tokenUrl

	depData, err := deposit.MarshalBinary()
	gtx := new(transactions.GenTransaction)
	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.SigInfo.URL = *destAccount.AsString()
	gtx.SigInfo.Nonce = wallet.Nonce
	gtx.Transaction = depData
	if err := gtx.SetRoutingChainID(); err != nil {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("bad url generated %s: ", destAccount), err)
	}
	dataToSign := gtx.TransactionHash()

	ed := new(transactions.ED25519Sig)
	err = ed.Sign(wallet.Nonce, wallet.PrivateKey, dataToSign)
	if err != nil {
		return NewSubmissionError(err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	txInfo, err := api.query.BroadcastTx(gtx)
	if err != nil {
		return NewAccumulateError(err)
	}

	stat := api.query.BatchSend()

	res := <-stat

	ret := acmeapi.APIDataResponse{}
	ret.Type = "faucet"

	var msg json.RawMessage
	resolved, err := res.ResolveTransactionResponse(txInfo)
	if err != nil {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"error\":\"%v\"}", gtx.TransactionHash(), err))
		ret.Data = &msg
		return ret
	}

	if resolved.Code != 0 || len(resolved.MempoolError) != 0 {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"log\":\"%s\",\"hash\":\"%x\",\"code\":\"%d\",\"mempool\":\"%s\",\"codespace\":\"%s\"}", gtx.TransactionHash(), resolved.Log, resolved.Hash, resolved.Code, resolved.MempoolError, resolved.Codespace))
	} else {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"hash\":\"%x\",\"codespace\":\"%s\"}", gtx.TransactionHash(), resolved.Hash, resolved.Codespace))
	}
	ret.Data = &msg
	return &ret
}
