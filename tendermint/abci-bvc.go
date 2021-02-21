package tendermint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/accumulator"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	abcicli "github.com/tendermint/tendermint/abci/client"
	cfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/types"

	//nm "github.com/AccumulateNetwork/accumulated/vbc/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	//grpccore "github.com/tendermint/tendermint/rpc/grpc"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	dbm "github.com/tendermint/tm-db"
	//"github.com/tendermint/tendermint/types"

	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/version"
	"os"

	vadb "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/database"
	//dbm "github.com/tendermint/tm-db"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/node"
	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/accumulated/database"
	pb "github.com/AccumulateNetwork/accumulated/proto"
	"github.com/AccumulateNetwork/accumulated/tendermint/dbvc"
	"github.com/AccumulateNetwork/accumulated/validator"
	abcitypes "github.com/tendermint/tendermint/abci/types"

	"sync"
	//"time"
)



var (
	stateKey        = []byte("stateKey")
	kvPairPrefixKey = []byte("kvPairKey:")

	ProtocolVersion uint64 = 0x1
)

type State struct {
	db      dbm.DB
	Size    int64  `json:"size"`
	Height  int64  `json:"height"`
	AppHash []byte `json:"app_hash"`
}

func loadState(db dbm.DB) State {
	var state State
	state.db = db
	stateBytes, err := db.Get(stateKey)
	if err != nil {
		panic(err)
	}
	if len(stateBytes) == 0 {
		return state
	}
	err = json.Unmarshal(stateBytes, &state)
	if err != nil {
		panic(err)
	}
	return state
}

func saveState(state State) {
	stateBytes, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	err = state.db.Set(stateKey, stateBytes)
	if err != nil {
		panic(err)
	}
}

func prefixKey(key []byte) []byte {
	return append(kvPairPrefixKey, key...)
}

type AccumulatorVMApplication struct {
//	db           *badger.DB
//	currentBatch *badger.Txn

	abcitypes.BaseApplication
	RetainBlocks int64
	mutex sync.Mutex
	waitgroup sync.WaitGroup

	//router  router2.Router

	Height int64
	EntryFeed chan node.EntryHash
	AccNumber int64
	//EntryFeed = chan make(chan node.EntryHash, 10000)
	accountState map[string]AccountStateStruct
	BootstrapHeight int64
	ChainId [32]byte
	Val *validator.ValidatorContext
	chainval map[uint64]*validator.ValidatorContext
	DB vadb.DB

	state State

	//recording of proofs
	EntryHashStream chan node.EntryHash        // Stream of hashes to record
	//AccumulatorDB   *dbm.DB             // Databases where hashes are recorded
	ACCs            []*accumulator.Accumulator // Accumulators to record hashes
	EntryFeeds      []chan node.EntryHash
	Controls        []chan bool
	MDFeeds         []chan *valacctypes.Hash

    ///
	valTypeRegDB    dbm.DB
	config *cfg.Config
	RPCContext rpctypes.Context
	server service.Service


}

func NewAccumulatorVMApplication(ConfigFile string, WorkingDir string) *AccumulatorVMApplication {
	name := "kvstore"
	db, err := dbm.NewGoLevelDB(name, WorkingDir)
	if err != nil {
		panic(err)
	}

	state := loadState(db)

	app := AccumulatorVMApplication{
		//router: new(router2.Router),
		RetainBlocks: 1, //only retain current block, we will manage our own state
		chainval: make(map[uint64]*validator.ValidatorContext),
		EntryFeed: make(chan node.EntryHash, 10000),
		AccNumber: 1,
		//EntryFeed : make(chan node.EntryHash, 10000),
		accountState : make(map[string]AccountStateStruct),
		BootstrapHeight: 99999999999,
		Val : nil,
		state : state,
	}
	app.Initialize(ConfigFile, WorkingDir)
    return &app
}


var _ abcitypes.Application = (*AccumulatorVMApplication)(nil)


func (app *AccumulatorVMApplication) AddValidator(val *validator.ValidatorContext) error {
	//validators are mapped to registered type id's.
	//getTypeId(val.GetChainId())

	//so perhaps, the validator should lookup typeid by chainid in the validator registration database.


	app.chainval[val.GetTypeId()] = val
	//tmp
	app.Val = val
	//registration of the validators should be done on-chain
	return nil
}

func (app *AccumulatorVMApplication) GetHeight ()(int64) {
	//
	//app.mutex.Lock()
	//ret := uint64(app.Val.GetCurrentHeight())
	//app.mutex.Unlock()
	//
	return app.state.Height
}

