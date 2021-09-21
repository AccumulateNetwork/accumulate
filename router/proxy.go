package router

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/gorilla/mux"
	"github.com/ybbus/jsonrpc/v2"
)

// proxyHandler makes JSON-RPC API request
func proxyHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Add("Content-Type", "application/json")

	// create new JSON RPC client
	c := jsonrpc.NewClient("http://localhost:34000/v1")

	// make "get" request to JSON RPC API
	vars := mux.Vars(r)
	params := &acmeapi.APIRequestGet{}

	if types.String(vars["url"]) != "" {
		fmt.Printf("=============== proxyHandler Is going to send : %s ===========\n\n\n", vars["url"])
		params.URL = types.String(vars["url"])
	}

	if types.String(vars["tx"]) != "" {
		fmt.Printf("=============== proxyHandler Is going to send : %s ===========\n\n\n", vars["tx"])
		txhash := types.Bytes32{}
		txhash.FromString(vars["tx"])
		params.Hash = txhash
	}

	result, err := c.Call("get", params)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
	}

	response, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
	}

	fmt.Fprintf(w, "%s", response)
}
