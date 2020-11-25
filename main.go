package main

import (
	"encoding/hex"
	"encoding/json"
	//pb "github.com/AccumulateNetwork/accumulated/proto"

	//	"errors"
	"flag"
	"fmt"
	"log"

	"context"
	"net/http"
	"os"

	//"github.com/AdamSLevy/jsonrpc2"
	"github.com/AdamSLevy/jsonrpc2/v14"

	// db "github.com/tendermint/tm-db"
	"path"

	"os/signal"
	"os/user"
	"syscall"

	//"github.com/dgraph-io/badger"
	"github.com/AccumulateNetwork/accumulated/tendermint"
	"github.com/AccumulateNetwork/accumulated/validator"

	"github.com/tendermint/tendermint/rpc/core"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	//"net/rpc/jsonrpc"
)

var ConfigFile [33]string
var WorkingDir [33]string
//var SpecialModeHeight int64 = 99999999999

func init() {

	usr,err := user.Current()
	if err != nil {
		log.Fatal( err )
		os.Exit(1)
	}

	flag.StringVar(&WorkingDir[0], "workingdir", usr.HomeDir +  "/.accumulate", "Path to data directory")
	flag.Parse()
	WorkingDir[1] = path.Join(WorkingDir[0],"/valacc/fct")
	WorkingDir[2] = path.Join(WorkingDir[0],"/valacc/ec")
	WorkingDir[3] = path.Join(WorkingDir[0],"/valacc/user")
	WorkingDir[0] = path.Join(WorkingDir[0],"/leader")
	ConfigFile[0] = path.Join(WorkingDir[0],"/config/config.toml")
	ConfigFile[1] = path.Join(WorkingDir[1],"/config/config.toml")
	ConfigFile[2] = path.Join(WorkingDir[2],"/config/config.toml")
	ConfigFile[3] = path.Join(WorkingDir[3],"/config/config.toml")
}

type factoid_tx struct {
    transaction []byte
}
type FactoidSubmit int
//
//func (t *FactoidSubmit) submit(r *http.Request, args *Args, result *Result) error {
//	log.Printf("factoid-submit\n")
//	//*result = Result(args.A * args.B)
//	return nil
//}
//
func factoid_submit(_ context.Context, params json.RawMessage) interface{} {

	var p struct {
		Transaction *string
	}

	type ret struct {
		Message string `json:"message"`
		Txid string    `json:"txid,omitempty"`
	}

	err := json.Unmarshal(params, &p)
	if err != nil {
		return ret{"transaction unmarshal error",""}
	}
	//send off the p.Transaction
	//rpctypes.WSRPCConnection()
	decoded, err := hex.DecodeString(*p.Transaction)

	res, err := core.BroadcastTxCommit(&rpctypes.Context{}, decoded)

//https://github.com/tendermint/tendermint/blob/27e8cea9ce06f6a75f0b26e6993bce139a5a9074/abci/example/kvstore/kvstore_test.go#L253
	//abcicli.NewClient(,grpc,true)
	// set up grpc app
//	kvstore = NewApplication()
//	gclient, gserver, err := makeGRPCClientServer(kvstore, "kvstore-grpc")
//	require.NoError(t, err)
//
//	t.Cleanup(func() {
//		if err := gserver.Stop(); err != nil {
//			t.Error(err)
//		}
//	})
//	t.Cleanup(func() {
//		if err := gclient.Stop(); err != nil {
//			t.Error(err)
//		}
//	})
//
//	runClientTests(t, gclient)
//}
	log.Printf(hex.EncodeToString(res.Hash))
	r := ret{"hello there",hex.EncodeToString(res.Hash) }
	//if err != nil {
	//	return &pb.Reply{Error: err.Error()} , nil
	//}


	//call tendermint
	//process response
	//ret{"Successfully submitted the transaction","aa8bac391e744340140ea0d95c7b37f9cc8a58601961bd751f5adb042af6f33b" }
	return r
}
// The RPC methods called in the JSON-RPC 2.0 specification examples.
func subtract(_ context.Context, params json.RawMessage) interface{} {
	// Parse either a params array of numbers or named numbers params.
	var a []float64
	if err := json.Unmarshal(params, &a); err == nil {
		if len(a) != 2 {
			return jsonrpc2.ErrorInvalidParams("Invalid number of array params")
		}
		return a[0] - a[1]
	}
	var p struct {
		Subtrahend *float64
		Minuend    *float64
	}
	if err := json.Unmarshal(params, &p); err != nil ||
		p.Subtrahend == nil || p.Minuend == nil {
		return jsonrpc2.ErrorInvalidParams(`Required fields "subtrahend" and ` +
			`"minuend" must be valid numbers.`)
	}
	return *p.Minuend - *p.Subtrahend
}
func sum(_ context.Context, params json.RawMessage) interface{} {
	var p []float64
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonrpc2.ErrorInvalidParams(err)
	}
	sum := float64(0)
	for _, x := range p {
		sum += x
	}
	return sum
}
func notifyHello(_ context.Context, _ json.RawMessage) interface{} {
	return ""
}
func getData(_ context.Context, _ json.RawMessage) interface{} {
	return []interface{}{"hello", 5}
}

func jsonrpcserver2(){
	//s := NewJSONRPCServer()
	//arith := new(Arith)
	//s.Register(arith)
	//http.Handle("/rpc", s)
	//http.ListenAndServe(":1234", nil)
	// Register RPC methods.
	methods := jsonrpc2.MethodMap{
		"subtract":     subtract,
		"sum":          sum,
		"notify_hello": notifyHello,
		"get_data":     getData,
		"factoid-submit": factoid_submit,
	}
	jsonrpc2.DebugMethodFunc = true
	handler := jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))
	http.ListenAndServe(":1234", handler)
}

func main() {

	fmt.Printf("Working dir: %v\n", WorkingDir[0])
	fmt.Printf("Working VM1 dir: %v\n", ConfigFile[1])
	fmt.Printf("Config File: %v\n", ConfigFile[0])
	fmt.Printf("Config File VM1: %v\n", ConfigFile[1])

	n := len(os.Args)
	for i := 0; i<n; i++ {
    	switch os.Args[i] {
		case "init":
			tendermint.Initialize("directory-block-leader", "tcp://127.0.0.1:26600","tcp://127.0.0.1:26601",ConfigFile[0],WorkingDir[0])
			tendermint.Initialize("accumulator-fct", "tcp://127.0.0.1:26610","tcp://127.0.0.1:26611",ConfigFile[1],WorkingDir[1])
			tendermint.Initialize("accumulator-ec", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621",ConfigFile[2],WorkingDir[2])
			tendermint.Initialize("accumulator-user", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631",ConfigFile[3],WorkingDir[3])
//			tendermint.Initialize("vm2", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621",ConfigFile[2],WorkingDir[2])
//			tendermint.Initialize("vm3", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631",ConfigFile[3],WorkingDir[3])
			os.Exit(0)
		}
	}


	//create a directory block leader
	app := tendermint.NewDirectoryBlockLeader()

	go app.Start(ConfigFile[0],WorkingDir[0])

	//make a factoid validator/accumulator
	//create a Factoid validator
	val := validator.NewFactoidValidator()

	//create a AccumulatorVM
	accvm1 := tendermint.NewAccumulatorVMApplication(ConfigFile[1],WorkingDir[1])
	accvm1.AddValidator(&val.ValidatorContext)

	go accvm1.Start()

	go jsonrpcserver2()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}