func (app *AccumulatorVMApplication) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	/*
	type RequestInfo struct {
		Version      string `protobuf:"bytes,1,opt,name=version,proto3" json:"version,omitempty"`
		BlockVersion uint64 `protobuf:"varint,2,opt,name=block_version,json=blockVersion,proto3" json:"block_version,omitempty"`
		P2PVersion   uint64 `protobuf:"varint,3,opt,name=p2p_version,json=p2pVersion,proto3" json:"p2p_version,omitempty"`
	}
	 */
	app.RetainBlocks = 1
	return abcitypes.ResponseInfo{
		Data:             fmt.Sprintf("{\"size\":%v}", app.state.Size),
		Version:          version.ABCIVersion,
		AppVersion:       ProtocolVersion,
		LastBlockHeight:  app.state.Height,
		LastBlockAppHash: app.state.AppHash,
	}
}

func (app *AccumulatorVMApplication) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return app.BaseApplication.SetOption(req)
}

func (app *AccumulatorVMApplication) GetAPIClient() (abcicli.Client, error) {
	app.waitgroup.Wait()
	//todo: fixme
	return makeGRPCClient("localhost:22223")//app.config.RPC.GRPCListenAddress)
}

func (app *AccumulatorVMApplication) Initialize(ConfigFile string, WorkingDir string) error {
	app.waitgroup.Add(1)
	fmt.Printf("Starting Tendermint (version: %v)\n", version.ABCIVersion)

	app.config = cfg.DefaultConfig()
	app.config.SetRoot(WorkingDir)

	viper.SetConfigFile(ConfigFile)
	if err := viper.ReadInConfig(); err != nil {

		return fmt.Errorf("viper failed to read config file: %w", err)
	}
	if err := viper.Unmarshal(app.config); err != nil {
		return fmt.Errorf("viper failed to unmarshal config: %w", err)
	}
	if err := app.config.ValidateBasic(); err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}


	str := "ValTypeReg"
	fmt.Printf("Creating %s\n", str)
	cdb, err := nm.DefaultDBProvider(&nm.DBContext{str, app.config})
	app.valTypeRegDB = cdb
	if err != nil {
		return fmt.Errorf("failed to create node accumulator database: %w", err)
	}

	//app.server, err = makeGRPCServer(app, app.config.RPC.ListenAddress)

	//app.client, err = makeGRPCClient(app.BaseApplication, app.config.RPC.ListenAddress)

	//RPCContext.RPCRequest// = new(JSONReq{})
	return nil
}

///ABCI call
func (app *AccumulatorVMApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	/*
	type RequestInitChain struct {
		Time            time.Time         `protobuf:"bytes,1,opt,name=time,proto3,stdtime" json:"time"`
		ChainId         string            `protobuf:"bytes,2,opt,name=chain_id,json=chainId,proto3" json:"chain_id,omitempty"`
		ConsensusParams *ConsensusParams  `protobuf:"bytes,3,opt,name=consensus_params,json=consensusParams,proto3" json:"consensus_params,omitempty"`
		Validators      []ValidatorUpdate `protobuf:"bytes,4,rep,name=validators,proto3" json:"validators"`
		AppStateBytes   []byte            `protobuf:"bytes,5,opt,name=app_state_bytes,json=appStateBytes,proto3" json:"app_state_bytes,omitempty"`
		InitialHeight   int64             `protobuf:"varint,6,opt,name=initial_height,json=initialHeight,proto3" json:"initial_height,omitempty"`
	}*/
	fmt.Printf("Initalizing Accumulator Router\n")

	acc := new(accumulator.Accumulator)
	app.ACCs = append(app.ACCs, acc)

	str := "accumulator-" + *app.Val.GetInfo().GetNamespace()// + "_" + *app.Val.GetInfo().GetInstanceName()
	if str != req.ChainId {
		fmt.Printf("Invalid chain validator\n")
		return abcitypes.ResponseInitChain{}
	}
	app.ChainId = sha256.Sum256([]byte(req.ChainId))
    //hchain := valacctypes.Hash(app.ChainId)
	entryFeed, control, mdHashes := acc.Init(&app.DB, (*valacctypes.Hash)(&app.ChainId))
	app.EntryFeeds = append(app.EntryFeeds, entryFeed)
	app.Controls = append(app.Controls, control)
	app.MDFeeds = append(app.MDFeeds, mdHashes)
	//spin up the accumulator
	go acc.Run()

	//app.router.Init(app.EntryFeed, 1)
	//go app.router.Run()
	//app.router.Init(EntryFeed, int(AccNumber))
    //go router.Run()

	return abcitypes.ResponseInitChain{AppHash: app.ChainId[:]}
}

