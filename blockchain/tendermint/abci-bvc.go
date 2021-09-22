package tendermint

import (
	"bytes"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"

	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/tendermint/tendermint/abci/example/code"

	//"crypto/ed25519"
	_ "crypto/sha256"

	_ "github.com/AccumulateNetwork/SMT/pmt"
	"github.com/tendermint/tendermint/rpc/client/local"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"

	"encoding/json"
	"fmt"
	"time"

	cryptoenc "github.com/tendermint/tendermint/crypto/encoding"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"

	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/version"

	"github.com/AccumulateNetwork/SMT/managed"
	vadb "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/database"

	"sync"

	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	abcitypes "github.com/tendermint/tendermint/abci/types"
)

func loadState(db dbm.DB) State {
	var tmstate State
	tmstate.db = db
	stateBytes, err := db.Get(stateKey)
	if err != nil {
		panic(err)
	}
	if len(stateBytes) == 0 {
		return tmstate
	}
	err = json.Unmarshal(stateBytes, &tmstate)
	if err != nil {
		panic(err)
	}
	return tmstate
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
	abcitypes.BaseApplication
	RetainBlocks int64
	mutex        sync.Mutex
	waitgroup    sync.WaitGroup

	Height int64

	ChainId [32]byte

	tmvalidators map[string]crypto.PubKey
	//Val *validator.ValidatorContext //change to use chainval below instead
	//chainval map[uint64]*validator.ValidatorContext //use this instead to make a group of validators that can be accessed via chain address.

	//begin deprecation
	DB vadb.DB

	state State

	valTypeRegDB dbm.DB
	//end deprecation

	config     *cfg.Config
	Address    crypto.Address
	Key        privval.FilePVKey
	RPCContext rpctypes.Context
	server     service.Service
	amLeader   bool
	dbvc       pb.BVCEntry

	mmdb state.StateDB

	lasthash managed.Hash

	txct int64

	timer time.Time

	submission   chan pb.Submission
	APIClient    coregrpc.BroadcastAPIClient
	Accrpcaddr   string
	RouterClient pb.ApiServiceClient

	LocalClient *local.Local

	chainValidatorNode *validator.Node
}

func NewAccumulatorVMApplication(ConfigFile string, WorkingDir string) *AccumulatorVMApplication {
	name := "kvstore"
	db, err := dbm.NewGoLevelDB(name, WorkingDir)
	if err != nil {
		panic(err)
	}

	tmState := loadState(db)

	app := AccumulatorVMApplication{
		RetainBlocks: 1,       //only retain current block, we will manage our own states
		state:        tmState, //this will save the current state of the blockchain we can use if we need to restart
	}

	_ = app.Initialize(ConfigFile, WorkingDir)

	return &app
}

var _ abcitypes.Application = (*AccumulatorVMApplication)(nil)

func (app *AccumulatorVMApplication) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {

	defer func() {
		// fmt.Printf("*** return Info %v \n", "")
	}()
	//todo: load up the merkle databases to the same state we're at...  We will need to rewind.

	if app.chainValidatorNode == nil {
		panic("Chain Validator Node not set!")
	}

	return abcitypes.ResponseInfo{
		Data:             fmt.Sprintf("{\"size\":%v}", app.state.Size),
		Version:          version.ABCIVersion,
		AppVersion:       ProtocolVersion,
		LastBlockHeight:  app.state.Height,
		LastBlockAppHash: app.state.AppHash,
	}
}

func (app *AccumulatorVMApplication) GetLocalClient() (local.Local, error) {
	return *app.LocalClient, nil
}
func (app *AccumulatorVMApplication) GetAPIClient() (coregrpc.BroadcastAPIClient, error) {
	return app.APIClient, nil
}

func (app *AccumulatorVMApplication) Initialize(ConfigFile string, WorkingDir string) error {
	defer func() {
		// fmt.Printf("*** return Initialize %v \n", "")
	}()
	app.RetainBlocks = 1

	app.waitgroup.Add(1)
	// fmt.Printf("Starting Tendermint (version: %v)\n", version.ABCIVersion)

	app.config = cfg.DefaultConfig()
	app.config.SetRoot(WorkingDir)

	// read private validator
	pv, err := privval.LoadFilePV(
		app.config.PrivValidator.KeyFile(),
		app.config.PrivValidator.StateFile(),
	)
	if err != nil {
		return err
	}

	// fmt.Printf("Node Public Address: 0x%X\n", pv.Key.PubKey.Address())
	app.Key = pv.Key
	app.Address = make([]byte, len(pv.Key.PubKey.Address()))

	copy(app.Address, pv.Key.PubKey.Address())

	v := viper.New()
	v.SetConfigFile(ConfigFile)
	v.AddConfigPath(WorkingDir)
	if err := v.ReadInConfig(); err != nil {

		return fmt.Errorf("viper failed to read config file: %w", err)
	}
	if err := v.Unmarshal(app.config); err != nil {
		return fmt.Errorf("viper failed to unmarshal config: %w", err)
	}
	if err := app.config.ValidateBasic(); err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}

	return nil
}

