package tendermint

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	cfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/version"
	"os"

	//"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/node"
	//router2 "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/router"
	"github.com/AccumulateNetwork/accumulated/database"
	pb "github.com/AccumulateNetwork/accumulated/proto"
	abci "github.com/tendermint/tendermint/abci/types"
	ed25519 "golang.org/x/crypto/ed25519"
	"time"
)

const (
	keyPtr = 0 					// [Key]  32 bytes
	keyPtrE = keyPtr + 32

	msgTypePtr = keyPtrE 	// [Type] 1 byte  (var_int)

 	noncePtr = msgTypePtr + 1	// [Nonce] 8 byte (typically time stamp)
 	dataPtr = noncePtr + 8    	// [Data] ? bytes
	// [Sign] 64 bytes (type+nonce+data)
)

const BanListTrigger = -10000




type DirectoryBlockLeader struct {

	abci.BaseApplication
//	db           *badger.DB
//	currentBatch *badger.Txn
	//router = new(router2.Router)
	AccNumber int64
	Hash [32]uint8
	//EntryFeed = chan make(chan node.EntryHash, 10000)
	accountState map[string]AccountStateStruct
	BootstrapHeight int64
	Height int64
}

func NewDirectoryBlockLeader() *DirectoryBlockLeader {
	app := DirectoryBlockLeader{
//		db: db,
		//router: new(router2.Router),
		AccNumber: 1,
		//EntryFeed : make(chan node.EntryHash, 10000),
		accountState : make(map[string]AccountStateStruct),
		BootstrapHeight: 99999999999,
		Height : 0,
	}
    return &app
}

var _ abci.Application = (*DirectoryBlockLeader)(nil)

func (app *DirectoryBlockLeader) GetHeight ()(uint64) {
	return uint64(app.Height)
}

func (DirectoryBlockLeader) Info(req abci.RequestInfo) abci.ResponseInfo {
	return abci.ResponseInfo{}
}

func (DirectoryBlockLeader) SetOption(req abci.RequestSetOption) abci.ResponseSetOption {
	return abci.ResponseSetOption{}
}

// new transaction is added to the Tendermint Core. Check if it is valid.
func (app *DirectoryBlockLeader) CheckTx(req abci.RequestCheckTx) abci.ResponseCheckTx {
	bytesLen := len(req.Tx) - dataPtr - 64
	code := app.isValid(req.Tx,bytesLen)
	return abci.ResponseCheckTx{Code: code, GasWanted: 1}
}


func (app *DirectoryBlockLeader) InitChain(req abci.RequestInitChain) abci.ResponseInitChain {
	fmt.Printf("Initalizing Accumulator Router\n")
	//app.router.Init(EntryFeed, int(AccNumber))
    //go router.Run()
	return abci.ResponseInitChain{}
}

// ------ BeginBlock -> DeliverTx -> EndBlock -> Commit
// When Tendermint Core has decided on the block, it's transferred to the application in 3 parts:
// BeginBlock, one DeliverTx per transaction and EndBlock in the end.

//Here we create a batch, which will store block's transactions.
func (app *DirectoryBlockLeader) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	//app.currentBatch = app.db.NewTransaction(true)
	//may require request to get data from
	//do any housekeeping for accumulator?
	app.Height = req.Header.Height
	return abci.ResponseBeginBlock{}
}

// Invalid transactions, we again return the non-zero code.
// Otherwise, we add it to the current batch.
func (app *DirectoryBlockLeader) DeliverTx(req abci.RequestDeliverTx) ( response abci.ResponseDeliverTx) {

	//obtain merkle dag root from VM
	//write that to whereever...
	bytesLen := len(req.Tx) - dataPtr - 64
	code := app.isValid(req.Tx,bytesLen)

	if code != 0 {
		return abci.ResponseDeliverTx{Code: code}
	}
    //if the message is a custom message for us... deal with it...

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

	switch pb.EntryMsgType(req.Tx[msgTypePtr]) {

		case pb.Entry_WriteEntryBytes:
				//Nothing extra to process, all good response.Code defaults to 0
		case pb.Entry_WriteKeyValue:
			err = app.WriteKeyValue(AccountState,data)
			if err != nil {
				response.Code = 1
				response.Info = err.Error()
			}
		case pb.Entry_AccountWrite:
			println("Entry_AccountAdd")
			/*
			err = app.AccountWrite(AccountState,data)
			if err != nil {
				response.Code = 1
				response.Info = err.Error()
			}

			 */

		case pb.Entry_GasAllowanceUpdate:
/*			err = app.GasAllowanceUpdate(AccountState,data)
			if err != nil {
				response.Code = 1
				response.Info = err.Error()
			}

 */
		default:
			response.Code = 1
			response.Info = fmt.Sprintf("Uknown message type %v \n",req.Tx[msgTypePtr])
	}

	if (err != nil) {
		println(err.Error())
	}

	return response
}

//Commit instructs the application to persist the new state.
func (app *DirectoryBlockLeader) Commit() abci.ResponseCommit {
	//app.currentBatch.Commit()
	//need to get hash data / merkle dag from accumulator and place it here.
	return abci.ResponseCommit{Data: []byte{}}
}


func (DirectoryBlockLeader) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
	return abci.ResponseEndBlock{}
}

//------------------------


// when the client wants to know whenever a particular key/value exist, it will call Tendermint Core RPC /abci_query endpoint
func (app *DirectoryBlockLeader) Query(reqQuery abci.RequestQuery) (resQuery abci.ResponseQuery) {
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


func (app *DirectoryBlockLeader) isValid(tx []byte, bytesLen int) (code uint32) {

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

func (app *DirectoryBlockLeader) GetAccountState (publicKey []byte) (acc AccountStateStruct,err error)  {

	keyString := string(publicKey)
	var ok bool

	//Query cache first
	if acc, ok = app.accountState[keyString]; !ok {
		//Not found in cache, read disk
		account, err := GetAccount(publicKey)
		if err !=nil{  //No account for publicKey found!
			if app.Height < app.BootstrapHeight {
				return app.MakeBootStrapAccount(publicKey)
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

func (app *DirectoryBlockLeader) MakeBootStrapAccount(publicKey []byte)(state AccountStateStruct,err error){

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

func (app *DirectoryBlockLeader) WriteKeyValue(account AccountStateStruct, data []byte) (err error) {

	KeyValue := &pb.KeyValue{}

	err = proto.Unmarshal(data,KeyValue)
	if err != nil {
		return err
	}

	KeyValue.Height = uint64(app.Height)
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


func (app *DirectoryBlockLeader) Start(ConfigFile string, WorkingDir string) (*nm.Node, error) {
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

	if database.InitDBs(config, nm.DefaultDBProvider ) !=nil {
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