///ABCI / block calls
///   BeginBlock <---
///   [CheckTx]
///   [DeliverTx]
///   EndBlock
///   Commit
// ------ BeginBlock -> DeliverTx -> EndBlock -> Commit
// When Tendermint Core has decided on the block, it's transferred to the application in 3 parts:
// BeginBlock, one DeliverTx per transaction and EndBlock in the end.

//Here we create a batch, which will store block's transactions.
func (app *AccumulatorVMApplication) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	//app.currentBatch = app.db.NewTransaction(true)
	//app.Height = req.Header.Height
	// reset valset changes


/*
	app.ValUpdates = make([]types.ValidatorUpdate, 0)

	// Punish validators who committed equivocation.
	for _, ev := range req.ByzantineValidators {
		if ev.Type == types.EvidenceType_DUPLICATE_VOTE {
			addr := string(ev.Validator.Address)
			if pubKey, ok := app.valAddrToPubKeyMap[addr]; ok {
				app.updateValidator(types.ValidatorUpdate{
					PubKey: pubKey,
					Power:  ev.Validator.Power - 1,
				})
				app.logger.Info("Decreased val power by 1 because of the equivocation",
					"val", addr)
			} else {
				app.logger.Error("Wanted to punish val, but can't find it",
					"val", addr)
			}
		}
	}

*/
	//need to fix me, rather than update every validator, instead have validator ask for it if needed.
	app.Val.SetCurrentBlock(req.Header.Height,&req.Header.Time,&req.Header.ChainID)
	return abcitypes.ResponseBeginBlock{}
}

///ABCI / block calls
///   BeginBlock
///   [CheckTx] <---
///   [DeliverTx]
///   EndBlock
///   Commit

// new transaction is added to the Tendermint Core. Check if it is valid.
func (app *AccumulatorVMApplication) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	/*
		type RequestCheckTx struct {
			Tx   []byte      `protobuf:"bytes,1,opt,name=tx,proto3" json:"tx,omitempty"`
			Type CheckTxType `protobuf:"varint,2,opt,name=type,proto3,enum=tendermint.abci.CheckTxType" json:"type,omitempty"`
		}*/
	//bytesLen := len(req.Tx) - dataPtr - 64
	//code := app.isValid(req.Tx,bytesLen)

	//code = 0
	ret := abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}

	addr := binary.BigEndian.Uint64(req.Tx)
	//addr := req.GetType()
    if val, ok := app.chainval[addr]; ok {
		val.Check(req.GetTx())
    	//need transaction id.
	}

	return ret//abcitypes.ResponseCheckTx{Code: code, GasWanted: 1}
}

///ABCI / block calls
///   BeginBlock
///   [CheckTx]
///   [DeliverTx] <---
///   EndBlock
///   Commit

// Invalid transactions, we again return the non-zero code.
// Otherwise, we add it to the current batch.
func (app *AccumulatorVMApplication) DeliverTx(req abcitypes.RequestDeliverTx) ( response abcitypes.ResponseDeliverTx) {


	code := 0
	//addr := binary.BigEndian.Uint64(Tx[0:32])

	//ret := abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}

	addr := binary.BigEndian.Uint64(req.Tx)
	//addr := req.GetType()
	if val, ok := app.chainval[addr]; ok {
		val.Validate(req.GetTx())
		//need transaction id.
	}

	//bytesLen := len(req.Tx) - dataPtr - 64
	//code := app.isValid(req.Tx,bytesLen)

	if code != 0 {
		//intentionally reject Transaction to prevent it from being stored and will instead add it to the valacc pool.
		//we'll do the honors of storing the transaction ourselves.


		return abcitypes.ResponseDeliverTx{Code: 1, Data: app.ChainId[0:32]}
		//return abcitypes.ResponseDeliverTx{Code: 0}
	}
	//
	//AccountState, err := app.GetAccountState(req.Tx[0:32])
	//
	//if err != nil {
	//	response.Code = 3
	//	response.Info = err.Error()
	//	println(err.Error())
	//	return response
	//}
	//
	//AccountState.MessageCountDown--  //We decrement before we test, so we can see if there is abnormal activity
	//fmt.Printf("Messages Left = %v\n",AccountState.MessageCountDown)
	//if AccountState.MessageCountDown < 0{
	//	//if (AccountState.MessageCountDown < BanListTrigger) TODO
	//	response.Code = 4
	//	response.Info = "Account exceeded message count"
	//	println(err.Error())
	//	return response
	//}
	//AccountState.LastBlockHeight = app.Val.GetCurrentHeight()
	//
	////Grab our payload
	//data := req.Tx[dataPtr:dataPtr + bytesLen]
	//
	//var _ = app.Val.Validate(data)
	//pass into accumulator...
    //app.Acc.Accumulate(ret)

	//switch pb.EntryMsgType(req.Tx[msgTypePtr]) {
	//	case pb.Entry_WriteEntryBytes:
	//			//Nothing extra to process, all good response.Code defaults to 0
	//	case pb.Entry_WriteKeyValue:
	//		err = app.WriteKeyValue(AccountState,data)
	//		if err != nil {
	//			response.Code = 1
	//			response.Info = err.Error()
	//		}
	//	default:
	//		response.Code = 1
	//		response.Info = fmt.Sprintf("Uknown message type %v \n",req.Tx[msgTypePtr])
	//}

/*	if (err != nil) {
		println(err.Error())
	}
*/
	return response
}