// SetAccumulateNode will set the chain validator set to use
func (app *AccumulatorVMApplication) SetAccumulateNode(node *validator.Node) {
	defer func() {
		// fmt.Printf("*** return SetAccumulateNode %v \n", "")
	}()
	app.chainValidatorNode = node
}

// InitChain /ABCI call is issued only 1 time at the creation of the chain.
func (app *AccumulatorVMApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	defer func() {
		// fmt.Printf("*** return InitChain %v \n", "")
	}()

	// fmt.Printf("Initializing tendermint chain\n")

	var networkid [32]byte
	networkid[31] = 1
	app.ChainId = networkid

	//register a list of the validators.
	for _, v := range req.Validators {
		r := app.updateValidator(v)
		if r.IsErr() {
			//app.logger.Error("Error updating validators", "r", r)
			// fmt.Printf("Error updating validators \n")
		}
	}

	return abcitypes.ResponseInitChain{AppHash: app.ChainId[:]}
}

// BeginBlock /ABCI / block calls
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
	defer func() {
		// fmt.Printf("*** return BeginBlock %v \n", "")
	}()
	//Identify the leader for this block, if we are the proposer... then we are the leader.
	leader := bytes.Compare(app.Address.Bytes(), req.Header.GetProposerAddress()) == 0
	app.chainValidatorNode.BeginBlock(req.Header.Height, &req.Header.Time, leader)

	app.timer = time.Now()

	// fmt.Printf("Begin Block %d on network id %s\n", req.Header.Height, req.Header.ChainID)

	app.txct = 0

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

	return abcitypes.ResponseBeginBlock{}
}

// CheckTx /ABCI / block calls
///   BeginBlock
///   [CheckTx] <---
///   [DeliverTx]
///   EndBlock
///   Commit
// new transaction is added to the Tendermint Core. Check if it is valid.
func (app *AccumulatorVMApplication) CheckTx(req abcitypes.RequestCheckTx) (rct abcitypes.ResponseCheckTx) {
	defer func() {
		//// fmt.Printf("*** return CheckTx IsErr %v IsOK %v \n", rct.IsErr(), rct.IsOK())
	}()
	//create a default response
	ret := abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}

	//the submission is the format of the Tx input
	sub := new(transactions.GenTransaction)

	//unpack the request
	rem, err := sub.UnMarshal(req.Tx)

	//check to see if there was an error decoding the submission
	if len(rem) != 0 || err != nil {
		//reject it
		return abcitypes.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
			Log: fmt.Sprintf("Unable to decode transaction")}
	}

	err = app.chainValidatorNode.CanTransact(sub)

	if err != nil {
		ret.Code = 2
		ret.GasWanted = 0
		ret.GasUsed = 0
		ret.Info = fmt.Sprintf("entry check failed %v for url %x, %v \n", sub.TransactionType(), sub.ChainID, err)
		return ret
	}

	//if we get here, the TX, passed reasonable check, so allow for dispatching to everyone else
	return ret
}

// DeliverTx ABCI / block calls
//   <BeginBlock>
//   [CheckTx]
//   [DeliverTx] <---
//   <EndBlock>
//   <Commit>
// Invalid transactions, we again return the non-zero code.
// Otherwise, we add it to the current batch.
func (app *AccumulatorVMApplication) DeliverTx(req abcitypes.RequestDeliverTx) (rdt abcitypes.ResponseDeliverTx) {
	defer func() {
		// fmt.Printf("*** return DeliverTx IsOk %v IsErr %v \n", rdt.IsOK(), rdt.IsErr())
	}()
	ret := abcitypes.ResponseDeliverTx{GasWanted: 1, GasUsed: 0, Data: nil, Code: code.CodeTypeOK}

	sub := &transactions.GenTransaction{}

	//unpack the request
	//how do i detect errors?  This causes segfaults if not tightly checked.
	_, err := sub.UnMarshal(req.Tx)
	if err != nil {
		return abcitypes.ResponseDeliverTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
			Log: fmt.Sprintf("Unable to decode transaction")}
	}

	//run through the validation node
	err = app.chainValidatorNode.Validate(sub)

	if err != nil {
		ret.Code = code.CodeTypeUnauthorized
		//ret.GasWanted = 0
		//ret.GasUsed = 0
		//we don't care about failure as far as tendermint is concerned.
		ret.Info = fmt.Sprintf("entry check failed %v on validator %v \n", sub.TransactionType(), err)
		return ret
	}

	//now we need to store the data returned by the validator and feed into accumulator
	app.txct++

	return ret
}

