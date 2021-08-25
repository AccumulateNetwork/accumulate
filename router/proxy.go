package router

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/ybbus/jsonrpc/v2"
)

// proxyHandler makes JSON-RPC API request
func proxyHandler(w http.ResponseWriter, r *http.Request) {

	// create new JSON RPC client
	c := jsonrpc.NewClient("http://localhost:25999/v1")

	// make "get" request to JSON RPC API
	vars := mux.Vars(r)
	params := &APIURLRequest{URL: vars["url"]}

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
