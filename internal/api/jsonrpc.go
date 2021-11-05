package api

import (
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/AccumulateNetwork/accumulated"
	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/genesis"
	accurl "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	promapi "github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	abci "github.com/tendermint/tendermint/abci/types"
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

	v, err := protocol.NewValidator()
	if err != nil {
		return nil, err
	}

	api := &API{}
	api.config = config
	api.validate = v
	api.query = q

	methods := jsonrpc2.MethodMap{
		// Metadata
		"version": api.version,
		"metrics": api.Metrics,

		// URL
		"get":           api.getData,
		"get-directory": api.GetDirectory,
		"directory":     api.GetDirectory,

		// Chain
		"chain": api.getDataByChainId,

		// ADI
		"adi":        api.getADI,
		"adi-create": api.createADI,

		// Key management
		"sig-spec":              api.getSigSpec,
		"create-sig-spec":       api.createSigSpec,
		"sig-spec-group":        api.getSigSpecGroup,
		"create-sig-spec-group": api.createSigSpecGroup,
		"update-key-page":       api.updateKeyPage,
		"update-sig-spec":       api.updateKeyPage,

		// Updated Key Management
		"key-page-create": api.createSigSpec,
		"key-page-update": api.updateKeyPage,
		"key-page":        api.getSigSpec,
		"key-book-create": api.createSigSpec,
		"key-book":        api.getSigSpecGroup,

		// token
		"token":                 api.getToken,
		"token-create":          api.createToken,
		"token-account":         api.getTokenAccount,
		"token-account-create":  api.createTokenAccount,
		"token-account-history": api.getTokenAccountHistory,
		"token-tx":              api.getTokenTx,
		"token-tx-create":       api.createTokenTx,

		// faucet
		"faucet": api.Faucet,

		// credits
		"add-credits": api.addCredits,
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

func (api *API) prepareCreate(params json.RawMessage, data encoding.BinaryMarshaler, fields ...string) (*acmeapi.APIRequestRaw, []byte, error) {
	var err error
	req := &acmeapi.APIRequestRaw{}

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		return nil, nil, err
	}

	// validate request
	if err = api.validate.Struct(req); err != nil {
		return nil, nil, err
	}

	// parse req.tx.data
	if err = json.Unmarshal(*req.Tx.Data, &data); err != nil {
		return nil, nil, err
	}

	// validate request data
	if len(fields) == 0 {
		if err = api.validate.Struct(data); err != nil {
			return nil, nil, err
		}
	} else {
		if err = api.validate.StructPartial(data, fields...); err != nil {
			return nil, nil, err
		}
	}

	// Tendermint integration here
	var payload []byte
	if payload, err = data.MarshalBinary(); err != nil {
		return nil, nil, err
	}

	return req, payload, nil
}

// createSigSpec creates a signature specification
func (api *API) createSigSpec(_ context.Context, params json.RawMessage) interface{} {
	data := &protocol.CreateSigSpec{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "sigSpec"
	return ret
}

// updateKeyPage adds, removes, or updates key in a key page
func (api *API) updateKeyPage(_ context.Context, params json.RawMessage) interface{} {
	data := &protocol.UpdateKeyPage{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "sigSpec"
	return ret
}

// createSigSpecGroup creates a signature specification group
func (api *API) createSigSpecGroup(_ context.Context, params json.RawMessage) interface{} {
	data := &protocol.CreateSigSpecGroup{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "sigSpecGroup"
	return ret
}

func (api *API) addCredits(_ context.Context, params json.RawMessage) interface{} {
	data := &protocol.AddCredits{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "addCredits"
	return ret
}

func (api *API) unmarshalRequest(params json.RawMessage, data interface{}) error {
	err := json.Unmarshal(params, data)
	if err != nil {
		return err
	}

	// validate URL
	err = api.validate.Struct(data)
	if err != nil {
		return err
	}

	return nil
}

func (api *API) prepareGetByChainId(params json.RawMessage) (*acmeapi.APIRequestChainId, error) {
	req := &acmeapi.APIRequestChainId{}
	return req, api.unmarshalRequest(params, req)
}

func (api *API) prepareGet(params json.RawMessage) (*acmeapi.APIRequestURL, error) {
	req := &acmeapi.APIRequestURL{}
	return req, api.unmarshalRequest(params, req)
}

func (api *API) get(params json.RawMessage, expect ...types.ChainType) interface{} {
	req, err := api.prepareGet(params)
	if err != nil {
		return validatorError(err)
	}

	r, err := api.query.QueryByUrl(string(req.URL))
	if err != nil {
		return accumulateError(err)
	}

	resp, err := unmarshalQueryResponse(r.Response, expect...)
	if err != nil {
		return accumulateError(err)
	}

	return resp
}

// getData returns Accumulate Object by URL
func (api *API) getData(_ context.Context, params json.RawMessage) interface{} {
	req, err := api.prepareGet(params)
	if err != nil {
		return validatorError(err)
	}

	resp, err := api.query.GetChainStateByUrl(string(req.URL))
	if err != nil {
		return accumulateError(err)
	}

	return resp
}

// getData returns Accumulate Object by URL
func (api *API) getDataByChainId(_ context.Context, params json.RawMessage) interface{} {
	req := &acmeapi.APIRequestChainId{}
	err := api.unmarshalRequest(params, req)
	if err != nil {
		return validatorError(err)
	}

	resp, err := api.query.GetChainStateByChainId(req.ChainId)
	if err != nil {
		return accumulateError(err)
	}

	return resp
}

func (api *API) getSigSpec(_ context.Context, params json.RawMessage) interface{} {
	return api.get(params, types.ChainTypeSigSpec)
}

func (api *API) getSigSpecGroup(_ context.Context, params json.RawMessage) interface{} {
	return api.get(params, types.ChainTypeSigSpecGroup)
}

// getADI returns ADI info
func (api *API) getADI(_ context.Context, params json.RawMessage) interface{} {
	req, err := api.prepareGet(params)
	if err != nil {
		return validatorError(err)
	}

	resp, err := api.query.GetAdi(*req.URL.AsString())
	if err != nil {
		return accumulateError(err)
	}

	return resp
}

// createADI creates ADI
func (api *API) createADI(_ context.Context, params json.RawMessage) interface{} {
	data := &protocol.IdentityCreate{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "tokenTx"
	return ret
}

// getToken returns Token info
func (api *API) getToken(_ context.Context, params json.RawMessage) interface{} {
	req, err := api.prepareGet(params)
	if err != nil {
		return validatorError(err)
	}

	resp, err := api.query.GetToken(*req.URL.AsString())
	if err != nil {
		return accumulateError(err)
	}

	return resp

}

// createToken creates Token
func (api *API) createToken(_ context.Context, params json.RawMessage) interface{} {
	data := &acmeapi.Token{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "token"
	return ret
}

// getTokenAccount returns Token Account info
func (api *API) getTokenAccount(_ context.Context, params json.RawMessage) interface{} {
	req, err := api.prepareGet(params)
	if err != nil {
		return validatorError(err)
	}

	resp, err := api.query.GetTokenAccount(*req.URL.AsString())
	if err != nil {
		return accumulateError(err)
	}

	return resp
}

// getTokenAccountHistory returns tx history for Token Account
func (api *API) getTokenAccountHistory(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURLPagination{}

	if err = json.Unmarshal(params, &req); err != nil {
		return validatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return validatorError(err)
	}

	// Tendermint integration here
	ret, err := api.query.GetTransactionHistory(*req.URL.AsString(), req.Start, req.Limit)
	if err != nil {
		return accumulateError(err)
	}

	ret.Type = "tokenAccountHistory"
	return ret
}

func (api *API) sendTx(req *acmeapi.APIRequestRaw, payload []byte) *acmeapi.APIDataResponse {
	tx := new(transactions.GenTransaction)
	tx.Transaction = payload

	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = string(req.Tx.Sponsor)
	tx.SigInfo.Nonce = req.Tx.Signer.Nonce
	tx.SigInfo.MSHeight = req.Tx.KeyPage.Height
	tx.SigInfo.PriorityIdx = req.Tx.KeyPage.Index

	ed := new(transactions.ED25519Sig)
	ed.Nonce = req.Tx.Signer.Nonce
	ed.PublicKey = req.Tx.Signer.PublicKey[:]
	ed.Signature = req.Tx.Sig.Bytes()

	tx.Signature = append(tx.Signature, ed)

	return api.broadcastTx(req.Wait, tx)
}

func (api *API) broadcastTx(wait bool, tx *transactions.GenTransaction) *acmeapi.APIDataResponse {
	// Disable websocket based behavior if it is not enabled
	if !api.config.EnableSubscribeTX {
		wait = false
	}

	ret := &acmeapi.APIDataResponse{}
	var msg json.RawMessage

	var done chan abci.TxResult
	if wait {
		done = make(chan abci.TxResult, 1)
	}

	txInfo, err := api.query.BroadcastTx(tx, done)
	if err != nil {
		msg = []byte(fmt.Sprintf("{\"error\":\"%v\"}", err))
		ret.Data = &msg
		return ret
	}

	resp := <-api.query.BatchSend()

	resolved, err := resp.ResolveTransactionResponse(txInfo)
	if err != nil {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"error\":\"%v\"}", tx.TransactionHash(), err))
		ret.Data = &msg
		return ret
	}

	if resolved.Code != 0 || len(resolved.MempoolError) != 0 {
		msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"log\":\"%s\",\"hash\":\"%x\",\"code\":\"%d\",\"mempool\":\"%s\",\"codespace\":\"%s\"}", tx.TransactionHash(), resolved.Log, resolved.Hash, resolved.Code, resolved.MempoolError, resolved.Codespace))
		ret.Data = &msg
		return ret
	}

	if wait {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()

		select {
		case txr := <-done:
			resolved := txr.Result
			hash := sha256.Sum256(txr.Tx)
			if txr.Result.Code != 0 {
				msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"log\":\"%s\",\"hash\":\"%x\",\"code\":\"%d\",\"codespace\":\"%s\"}", tx.TransactionHash(), resolved.Log, hash, resolved.Code, resolved.Codespace))
				ret.Data = &msg
				return ret
			}

		case <-timer.C:
		}
	}

	msg = []byte(fmt.Sprintf("{\"txid\":\"%x\",\"hash\":\"%x\",\"codespace\":\"%s\"}", tx.TransactionHash(), resolved.Hash, resolved.Codespace))
	ret.Data = &msg
	return ret

}

// createTokenAccount creates Token Account
func (api *API) createTokenAccount(_ context.Context, params json.RawMessage) interface{} {
	data := &protocol.TokenAccountCreate{}
	req, payload, err := api.prepareCreate(params, data)
	if err != nil {
		return validatorError(err)
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
		return validatorError(err)
	}

	// validate only TokenTx.Hash (Assuming the hash is the txid)
	if err = api.validate.StructPartial(req, "Hash"); err != nil {
		return validatorError(err)
	}

	// Tendermint's integration here
	resp, err := api.query.GetTransaction(req.Hash[:])
	if err != nil {
		return accumulateError(err)
	}

	if resp.Type != "tokenTx" && resp.Type != "syntheticTokenDeposit" {
		return validatorError(fmt.Errorf("transaction type is %s and not a token transaction", resp.Type))
	}

	return resp
}

// createTokenTx creates Token Tx
func (api *API) createTokenTx(_ context.Context, params json.RawMessage) interface{} {
	data := &acmeapi.TokenTx{}
	req, payload, err := api.prepareCreate(params, data, "From", "To")
	if err != nil {
		return validatorError(err)
	}

	ret := api.sendTx(req, payload)
	ret.Type = "tokenTx"
	return ret
}

// faucet API call (testnet only)
func (api *API) Faucet(_ context.Context, params json.RawMessage) interface{} {

	var err error
	req := &acmeapi.APIRequestURL{}

	if err = json.Unmarshal(params, &req); err != nil {
		return validatorError(err)
	}

	// validate URL
	if err = api.validate.Struct(req); err != nil {
		return validatorError(err)
	}

	u, err := accurl.Parse(*req.URL.AsString())
	if err != nil {
		return validatorError(err)
	}

	destAccount := types.String(u.String())
	addr, tok, err := protocol.ParseAnonymousAddress(u)
	switch {
	case err != nil:
		return jsonrpc2.NewError(ErrCodeValidation, "Invalid token account", fmt.Errorf("error parsing %q: %v", u, err))
	case addr == nil:
		return jsonrpc2.NewError(ErrCodeNotLiteAccount, "Invalid token account", fmt.Errorf("%q is not a lite account", u))
	case !protocol.AcmeUrl().Equal(tok):
		return jsonrpc2.NewError(ErrCodeNotAcmeAccount, "Invalid token account", fmt.Errorf("%q is not an ACME account", u))
	}

	tx := acmeapi.TokenTx{}
	tx.From.String = types.String(genesis.FaucetWallet.Addr)
	tx.AddToAccount(destAccount, 1000000000)

	txData, err := tx.MarshalBinary()

	genesis.FaucetWallet.Nonce = uint64(time.Now().UnixNano())
	gtx := new(transactions.GenTransaction)
	gtx.Routing = genesis.FaucetUrl.Routing()
	gtx.ChainID = genesis.FaucetUrl.ResourceChain()
	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.SigInfo.URL = *tx.From.String.AsString()
	gtx.SigInfo.Nonce = genesis.FaucetWallet.Nonce
	gtx.SigInfo.MSHeight = 1
	gtx.SigInfo.PriorityIdx = 0
	gtx.Transaction = txData
	if err := gtx.SetRoutingChainID(); err != nil {
		return jsonrpc2.NewError(-32802, fmt.Sprintf("bad url generated %s: ", destAccount), err)
	}
	dataToSign := gtx.TransactionHash()

	ed := new(transactions.ED25519Sig)
	err = ed.Sign(genesis.FaucetWallet.Nonce, genesis.FaucetWallet.PrivateKey, dataToSign)
	if err != nil {
		return submissionError(err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	return api.broadcastTx(req.Wait, gtx)
}

func (api *API) version(_ context.Context, params json.RawMessage) interface{} {
	ret := acmeapi.APIDataResponse{}
	ret.Type = "version"

	data, _ := json.Marshal(map[string]interface{}{
		"version":        accumulated.Version,
		"commit":         accumulated.Commit,
		"versionIsKnown": accumulated.IsVersionKnown(),
	})
	raw := json.RawMessage(data)
	ret.Data = &raw

	return ret
}

// Metrics returns Metrics for explorer (tps, etc.)
func (api *API) Metrics(_ context.Context, params json.RawMessage) interface{} {
	req := new(protocol.MetricsRequest)
	err := api.unmarshalRequest(params, req)
	if err != nil {
		return validatorError(err)
	}

	c, err := promapi.NewClient(promapi.Config{
		// TODO Change this to an AWS Prometheus instance
		Address: "http://18.119.26.7:9090",
	})
	if err != nil {
		return internalError(err)
	}
	papi := prometheus.NewAPI(c)

	if req.Duration == 0 {
		req.Duration = time.Hour
	}

	res := new(protocol.MetricsResponse)
	switch req.Metric {
	case "tps":
		query := fmt.Sprintf(metricTPS, req.Duration)
		v, _, err := papi.Query(context.Background(), query, time.Now())
		if err != nil {
			return metricsQueryError(fmt.Errorf("query failed: %w", err))
		}
		vec, ok := v.(model.Vector)
		if !ok {
			return ErrMetricsNotAVector
		}
		if len(vec) == 0 {
			return ErrMetricsVectorEmpty
		}
		res.Value = vec[0].Value / model.SampleValue(req.Duration.Seconds())
	default:
		return validatorError(fmt.Errorf("%q is not a valid metric", req.Metric))
	}

	ret := acmeapi.APIDataResponse{}
	ret.Type = "metrics"

	data, _ := json.Marshal(res)
	raw := json.RawMessage(data)
	ret.Data = &raw

	return ret
}

// GetDirectory returns ADI directory entries
func (api *API) GetDirectory(_ context.Context, params json.RawMessage) interface{} {
	req, err := api.prepareGet(params)
	if err != nil {
		return validatorError(err)
	}

	resp, err := api.query.GetDirectory(*req.URL.AsString())
	if err != nil {
		return accumulateError(err)
	}

	return resp
}