// EndBlock ABCI / block calls
//   BeginBlock
//   [CheckTx]
//   [DeliverTx]
//   EndBlock <---
//   Commit
// Update the validator set
func (app *AccumulatorVMApplication) EndBlock(req abcitypes.RequestEndBlock) (resp abcitypes.ResponseEndBlock) {
	defer func() {
		// fmt.Printf("*** return EndBlock %v \n", "")
	}()
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

	return abcitypes.ResponseEndBlock{} //ValidatorUpdates: app.ValUpdates}
}

// Commit instructs the application to persist the new state.
// ABCI / block calls
//    BeginBlock
//    [CheckTx]
//    [DeliverTx]
//    EndBlock
//    Commit <---
func (app *AccumulatorVMApplication) Commit() (resp abcitypes.ResponseCommit) {
	defer func() {
		//fmt.Printf("*** return Commit %v \n", "")
	}()
	//end the current batch of transactions in the Stateful Merkle Tree

	mdRoot, err := app.chainValidatorNode.EndBlock()
	resp.Data = mdRoot

	if err != nil {
		//should never get here.
		panic(err)
	}

	//this will truncate what tendermint stores since we only care about current state
	if app.RetainBlocks > 0 && app.Height >= app.RetainBlocks {
		//todo: uncomment the next line when we have smt state syncing complete. For now, we are retaining everything for test net
		//resp.RetainHeight = app.Height - app.RetainBlocks + 1
	}

	//save the state
	app.state.Size += app.txct
	app.state.AppHash = mdRoot
	app.state.Height++
	//saveState(app.state)

	duration := time.Since(app.timer)
	fmt.Printf("%d transactions in %f seconds for a TPS of %f\n", app.txct, duration.Seconds(), float64(app.txct)/duration.Seconds())

	return resp
}

//------------------------ Query Stuff ------------------------

func (app *AccumulatorVMApplication) ListSnapshots(
	req abcitypes.RequestListSnapshots) abcitypes.ResponseListSnapshots {

	defer func() {
		// fmt.Printf("*** return ListSnkapshots %v \n", "")
	}()
	req.ProtoMessage()
	return abcitypes.ResponseListSnapshots{}
}

func (app *AccumulatorVMApplication) LoadSnapshotChunk(
	req abcitypes.RequestLoadSnapshotChunk) abcitypes.ResponseLoadSnapshotChunk {

	defer func() {
		// fmt.Printf("*** return LoadSnapshotChunk %v \n", "")
	}()

	//req.Height
	//resp := abcitypes.ResponseLoadSnapshotChunk{}
	//need to get a block of data between markers.
	//resp.Chunk = app.mm.GetState(req.Height)
	return abcitypes.ResponseLoadSnapshotChunk{}
}

func (app *AccumulatorVMApplication) OfferSnapshot(
	req abcitypes.RequestOfferSnapshot) abcitypes.ResponseOfferSnapshot {
	defer func() {
		// fmt.Printf("*** return OfferSnapshot %v \n", "")
	}()
	return abcitypes.ResponseOfferSnapshot{Result: abcitypes.ResponseOfferSnapshot_ABORT}
}

func (app *AccumulatorVMApplication) ApplySnapshotChunk(

	req abcitypes.RequestApplySnapshotChunk) abcitypes.ResponseApplySnapshotChunk {
	defer func() {
		// fmt.Printf("*** return ApplySnapshotChunk %v \n", "")
	}()

	return abcitypes.ResponseApplySnapshotChunk{Result: abcitypes.ResponseApplySnapshotChunk_ABORT}
}

