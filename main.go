package main

import (
	"encoding/hex"
	"encoding/json"
	kv "github.com/AccumulateNetwork/accumulated/example/kvstore"
	pb "github.com/AccumulateNetwork/accumulated/proto"
	proto1 "github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	cfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"sync"
	"time"

	//pb "github.com/AccumulateNetwork/accumulated/proto"
	abcicli "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/rpc/core"
	//grpccore "github.com/tendermint/tendermint/rpc/grpc"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
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
	//"net/rpc/jsonrpc"

	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"

	abciserver "github.com/tendermint/tendermint/abci/server"
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

func makeGRPCClientServer(app types.Application, name string) (abcicli.Client, service.Service, error) {
	// Start the listener
	socket := fmt.Sprintf("unix://%s.sock", name)
	logger := tmlog.TestingLogger()

	gapp := types.NewGRPCApplication(app)
	server := abciserver.NewGRPCServer(socket, gapp)
	server.SetLogger(logger.With("module", "abci-server"))
	if err := server.Start(); err != nil {
		return nil, nil, err
	}

	client := abcicli.NewGRPCClient(socket, true)
	client.SetLogger(logger.With("module", "abci-client"))
	if err := client.Start(); err != nil {
		if err := server.Stop(); err != nil {
			return nil, nil, err
		}
		return nil, nil, err
	}
	return client, server, nil
}
type factomapi struct {
	//client grpccore.BroadcastAPIClient
	client abcicli.Client
}

func NewFactomAPI(client abcicli.Client) (r *factomapi){/*client grpccore.BroadcastAPIClient*/
	r = &factomapi{client}
	return r
}

type factoid_tx struct {
    transaction []byte
}
type FactoidSubmit int

var counter int
var mm sync.Mutex
var gtimer time.Time

func (app *factomapi) factoid_submit(ctx context.Context, params json.RawMessage) interface{} {
	mm.Lock()
	if counter == 0 {
		gtimer = time.Now()

	}
	counter = counter + 1
	mm.Unlock()
	start := time.Now()
	start0 := start
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

	//ctx := context.Background()

	sig, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")


	duration0 := time.Since(start)
	vr := &pb.ValRequest{15,1,0,0,0,decoded,sig}
	msg, err := proto1.Marshal(vr)
	type delivertx struct {
		Tx []byte `json:"tx"`
	}
	dtx := delivertx{msg }
	duration1 := time.Since(start)
	start = time.Now()

	var c jsonrpc2.Client

	var result int
	if true {
		err = c.Request(nil, "http://localhost:26611", "broadcast_tx_sync", dtx, &result)
	} else {
		//if _, ok := err.(jsonrpc2.Error); ok {
		//	// received Error Request
		//}
		//if err != nil {
		//	// some JSON marshaling or network error
		//}
		//fmt.Printf("The sum of %v is %v.\n", params, result)
		//
		//
		app.client.CheckTxSync(types.RequestCheckTx{Tx: msg})
		app.client.DeliverTxAsync(types.RequestDeliverTx{msg})
		//app.client.BroadcastTx(ctx,&grpccore.RequestBroadcastTx{msg}) //app.client.DeliverTxSync(types.RequestDeliverTx{Tx: decoded})
		//if err2 != nil {
		//	return ret{"broadcast tx error",""}
		//}
		//res.GetDeliverTx()
	}
	duration2 := time.Since(start)


	//if err != nil {
	//	return &pb.Reply{Error: err.Error()} , nil
	//}

	//app.client.DeliverTxSync(ctx, types.RequestDeliverTx{decoded})
		//.BroadcastTxCommit(&rpctypes.Context{}, decoded)

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

	//log.Printf(hex.EncodeToString(res.GetData()))//res.DeliverTx.Data))

	//if err != nil {
	//	return &pb.Reply{Error: err.Error()} , nil
	//}

	durationT := time.Since(start0)
	mm.Lock()
	cum := time.Since(gtimer)
	r := ret{fmt.Sprintf("%d: (%f,cumulative: %f s) Hello there",counter,
		float64(durationT.Microseconds())/1000.0,float64(cum.Microseconds())/1000000.0),
		""}//,hex.EncodeToString(res.Data) }

	fmt.Printf("%f %d: Total: %d, Unmarshal: %d, Marshal: %d, TMBroadcast: %d\n",float64(cum.Microseconds())/1000000.0,counter, durationT.Microseconds(), duration0.Microseconds(), duration1.Microseconds(), duration2.Microseconds())

	mm.Unlock()
	//call tendermint
	//process response
	//ret{"Successfully submitted the transaction","aa8bac391e744340140ea0d95c7b37f9cc8a58601961bd751f5adb042af6f33b" }
	return r
}
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

func jsonrpcserver2(client abcicli.Client){//grpccore.BroadcastAPIClient){
	//s := NewJSONRPCServer()
	//arith := new(Arith)
	//s.Register(arith)
	//http.Handle("/rpc", s)
	//http.ListenAndServe(":1234", nil)
	// Register RPC methods.
	fct := NewFactomAPI(client)
	methods := jsonrpc2.MethodMap{
		"subtract":     subtract,
		"sum":          sum,
		"notify_hello": notifyHello,
		"get_data":     getData,
		"factoid-submit": fct.factoid_submit,
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
			tendermint.Initialize("directory-block-leader", "tcp://127.0.0.1:26600","tcp://127.0.0.1:26601","tcp://127.0.0.1:26602",ConfigFile[0],WorkingDir[0])
			tendermint.Initialize("accumulator-fct", "tcp://127.0.0.1:26610","tcp://127.0.0.1:26611","tcp://127.0.0.1:26612",ConfigFile[1],WorkingDir[1])
			tendermint.Initialize("accumulator-ec", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621","tcp://127.0.0.1:26622",ConfigFile[2],WorkingDir[2])
			tendermint.Initialize("accumulator-user", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631","tcp://127.0.0.1:26632",ConfigFile[3],WorkingDir[3])
//			tendermint.Initialize("vm2", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621",ConfigFile[2],WorkingDir[2])
//			tendermint.Initialize("vm3", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631",ConfigFile[3],WorkingDir[3])
			os.Exit(0)
		}
	}

	counter = 0

	//create a directory block leader
	//app := tendermint.NewDirectoryBlockLeader()

	//go app.Start(ConfigFile[0],WorkingDir[0])

	//make a factoid validator/accumulator




	//create a Factoid validator
	//db val := validator.NewFactoidValidator()

	//create a AccumulatorVM
	//db accvm1 := tendermint.NewAccumulatorVMApplication(ConfigFile[1],WorkingDir[1])
	//db accvm1.AddValidator(&val.ValidatorContext)

	//db go accvm1.Start()

	//time.Sleep(10000 * time.Millisecond)
	//db accvm1api, _ := accvm1.GetAPIClient()
	//db go jsonrpcserver2(accvm1api)

	app := kv.NewPersistentKVStoreApplication(WorkingDir[1])
	//app := kv.NewApplication()

	//fig := cfg.ResetTestRoot("node_priv_val_tcp_test")
	config := cfg.DefaultConfig()
	config.SetRoot(WorkingDir[1])
	viper.SetConfigFile(ConfigFile[1])
	if err := viper.ReadInConfig(); err != nil {

		fmt.Errorf("viper failed to read config file: %w", err)
		os.Exit(1)
	}
	if err := viper.Unmarshal(config); err != nil {
		fmt.Errorf("viper failed to unmarshal config: %w", err)
		os.Exit(1)
	}
	if err := config.ValidateBasic(); err != nil {
		fmt.Errorf("config is invalid: %w", err)

		os.Exit(1)
	}


//	defer os.RemoveAll(config.RootDir)

	// create node


//not good here ->
    //logger := tmlog.NewNopLogger()
    logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))

	//nd, err := nm.DefaultNewNode(config, logger)

    var err error
	logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel())
	if err != nil {
		fmt.Errorf("failed to parse log level: %w", err)
		os.Exit(1)
	}

	pv := privval.LoadFilePV(
		config.PrivValidatorKeyFile(),
		config.PrivValidatorStateFile(),
	)

	nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
	if err != nil {
		fmt.Errorf("failed to load node's key: %w", err)
		os.Exit(2)
	}

	danode, err := nm.NewNode(
		config,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(config),
		nm.DefaultDBProvider,
		nm.DefaultMetricsProvider(config.Instrumentation),
		logger)

	if err != nil {
		fmt.Errorf("failed to create new Tendermint node: %w", err)
		os.Exit(3)
	}
	go  danode.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}