///ABCI / block calls
///   BeginBlock
///   [CheckTx]
///   [DeliverTx]
///   EndBlock <---
///   Commit


// Update the validator set
func (app *AccumulatorVMApplication) EndBlock(req abcitypes.RequestEndBlock) (resp abcitypes.ResponseEndBlock) {
	// Select our leader who will initiate consensus on dbvc chain.
	//resp.ConsensusParamUpdates
	//for _, ev := range req.ByzantineValidators {
	//	if ev.Type == types.EvidenceType_DUPLICATE_VOTE {
	//		addr := string(ev.Validator.Address)
	//		if pubKey, ok := app.valAddrToPubKeyMap[addr]; ok {
	//			app.updateValidator(types.ValidatorUpdate{
	//				PubKey: pubKey,
	//				Power:  ev.Validator.Power - 1,
	//			})
	//			app.logger.Info("Decreased val power by 1 because of the equivocation",
	//				"val", addr)
	//		} else {
	//			app.logger.Error("Wanted to punish val, but can't find it",
	//				"val", addr)
	//		}
	//	}
	//}

	return abcitypes.ResponseEndBlock{}//ValidatorUpdates: app.ValUpdates}
}
///ABCI / block calls
///   BeginBlock
///   [CheckTx]
///   [DeliverTx]
///   EndBlock
///   Commit <---

//Commit instructs the application to persist the new state.
func (app *AccumulatorVMApplication) Commit() abcitypes.ResponseCommit {
	//app.currentBatch.Commit()
	//pull merkle DAG from the accumulator and put on blockchain as the Data

	dblock := dbvc.DBlock{}//NewDBlock()
	data, _ := dblock.MarshalBinary()

	//saveDBlock
	resp := abcitypes.ResponseCommit{Data: data}
	if app.RetainBlocks > 0 && app.Height >= app.RetainBlocks {
		resp.RetainHeight = app.Height - app.RetainBlocks + 1
	}
	return resp
	//return abcitypes.ResponseCommit{Data: []byte{}}
}


//------------------------


func (app *AccumulatorVMApplication) ListSnapshots(
	req abcitypes.RequestListSnapshots) abcitypes.ResponseListSnapshots {
	return abcitypes.ResponseListSnapshots{}
}

func (app *AccumulatorVMApplication) LoadSnapshotChunk(
	req abcitypes.RequestLoadSnapshotChunk) abcitypes.ResponseLoadSnapshotChunk {
	return abcitypes.ResponseLoadSnapshotChunk{}
}

func (app *AccumulatorVMApplication) OfferSnapshot(
	req abcitypes.RequestOfferSnapshot) abcitypes.ResponseOfferSnapshot {
	return abcitypes.ResponseOfferSnapshot{Result: abcitypes.ResponseOfferSnapshot_ABORT}
}

func (app *AccumulatorVMApplication) ApplySnapshotChunk(

	req abcitypes.RequestApplySnapshotChunk) abcitypes.ResponseApplySnapshotChunk {
	return abcitypes.ResponseApplySnapshotChunk{Result: abcitypes.ResponseApplySnapshotChunk_ABORT}
}


