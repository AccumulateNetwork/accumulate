package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	pb "github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"github.com/AdamSLevy/jsonrpc2/v14"
	proto1 "github.com/golang/protobuf/proto"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/abci/types"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	tmtypes "github.com/tendermint/tendermint/types"
	//coregrpc "github.com/tendermint/tendermint/rpc/grpc"

	rpchttp "github.com/tendermint/tendermint/rpc/client/http"

	"log"
	"net/http"
	"os"
	"strconv"
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
	client *rpchttp.HTTP
}

func NewFactomAPI(client *rpchttp.HTTP) (r *factomapi) { /*client grpccore.BroadcastAPIClient*/
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
		Txid    string `json:"txid,omitempty"`
	}

	err := json.Unmarshal(params, &p)
	if err != nil {
		return ret{"transaction unmarshal error", ""}
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
	hex.Decode(chainid[:], []byte(chainadi))
	chainhash := chainid[:]
	var duration1 time.Duration
	for i := 0; i < 1; /* 00000 */ i++ {
		vr := &pb.Submission{}
		//vr.Nonce = 0
		//vr.Signed = sig
		vr.Identitychain = managed.Hash(sha256.Sum256([]byte("FA000bt"))).Bytes() //this needs to point to the identity... :(

		vr.Type = validator.GetTypeIdFromName("fct")

		vr.Instruction = pb.AccInstruction_Token_Transaction

		vr.Data = decoded
		vr.Param1 = uint64(time.Since(start).Nanoseconds())
		vr.Param2 = 0

		vr.Chainid = make([]byte, 32)

		copy(vr.Chainid, chainhash[:])

		var msg tmtypes.Tx
		msg, _ = proto1.Marshal(vr)
		//type delivertx struct {
		//	Tx []byte `json:"tx"`
		//}
		//dtx := delivertx{msg }

		duration1 = time.Since(start)
		start = time.Now()

		//var c jsonrpc2.Client

		//batch := app.client.NewBatch()
		//req := tmtypes.Tx //types.RequestDeliverTx{}
		//req.Tx = msg

		resp, err := app.client.BroadcastTxAsync(context.Background(), msg) //DeliverTxAsync(req)
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
	r := ret{fmt.Sprintf("%d: (%f,cumulative: %f s) Hello there", counter,
		float64(durationT.Microseconds())/1000.0, float64(cum.Microseconds())/1000000.0),
		""} //,hex.EncodeToString(res.Data) }

	fmt.Printf("%f %d: Total: %d, Unmarshal: %d, Marshal: %d, TMBroadcast: %d\n", float64(cum.Microseconds())/1000000.0, counter, durationT.Microseconds(), duration0.Microseconds(), duration1.Microseconds(), duration2.Microseconds())

	mm.Unlock()
	//call tendermint
	//process response
	//ret{"Successfully submitted the transaction","aa8bac391e744340140ea0d95c7b37f9cc8a58601961bd751f5adb042af6f33b" }
	return r
}

func (app *factomapi) ablock_by_height(ctx context.Context, params json.RawMessage) interface{} {

	var p struct {
		Keymr *string `json:"keymr"`
	}

	type ret struct {
		Message string `json:"message"`
		Txid    string `json:"txid,omitempty"`
	}

	err := json.Unmarshal(params, &p)
	if err != nil {
		return ret{"ablock_by_height unmarshal error", ""}
	}

	return ret{"Not yet implemented", ""}
}

//func (app *factomapi) ack(ctx context.Context, params json.RawMessage) interface{} {
//}
func (app *factomapi) ack(ctx context.Context, params json.RawMessage) interface{} {

	var p struct {
		Hash    *string `json:"hash"`
		Chainid *string `json:"chainid"`
	}

	//Entry
	//{
	//	"jsonrpc":"2.0",
	//	"id":0,
	//	"result":{
	//	"committxid":"debbbb6b902de330bfaa78c6c9107eb0a451e10cd4523e150a8e8e6d5a042886",
	//		"entryhash":"1a6c96162e81d429de92b2f18a0ba9b428e505de0077a5d16ad5707f0f8a73b2",
	//		"commitdata":{
	//		"status":"DBlockConfirmed"
	//	},
	//	"entrydata":{
	//		"status":"DBlockConfirmed"
	//	}
	//}
	//}
	type Data struct {
		Status string `json:"status"`
	}
	type CommitAck struct {
		Committxid string `json:"committxid"`
		Entryhash  string `json:"entryhash"`
		Commitdata Data   `json:"commitdata"`
	}

	type eret struct {
		Result    CommitAck `json:"result"`
		Entrydata Data      `json:"entrydata"`
	}

	err := json.Unmarshal(params, &p)
	if err != nil {
		return eret{}
	}

	var r eret
	return r
}

func Jsonrpcserver2(client *rpchttp.HTTP, port int) { //grpccore.BroadcastAPIClient){
	fct := NewFactomAPI(client)
	methods := jsonrpc2.MethodMap{
		"factoid-submit":   fct.factoid_submit,
		"ablock-by-height": fct.ablock_by_height, //dbvc query?
		"ack":              fct.ack,
		//		"admin-block": fct.admin_block, //dbvc query?
		//		"chain-head": fct.chain_head,
		//		"commit-chain": fct.commit_chain,
		//		"commit-entry": fct.commit_entry,
		//		"current-minute": fct.current_minute, //no longer makes sense...
		//		"dblock-by-height": fct.dblock_by_height, //dbvc query???
		//		"directory-block": fct.directory_block, // dbvc query?
		//		"directory-block-head": fct.directory_block_head, //dbvc query?
		//		"ecblock-by-height": fct.eblock_by_height, //no sure this makes sense anymore
		//		"entry": fct.entry, //this pulls data from a chain based upon entry hash
		//		"entry-ack": fct.ack, //no longer used
		//		"entry-block": fct.entry_block, //no longer use entry blocks
		//		"entry-credit-balance": fct.entry_credit_balance, //this should be derived from current state
		//		"entry-credit-block": fct.entry_credit_block, //this no longer makes sense
		//		"entry-credit-rate": fct.entry_credit_rate, //maintained at each bvc
		//		"factoid-ack": fct.ack, //deprecated.
		//		"factoid-balance": fct.factoid_balance, //this should be either "token-balance" or generic "entry"
		//		"factoid-block": fct.factoid_block, //this no longer makes sense
		//		"fblock-by-height": fct.fblock_by_height, //this no longer makes sense
		//		"heights": fct.heights, //this is probably a dbvc thing
		//		"multiple-ec-balances": fct.multiple_ec_balances, //this should be more generic now
		//		"multiple-fct-balances", //this should be more generic now
		//		"pending-entries",
		//		"pending-transactions",
		//		"properties",
		//		"raw-data",
		//		"receipt", //this one is important.
		//		"reveal-chain", //commit / reveal combined now.
		//		"reveal-entry", // ditto
		//		"send-raw-message", //hmm.
		//		"transaction", //get details only valid for 2 weeks after transaction initiates

	}
	jsonrpc2.DebugMethodFunc = true
	handler := jsonrpc2.HTTPRequestHandler(methods, log.New(os.Stdout, "", 0))
	go http.ListenAndServe(":"+strconv.Itoa(port), handler)
}