//Query when the client wants to know whenever a particular key/value exist, it will call Tendermint Core RPC /abci_query endpoint
func (app *AccumulatorVMApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {

	defer func() {
		// fmt.Printf("*** return Qirtu %v \n", "")
	}()

	resQuery.Key = reqQuery.Data
	q := pb.Query{}
	err := proto.Unmarshal(reqQuery.Data, &q)
	if err != nil {
		resQuery.Info = fmt.Sprintf("requst is not an Accumulate Query\n")
		resQuery.Code = code.CodeTypeUnauthorized
		return resQuery
	}

	// fmt.Printf("query %s", q.ChainUrl)

	ret, err := app.chainValidatorNode.Query(&q)

	if err != nil {
		resQuery.Info = fmt.Sprintf("%v", err)
		resQuery.Code = code.CodeTypeUnauthorized
		return resQuery
	}

	//if we get here, we have a valid state object, so let's return it.
	resQuery.Code = code.CodeTypeOK
	//return a generic state object for the chain and let the query deal with decoding it
	resQuery.Value = ret

	///implement lazy sync calls. If a node falls behind it needs to have several query calls
	///1 get current height
	///2 get block data for height X
	///3 get block data for given hash
	return
}

func (app *AccumulatorVMApplication) GetName() string {
	defer func() {
		// fmt.Printf("*** return GetName %v \n", "")
	}()
	return app.config.ChainID()
}
func (app *AccumulatorVMApplication) Wait() {
	defer func() {
		// fmt.Printf("*** return Wait %v \n", "")
	}()
	app.waitgroup.Wait()
}

func (app *AccumulatorVMApplication) Start() (node service.Service, err error) {
	defer func() {
		if err != nil {
			// Ensure that waiters are not permanently blocked waiting for us
			app.waitgroup.Done()
		}
	}()

	// create logger
	logger, err := log.NewDefaultLogger(app.config.LogFormat, app.config.LogLevel, false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %w", err)
	}

	// create node
	node, err = nm.New(
		app.config,
		logger,
		proxy.NewLocalClientCreator(app),
		nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
	}
	//node.

	// fmt.Println("Accumulate Start" + app.config.ChainID())

	err = node.Start()
	if err != nil {
		panic(err)
	}

	WaitForGRPC(app.config.RPC.GRPCListenAddress)
	WaitForRPC(app.config.RPC.ListenAddress)

	localns, ok := node.(local.NodeService)
	if !ok {
		return nil, fmt.Errorf("node cannot be used as a local node service")
	}

	app.LocalClient, err = local.New(localns)
	if err != nil {
		return nil, fmt.Errorf("failed to create local client: %w", err)
	}
	client := GetGRPCClient(app.config.RPC.GRPCListenAddress) //makeGRPCClient(app.Accrpcaddr)//app.config.RPC.GRPCListenAddress)

	app.APIClient = client

	defer func() {
		node.Stop()
		node.Wait()
		// fmt.Println("Tendermint Stopped")
	}()

	app.waitgroup.Done()
	node.Wait()
	return node, nil
}

//updateValidator add, update, or remove a validator
func (app *AccumulatorVMApplication) updateValidator(v abcitypes.ValidatorUpdate) abcitypes.ResponseDeliverTx {
	defer func() {
		// fmt.Printf("*** return updateValidator %v \n", "")
	}()
	pubkey, _ := cryptoenc.PubKeyFromProto(v.PubKey)

	fmt.Printf("Val Pub Key 0x%X\n", pubkey.Address())
	/*
	   	if err != nil {
	   		panic(fmt.Errorf("can't decode public key: %w", err))
	   	}
	   	//key := []byte("val:" + string(pubkey.Bytes()))
	   	if v.Power == 0 {
	   		// remove validator
	   		_, found := app.tmvalidators[string(pubkey.Address())]// app.app.state.db.Has(key)
	   		if !found {
	   			pubStr := base64.StdEncoding.EncodeToString(pubkey.Bytes())
	   			return abcitypes.ResponseDeliverTx{
	   				Code: code.CodeTypeUnauthorized,
	   				Log:  fmt.Sprintf("Cannot remove non-existent validator %s", pubStr)}
	   		}
	   //		if !hasKey
	   		//if err = app.app.state.db.Delete(key); err != nil {
	   		//	panic(err)
	   		//}
	   		delete(app.tmvalidators, string(pubkey.Address()))
	   	} else {
	   		// add or update validator
	   		//value := bytes.NewBuffer(make([]byte, 0))
	   		//if err := types.WriteMessage(&v, value); err != nil {
	   		//	return types.ResponseDeliverTx{
	   		//		Code: code.CodeTypeEncodingError,
	   		//		Log:  fmt.Sprintf("Error encoding validator: %v", err)}
	   		//}
	   		//if err = app.app.state.db.Set(key, value.Bytes()); err != nil {
	   		//	panic(err)
	   		//}
	   		app.tmvalidators[string(pubkey.Address())] = pubkey
	   	}
	*/

	// we only update the changes array if we successfully updated the tree
	//app.ValUpdates = append(app.ValUpdates, v)

	return abcitypes.ResponseDeliverTx{Code: code.CodeTypeOK}
}
