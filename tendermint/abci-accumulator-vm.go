package tendermint

import (
	"crypto/sha256"
	"fmt"
	"github.com/AccumulusNetwork/ValidatorAccumulator/ValAcc/accumulator"
	"github.com/spf13/viper"
	cfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	"github.com/tendermint/tendermint/version"
	"os"

	dbm "github.com/tendermint/tm-db"
	"github.com/AccumulusNetwork/ValidatorAccumulator/ValAcc/node"
	valacctypes "github.com/AccumulusNetwork/ValidatorAccumulator/ValAcc/types"
	vadb "github.com/AccumulusNetwork/ValidatorAccumulator/ValAcc/database"
	"github.com/AccumulusNetwork/accumulated/database"
	pb "github.com/AccumulusNetwork/accumulated/proto"
	"github.com/AccumulusNetwork/accumulated/validator"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	ed25519 "golang.org/x/crypto/ed25519"
	"time"
)



type AccumulatorVMApplication struct {
//	db           *badger.DB
//	currentBatch *badger.Txn
	abcitypes.BaseApplication

	router  router2.Router

	EntryFeed chan node.EntryHash
	AccNumber int64
	//EntryFeed = chan make(chan node.EntryHash, 10000)
	accountState map[string]AccountStateStruct
	BootstrapHeight int64
	Height int64
	Val validator.ValidatorInterface


	EntryHashStream chan node.EntryHash        // Stream of hashes to record
	AccumulatorDB   *dbm.DB             // Databases where hashes are recorded
	ACCs            []*accumulator.Accumulator // Accumulators to record hashes
	EntryFeeds      []chan node.EntryHash
	Controls        []chan bool
	MDFeeds         []chan *valacctypes.Hash
}

func NewAccumulatorVMApplication(val validator.ValidatorInterface) *AccumulatorVMApplication {

	app := AccumulatorVMApplication{
//		db: db,
		//router: new(router2.Router),
		EntryFeed: make(chan node.EntryHash, 10000),
		AccNumber: 1,
		//EntryFeed : make(chan node.EntryHash, 10000),
		accountState : make(map[string]AccountStateStruct),
		BootstrapHeight: 99999999999,
		Height : 0,
		Val : val,
	}
    return &app
}


var _ abcitypes.Application = (*AccumulatorVMApplication)(nil)


func (app *AccumulatorVMApplication) GetHeight ()(uint64) {
	return uint64(app.Height)
}

func (AccumulatorVMApplication) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{}
}

func (AccumulatorVMApplication) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return abcitypes.ResponseSetOption{}
}

// new transaction is added to the Tendermint Core. Check if it is valid.
func (app *AccumulatorVMApplication) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	bytesLen := len(req.Tx) - dataPtr - 64
	code := app.isValid(req.Tx,bytesLen)
	return abcitypes.ResponseCheckTx{Code: code, GasWanted: 1}
}


func (app *AccumulatorVMApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	fmt.Printf("Initalizing Accumulator Router\n")

	acc := new(accumulator.Accumulator)
	app.ACCs = append(app.ACCs, acc)

	str := "accumulator_" + *app.Val.GetInfo().GetTypeName() + "_" + *app.Val.GetInfo().GetInstanceName()
	chainID := valacctypes.Hash(sha256.Sum256([]byte(str)))
	//fixme: Requires Badger...


	entryFeed, control, mdHashes := acc.Init(app.AccumulatorDB, &chainID)
	app.EntryFeeds = append(app.EntryFeeds, entryFeed)
	app.Controls = append(app.Controls, control)
	app.MDFeeds = append(app.MDFeeds, mdHashes)
	//spin up the accumulator
	go acc.Run()

	//app.router.Init(app.EntryFeed, 1)
	//go app.router.Run()
	//app.router.Init(EntryFeed, int(AccNumber))
    //go router.Run()

	return abcitypes.ResponseInitChain{}
}

// ------ BeginBlock -> DeliverTx -> EndBlock -> Commit
// When Tendermint Core has decided on the block, it's transferred to the application in 3 parts:
// BeginBlock, one DeliverTx per transaction and EndBlock in the end.

//Here we create a batch, which will store block's transactions.
func (app *AccumulatorVMApplication) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	//app.currentBatch = app.db.NewTransaction(true)
	app.Height = req.Header.Height

	app.Val.SetCurrentBlock(req.Header.Height,&req.Header.Time,&req.Header.ChainID)
	return abcitypes.ResponseBeginBlock{}
}

// Invalid transactions, we again return the non-zero code.
// Otherwise, we add it to the current batch.
func (app *AccumulatorVMApplication) DeliverTx(req abcitypes.RequestDeliverTx) ( response abcitypes.ResponseDeliverTx) {

	bytesLen := len(req.Tx) - dataPtr - 64
	code := app.isValid(req.Tx,bytesLen)

	if code != 0 {
		return abcitypes.ResponseDeliverTx{Code: code}
	}

	AccountState, err := app.GetAccountState(req.Tx[0:32])

	if err != nil {
		response.Code = 3
		response.Info = err.Error()
		println(err.Error())
		return response
	}

	AccountState.MessageCountDown--  //We decrement before we test, so we can see if there is abnormal activity
	fmt.Printf("Messages Left = %v\n",AccountState.MessageCountDown)
	if AccountState.MessageCountDown < 0{
		//if (AccountState.MessageCountDown < BanListTrigger) TODO
		response.Code = 4
		response.Info = "Account exceeded message count"
		println(err.Error())
		return response
	}
	AccountState.LastBlockHeight = app.Height

	//Grab our payload
	data := req.Tx[dataPtr:dataPtr + bytesLen]

	var ret = app.Val.Validate(data)
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

	if (err != nil) {
		println(err.Error())
	}

	return response
}

