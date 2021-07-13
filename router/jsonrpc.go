package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/SMT/smt"
	pb "github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"github.com/AdamSLevy/jsonrpc2/v14"
	proto1 "github.com/golang/protobuf/proto"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/abci/types"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/rpc/core"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"

	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

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

	//sig, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")


	duration0 := time.Since(start)
	//
	//uint64  validatorAddr  = 1; // block validation chain address - sha256(chainId)[0:7] - big-endian or little-endian?
	//uint32  instruction 	 = 2; // instruction code used by the validator - e.g. "write data"
	//uint32  parameter1     = 3; // parameter 1 specific to the instruction
	//uint32  parameter2     = 4; // parameter 2 specific to the instruction
	//uint64 	nonce          = 5; // typically time stamp or monotonic counter
	//bytes   data           = 6; // payload for validator - Command Data
	//bytes   signed         = 7; // ed25519 signature

	chainadi := "000000000000000000000000000000000000000000000000000000000000000f"
	var chainid [32]byte
	hex.Decode(chainid[:],[]byte(chainadi))
	chainhash := chainid[:]
	var duration1 time.Duration
	for i := 0; i < 1/* 00000 */; i++ {
		vr := &pb.Submission{}
		//vr.Nonce = 0
		//vr.Signed = sig
		vr.Identitychain = smt.Hash(sha256.Sum256([]byte("FA000bt"))).Bytes() //this needs to point to the identity... :(

		vr.Type = validator.GetTypeIdFromName("fct")

		vr.Instruction = pb.AccInstruction_Token_Transaction

		vr.Data = decoded
		vr.Param1 = uint64(time.Since(start).Nanoseconds())
		vr.Param2 = 0

		vr.Chainid = make([]byte, 32)

		copy(vr.Chainid,chainhash[:])

		msg, _ := proto1.Marshal(vr)
		//type delivertx struct {
		//	Tx []byte `json:"tx"`
		//}
		//dtx := delivertx{msg }

		duration1 = time.Since(start)
		start = time.Now()

		//var c jsonrpc2.Client

		req := types.RequestDeliverTx{}
		req.Tx = msg
		resp := app.client.DeliverTxAsync(req)
		if resp == nil {
			fmt.Printf("Error received from Check Tx Sync %v", err)
		}
		//fmt.Printf("Result : %s", resp.)
		//var result int
		//err = c.Request(nil, "http://localhost:26611", "broadcast_tx_sync", dtx, &result)

		//err = c.Request(nil, "http://localhost:26611", "broadcast_tx_commit", dtx, &result)
	}
	//for i := 0; i < 100; i++ {
	//
	//	app.client.CheckTxSync(types.RequestCheckTx{Tx: msg})
	//	app.client.DeliverTxAsync(types.RequestDeliverTx{msg})
	//}
	//app.client.BroadcastTx(ctx,&grpccore.RequestBroadcastTx{msg}) //app.client.DeliverTxSync(types.RequestDeliverTx{Tx: decoded})
	//if err2 != nil {
	//	return ret{"broadcast tx error",""}
	//}
	//res.GetDeliverTx()
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

func Jsonrpcserver2(client abcicli.Client){//grpccore.BroadcastAPIClient){
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

