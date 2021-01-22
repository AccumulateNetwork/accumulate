package tendermint

import (
	"encoding/binary"
	//"encoding/binary"
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
    //"crypto/ed25519"
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

//(4 bytes)    networkid  //magic number0xACCXXXXX


////bvc entry header:
//type BVCEntryStruct {
//
//    uint32 BVCHeight          /// (4 bytes) Height of master chain block
//	uint32 MasterChainAddress /// (8 bytes) Address of chain
//
//}
//
//
//entry:
//key : bvcheight | chainaddr (12 bytes) : bvcpubkey (32bytes)
//value: mrhash
//value: signature



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

func (app *DirectoryBlockLeader) resolveDDIIatHeight(ddii []byte, bvcheight uint32) (ed25519.PublicKey, error) {
    //just give me a key...

	fmt.Printf("%s", string(ddii[:]))
	//need to find out what the public key for ddii was at height bvcheight
	pub, _, err := ed25519.GenerateKey(nil)
	return pub, err
}

func (app *DirectoryBlockLeader) verifyBVCMasterChain(addr uint64) error {

	return nil
}

// new transaction is added to the Tendermint Core. Check if it is valid.
func (app *DirectoryBlockLeader) CheckTx(req abci.RequestCheckTx) abci.ResponseCheckTx {
	//the ABCI request here is a Tx that consists data delivered from the BVC protocol buffer
    //data here can only come from an authorized VBC validator, otherwise they will be rejected
	//Step 1: check which BVC is sending the request and see if it is a valid Master Chain.
	header := pb.DBVCInstructionHeader{}

	proto.Unmarshal(req.GetTx(),&header) //if first 4 bytes are valid
	//ret := abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}


	err := app.verifyBVCMasterChain(header.GetBvcMasterChainAddr())
	if err != nil { //add validation here.
		//quick filter to see if the request if from a valid master chain
		return abci.ResponseCheckTx{Code: 1, GasWanted: 0}
	}

	switch header.GetInstruction() {
	case pb.DBVCInstructionHeader_BVCEntry:
		//Step 2: resolve DDII of BVC against VBC validator


		bvcreq := pb.BVCEntry{}

		err = proto.Unmarshal(req.GetTx(),&bvcreq)

		if err != nil {
			return abci.ResponseCheckTx{Code: 3, GasWanted: 0}
		}

		pub, err := app.resolveDDIIatHeight(bvcreq.GetBvcValidatorDDII(), binary.LittleEndian.Uint32(bvcreq.GetEntry()[0:4]))
		if err != nil {
			return abci.ResponseCheckTx{Code: 2, GasWanted: 0}
		}

		//Step 3: validate signature of signed accumulated MR

		if !ed25519.Verify(pub, bvcreq.GetEntry(), bvcreq.GetSignature()) {
			println("Invalid Signature")
			return abci.ResponseCheckTx{Code: 4, GasWanted: 0}
		}

	}
	//Step 4: if signature is valid send dispatch to accumulator directory block
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

	//if we get this far, than it has passed check tx,
	bvcreq := pb.BVCEntry{}
	err := proto.Unmarshal(req.GetTx(),&bvcreq)
	if err != nil {
		return abci.ResponseDeliverTx{Code: 2, GasWanted: 0}
	}

	bvcheight := binary.LittleEndian.Uint32(bvcreq.GetEntry()[0:4])
	bvcpubkey, err := app.resolveDDIIatHeight(bvcreq.GetBvcValidatorDDII(), bvcheight)
	if err != nil {
		return abci.ResponseDeliverTx{Code: 2, GasWanted: 0}
	}
	//everyone verify...

	if ed25519.Verify(bvcpubkey, bvcreq.GetEntry(), bvcreq.GetSignature()) {
		println("Invalid")
		return abci.ResponseDeliverTx{Code: 3, GasWanted: 0}
	}

	//timestamp := binary.LittleEndian.Uint32(bvcreq.GetEntry()[4:8])
	//mrhash := bvcreq.GetEntry()[8:40]

	key, err := proto.Marshal(bvcreq.GetHeader())
	err = app.WriteKeyValue(key,req.GetTx())
	if err != nil {
		response.Code = 1
		response.Info = err.Error()
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

func (app *DirectoryBlockLeader) WriteKeyValue(key []byte, data []byte) (err error) {

	//KeyValue := &pb.KeyValue{}
	//
	//err = proto.Unmarshal(data,KeyValue)
	//if err != nil {
	//	return err
	//}
	//
	//KeyValue.Height = uint64(app.Height)
	//data,err = proto.Marshal(KeyValue)

	//AccountAdd.

	err = database.KvStoreDB.Set(key,data)
	if err != nil{
		fmt.Printf("WriteKeyValue err %v\n",err)
	}

	return err
}


func (app *DirectoryBlockLeader) Start(ConfigFile string, WorkingDir string) (*nm.Node, error) {
	fmt.Printf("Starting Tendermint (version: %v)\n", version.ABCIVersion)

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
