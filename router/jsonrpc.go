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
)

type API struct {
	port int
}

var APISubmitInstructions = [...]string{
	"identity",               // ex "identity-create"
	"identity-key-update",    // ex "key-update"
	"identity-token-account", // ex "token-url-create"
	"token",                  // ex "token-issue"
	"token-tx",
	"chain", // ex "data-chain-create"
	"entry", // ex "data-entry"
	"scratch-chain",
	"scratch-entry",
}

var (
	ErrorMissingInstruction = jsonrpc2.NewError(-32801, "Missing Instruction", "parameter `instruction` is missing")
	ErrorInvalidInstruction = jsonrpc2.NewError(-32802, "Invalid Instruction", "instruction is invalid or deprecated")
)

// Temporarily put API related structs here, need to move into separate package after refactoring
type Status struct {
	Height int `json:"height" validate:"required,int"`
}

// StartJSONRPCAPI starts new JSON-RPC server
func StartJSONRPCAPI(port int) error {

	fmt.Printf("Starting JSON-RPC API at http://localhost:%d\n", port)

	methods := jsonrpc2.MethodMap{

		// node and network
		"status": status,

		// node and network
		"submit": submit,

		// query
		"query":      query,
		"deep-query": deepQuery,
	}

	apiHandler := jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))
	http.HandleFunc("/v1", apiHandler)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), nil))
	return nil

}

// status returns node status
func status(_ context.Context, params json.RawMessage) interface{} {

	resp := &Status{}
	resp.Height = 0

	return resp
}

// submit submits new data on blockchain
func submit(_ context.Context, params json.RawMessage) interface{} {

	instr, err := parseAPIInstruction(params)
	if err.Code != 0 {
		return ErrorInvalidInstruction
	}

	return instr
}

// query retrieves data from blockchain
func query(_ context.Context, params json.RawMessage) interface{} {

	return nil
}

// deepQuery retrieves cryptographic receipt of the current state
func deepQuery(_ context.Context, params json.RawMessage) interface{} {

	return nil
}