// when the client wants to know whenever a particular key/value exist, it will call Tendermint Core RPC /abci_query endpoint
func (app *AccumulatorVMApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	resQuery.Key = reqQuery.Data

	///implement lazy sync calls. If a node falls behind it needs to have several query calls
	///1 get current height
	///2 get block data for height X
	///3 get block data for given hash

	/*
	err := app.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(reqQuery.Data)
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		if err == badger.ErrKeyNotFound {
			resQuery.Log = "does not exist"
		} else {
			return item.Value(func(val []byte) error {
				resQuery.Log = "exists"
				resQuery.Value = val
				return nil
			})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	 */
	return
}


//func (app *AccumulatorVMApplication) isValid(tx []byte, bytesLen int) (code uint32) {
//
//	// [Key][Type][Nonce][Data][Sign]
//
//
//	//Zero bytes acceptable, but negative value indecates malformed message
//	if bytesLen < 0 {
//		return 1
//	}
//
//	//Maximim data size (TODO move to global configuration)
//	if bytesLen > 10240 {
//		return 2
//	}
//
//	signPtr := dataPtr + bytesLen
//
//	if ed25519.Verify(tx[keyPtr:keyPtrE], tx[msgTypePtr:signPtr], tx[signPtr:]) {
//		println("Valid")
//	 	return 0
//	}
//	println("NOT Valid")
//	return 3
//}



func (app *AccumulatorVMApplication) WriteKeyValue(account AccountStateStruct, data []byte) (err error) {

	KeyValue := &pb.KeyValue{}

	err = proto.Unmarshal(data,KeyValue)
	if err != nil {
		return err
	}

	KeyValue.Height = uint64(app.GetHeight())
	data,err = proto.Marshal(KeyValue)

	//AccountAdd.
	err = database.KvStoreDB.Set(KeyValue.Key,data)
	if err != nil{
		fmt.Printf("WriteKeyValue err %v\n",err)
	} else {
		fmt.Printf("WriteKeyValue: %v\n",KeyValue.Key)
	}
	return err
}


func (app *AccumulatorVMApplication) Start() (*nm.Node, error) {

	// create logger
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
	var err error
	logger, err = tmflags.ParseLogLevel(app.config.LogLevel, logger, cfg.DefaultLogLevel())
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %w", err)
	}

	// read private validator
	pv := privval.LoadFilePV(
		app.config.PrivValidatorKeyFile(),
		app.config.PrivValidatorStateFile(),
	)

	// read node key
	nodeKey, err := p2p.LoadNodeKey(app.config.NodeKeyFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load node's key: %w", err)
	}

	//initialize the accumulator database
	str := "accumulator_" + *app.Val.GetInfo().GetNamespace()// + "_" + *app.Val.GetInfo().GetInstanceName()
	fmt.Printf("Creating %s\n", str)
	db2, err := nm.DefaultDBProvider(&nm.DBContext{str, app.config})
	if err != nil {
		return nil,fmt.Errorf("failed to create node accumulator database: %w", err)
	}


    //app.DB.db2 = db2
	//accumulator database
	//dir := WorkingDir + "/" + str + ".db"

	//app.AccumulatorDB, err := dbm.NewDB(str,dbm.BadgerDBBackend,dir)
	//if err != nil {
	//	return nil,fmt.Errorf("failed to create node accumulator database: %w", err)
	//}

	app.DB.InitDB(db2)
	//db.Init(i)


	//initialize the validator databases
	//if app.Val.InitDBs(app.config, nm.DefaultDBProvider ) !=nil {
	//	fmt.Println("DB Error")
	//	return nil,nil //TODO
	//}


	// create node
	node, err := nm.NewNode(
		app.config,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(app.config),
		nm.DefaultDBProvider,
		nm.DefaultMetricsProvider(app.config.Instrumentation),
		logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
	}
	//node.


	fmt.Println("Tendermint Start")
	//app.config.RPC().
    node.Start()
	//var grpcSrv *grpc.Server
	makeGRPCServer(app, "127.0.0.1:22223")
	//grpcSrv, err = servergrpc.StartGRPCServer(app, app.config.RPC.GRPCListenAddress)
	//
	//gapp := types.NewGRPCApplication(app)
	//server := abciserver.NewGRPCServer(socket, gapp)
	//server.SetLogger(logger.With("module", "abci-server"))
	//if err := server.Start(); err != nil {
	//	return nil, err
	//}
	//
	//if err != nil {
	//	return err
	//}


	//s := node.Listeners()
	defer func() {
		node.Stop()
		node.Wait()
		fmt.Println("Tendermint Stopped")
	}()

    //time.Sleep(1000*time.Millisecond)
	if node.IsListening() {
		fmt.Print("node is listening")
	}
	app.waitgroup.Done()
	node.Wait()

	return node,nil
}