//Commit instructs the application to persist the new state.
func (app *AccumulatorVMApplication) Commit() abcitypes.ResponseCommit {
	//app.currentBatch.Commit()
	//pull merkle DAG from the accumulator and put on blockchain as the Data
	return abcitypes.ResponseCommit{Data: []byte{}}
}


func (AccumulatorVMApplication) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	//do any cleanup for block sealing...
	return abcitypes.ResponseEndBlock{}
}

//------------------------


// when the client wants to know whenever a particular key/value exist, it will call Tendermint Core RPC /abci_query endpoint
func (app *AccumulatorVMApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	resQuery.Key = reqQuery.Data
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


func (app *AccumulatorVMApplication) isValid(tx []byte, bytesLen int) (code uint32) {

	// [Key][Type][Nonce][Data][Sign]


	//Zero bytes acceptable, but negative value indecates malformed message
	if bytesLen < 0 {
		return 1
	}

	//Maximim data size (TODO move to global configuration)
	if bytesLen > 10240 {
		return 2
	}

	signPtr := dataPtr + bytesLen

	if ed25519.Verify(tx[keyPtr:keyPtrE], tx[msgTypePtr:signPtr], tx[signPtr:]) {
		println("Valid")
	 	return 0
	}
	println("NOT Valid")
	return 3
}

func (app *AccumulatorVMApplication) GetAccountState (publicKey []byte) (acc AccountStateStruct,err error)  {

	keyString := string(publicKey)
	var ok bool

	//Query cache first
	if acc, ok = app.accountState[keyString]; !ok {
		//Not found in cache, read disk
		account, err := GetAccount(publicKey)
		if err !=nil{  //No account for publicKey found!
			if app.Height < app.BootstrapHeight {
			}else {
				return acc, err
			}
		}
		println("Account Found on Disk")
		acc = AccountStateStruct{
			PublicKey:        publicKey,
			MessageAllowance: account.MessageAllowance,
			MessageCountDown: account.MessageAllowance,
		}
		app.accountState[keyString] = acc
	} else {
		println("Account Found in Cache")
	}

	acc.LastAccess = time.Now().UnixNano()
	return acc,nil
}

func (app *AccumulatorVMApplication) MakeBootStrapAccount(publicKey []byte)(state AccountStateStruct,err error){

	println("Making BootStrap Account")

	account := pb.Account {
		Name: "Bootstrap Account",
		MessageAllowance: 20000,
		AllowAddAccounts: true,
		AllowAddGroups: true,
	}

	state = AccountStateStruct {
		PublicKey:        publicKey,
		MessageCountDown: account.MessageAllowance,
		MessageAllowance: account.MessageAllowance,
	}
	app.accountState[string(publicKey)] = state
	return state,nil
}

func (app *AccumulatorVMApplication) WriteKeyValue(account AccountStateStruct, data []byte) (err error) {

	KeyValue := pb.KeyValue{}

	err = KeyValue.Unmarshal(data)
	if err != nil {
		return err
	}

	KeyValue.Height = uint64(app.Height)
	data,err = KeyValue.Marshal()

	//AccountAdd.
	err = database.KvStoreDB.Set(KeyValue.Key,data)
	if err != nil{
		fmt.Printf("WriteKeyValue err %v\n",err)
	} else {
		fmt.Printf("WriteKeyValue: %v\n",KeyValue.Key)
	}
	return err
}


func (app *AccumulatorVMApplication) Start(ConfigFile string, WorkingDir string) (*nm.Node, error) {
	fmt.Printf("Starting Tendermint (version: %v)\n", version.Version)

	config := cfg.DefaultConfig()
	config.SetRoot(WorkingDir)

	viper.SetConfigFile(ConfigFile)
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("viper failed to read config file: %w", err)
	}
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("viper failed to unmarshal config: %w", err)
	}
	if err := config.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("config is invalid: %w", err)
	}

	// create logger
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
	var err error
	logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel())
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %w", err)
	}

	// read private validator
	pv := privval.LoadFilePV(
		config.PrivValidatorKeyFile(),
		config.PrivValidatorStateFile(),
	)

	// read node key
	nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load node's key: %w", err)
	}

	//initialize the accumulator database
	str := "accumulator_" + *app.Val.GetInfo().GetTypeName() + "_" + *app.Val.GetInfo().GetInstanceName()
	fmt.Printf("Creating %s\n", str)
	app.AccumulatorDB, err = nm.DefaultDBProvider(&nm.DBContext{str, config})
	if err != nil {
		return nil,fmt.Errorf("failed to create node accumulator database: %w", err)
	}

	//accumulator database
	dir := WorkingDir + "/" + str + ".db"

	db2 := dbm.NewDB(str,dbm.BadgerDBBackend,dir)
	DB := vadb.InitDB(db2)
	//db.Init(i)

	//initialize the validator databases
	if app.Val.InitDBs(config, nm.DefaultDBProvider ) !=nil {
		fmt.Println("DB Error")
		return nil,nil //TODO
	}

	// create node
	node, err := nm.NewNode(
		config,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(config),
		nm.DefaultDBProvider,
		nm.DefaultMetricsProvider(config.Instrumentation),
		logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
	}

	fmt.Println("Tendermint Start")
	node.Start()

	defer func() {
		node.Stop()
		node.Wait()
		fmt.Println("Tendermint Stopped")
	}()

	node.Wait()

	return node,nil
}