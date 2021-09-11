package tendermint

import (
	"context"
	"crypto/sha256"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/tendermint/tendermint/abci/example/code"

	//"crypto/ed25519"
	_ "crypto/sha256"
	"net"

	_ "github.com/AccumulateNetwork/SMT/pmt"
	tmnet "github.com/tendermint/tendermint/libs/net"
	"github.com/tendermint/tendermint/rpc/client/local"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
	"google.golang.org/grpc"

	"encoding/json"
	"fmt"
	"time"

	cryptoenc "github.com/tendermint/tendermint/crypto/encoding"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"

	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	dbm "github.com/tendermint/tm-db"

	"os"

	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/version"

	"bytes"

	"github.com/AccumulateNetwork/SMT/managed"
	vadb "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/database"

	"sync"

	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	abcitypes "github.com/tendermint/tendermint/abci/types"
)

//
//var (
//	stateKey        = []byte("stateKey")
//	kvPairPrefixKey = []byte("kvPairKey:")
//
//	ProtocolVersion uint64 = 0x1
//)
//
//type State struct {
//	db      dbm.DB
//	Size    int64  `json:"size"`
//	Height  int64  `json:"height"`
//	AppHash []byte `json:"app_hash"`
//}

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

	bvc *validator.BlockValidatorChain
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
}

func NewAccumulatorVMApplication(ConfigFile string, WorkingDir string) *AccumulatorVMApplication {
	//name := "kvstore"
	//db, err := dbm.NewGoLevelDB(name, WorkingDir)
	//if err != nil {
	//	panic(err)
	//}
	//
	//tmstate := loadState(db)

	app := AccumulatorVMApplication{
		//router: new(router2.Router),
		RetainBlocks: 1, //only retain current block, we will manage our own tmstate
		//state:        tmstate,
	}
	_ = app.Initialize(ConfigFile, WorkingDir)

	return &app
}

var _ abcitypes.Application = (*AccumulatorVMApplication)(nil)

func (app *AccumulatorVMApplication) AddValidator(val *validator.ValidatorContext) error {
	//validators are mapped to registered type id's.
	//getTypeId(val.GetChainId())

	//so perhaps, the validator should lookup typeid by chainid in the validator registration database.

	//TODO: Revisit chainid to address.
	return nil
}

func (app *AccumulatorVMApplication) GetHeight() int64 {
	return app.state.Height
}

func (app *AccumulatorVMApplication) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {

	//todo: load up the merkle databases to the same state we're at...

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

func (app *AccumulatorVMApplication) GetLocalClient() (local.Local, error) {
	return *app.LocalClient, nil
}
func (app *AccumulatorVMApplication) GetAPIClient() (coregrpc.BroadcastAPIClient, error) {
	return app.APIClient, nil
}

func (app *AccumulatorVMApplication) Initialize(ConfigFile string, WorkingDir string) error {
	app.waitgroup.Add(1)
	fmt.Printf("Starting Tendermint (version: %v)\n", version.ABCIVersion)

	app.config = cfg.DefaultConfig()
	app.config.SetRoot(WorkingDir)

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
	app.Accrpcaddr = v.GetString("accumulate.AccRPCAddress")

	//create a connection to the router.
	routeraddress := v.GetString("accumulate.RouterAddress")
	if len(routeraddress) == 0 {
		return fmt.Errorf("accumulate.RouterAddress token not specified in config file")
	}

	conn, err := grpc.Dial(routeraddress, grpc.WithBlock(), grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		return fmt.Errorf("error Openning GRPC client in router")
	}
	//defer conn.Close()
	app.RouterClient = pb.NewApiServiceClient(conn)

	//name := "blockstate"
	//db, err := dbm.NewGoLevelDB(name, WorkingDir)
	//if err != nil {
	//	panic(err)
	//}

	//app.state = loadState(db)

	str := "ValTypeReg"
	fmt.Printf("Creating %s\n", str)
	cdb, err := nm.DefaultDBProvider(&nm.DBContext{ID: str, Config: app.config})
	app.valTypeRegDB = cdb
	if err != nil {
		return fmt.Errorf("failed to create node accumulator database: %w", err)
	}

	networkId := viper.GetString("instrumentation/namespace")
	bvcId := sha256.Sum256([]byte(networkId))
	dbfilename := WorkingDir + "/" + "valacc.db"
	err = app.mmdb.Open(dbfilename, bvcId[:], false, true)

	if err != nil {
		return err
	}

	app.bvc = validator.NewBlockValidatorChain()

	return nil
}

// InitChain /ABCI call
func (app *AccumulatorVMApplication) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	fmt.Printf("Initalizing Accumulator Router\n")

	//this is more like network ID for us...  so perhaps should simply be number 1 to N
	///in theory these can be used to host the administration chains.
	///adi "hash" of network 1 is
	///0x0000000000000000000000000000000000000000000000000000000000000001
	///0x0000000000000000000000000000000000000000000000000000000000000002
	///0x0000000000000000000000000000000000000000000000000000000000000003
	///...
	///url would look like acc://000000000000000000000000000000000000000000000000000000000000000N/whatevs,
	///where N is the network ID
	/// one use case of this chain would be to hold all the pubkeys for all the validtors on the other networks.
	/// the chain would get updated by the dbvc every time a bvc adds or removes a node validator.
	var networkid [32]byte
	networkid[31] = 1
	app.ChainId = networkid

	//for testnet...
	app.createBootstrapAccount()

	////commits will be stored here and key'ed via entry hash.

	//Temporary work around for chicken / egg problem at genesis block
	//we could use admin chains go get around this

	for _, v := range req.Validators {
		r := app.updateValidator(v)
		if r.IsErr() {
			//app.logger.Error("Error updating validators", "r", r)
			fmt.Printf("Error updating validators \n")
		}
	}

	app.submission = make(chan pb.Submission)
	//go app.dispatch()

	return abcitypes.ResponseInitChain{AppHash: app.ChainId[:]}
}

func (app *AccumulatorVMApplication) createBootstrapAccount() {
	tokenUrl := "wileecoyote/ACME"
	adi, chainPath, err := types.ParseIdentityChainPath(&tokenUrl)
	if err != nil {
		panic(err)
	}

	is := state.NewIdentityState(adi)
	keyHash := sha256.Sum256(app.Key.PubKey.Bytes())
	_ = is.SetKeyData(state.KeyTypeSha256, keyHash[:])
	idStateData, err := is.MarshalBinary()
	if err != nil {
		panic(err)
	}

	identity := types.GetIdentityChainFromIdentity(&adi)
	chainid := types.GetChainIdFromChainPath(&chainPath)

	ti := api.NewToken(chainPath, "ACME", 8)

	tas := state.NewToken(chainPath)
	tas.Precision = ti.Precision
	tas.Symbol = ti.Symbol
	tas.Meta = ti.Meta

	tasstatedata, err := tas.MarshalBinary()
	if err != nil {
		panic(err)
	}
	err = app.mmdb.AddStateEntry(identity[:], idStateData)
	if err != nil {
		panic(err)
	}
	err = app.mmdb.AddStateEntry(chainid[:], tasstatedata)
	if err != nil {
		panic(err)
	}
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
	//app.currentBatch = app.db.NewTransaction(true)
	//app.Height = req.Header.Height
	// reset valset changes

	app.timer = time.Now()

	fmt.Printf("Begin Block %d on shard %s\n", req.Header.Height, req.Header.ChainID)
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
	//TODO: Purge any expired entry / chain commits

	//Identify the leader for this block.
	//if we are the proposer... then we are the leader.
	app.amLeader = bytes.Compare(app.Address.Bytes(), req.Header.GetProposerAddress()) == 0

	//fmt.Printf("Public Address: 0x%X\n",app.Address)
	//fmt.Printf("Public Address: 0x%X\n",req.Header.GetProposerAddress())

	if app.amLeader {
		//TODO: determine if anything needs to be done here.
	}
	return abcitypes.ResponseBeginBlock{}
}

// CheckTx /ABCI / block calls
///   BeginBlock
///   [CheckTx] <---
///   [DeliverTx]
///   EndBlock
///   Commit
// new transaction is added to the Tendermint Core. Check if it is valid.
func (app *AccumulatorVMApplication) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {

	//create a default response
	ret := abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}

	//the submission is the format of the Tx input
	sub := &pb.Submission{}

	//unpack the request
	err := proto.Unmarshal(req.Tx, sub)

	//check to see if there was an error decoding the submission
	if err != nil {
		//reject it
		return abcitypes.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
			Log: fmt.Sprintf("Unable to decode transaction")}
	}

	currentState, err := state.NewStateEntry(nil, nil, &app.mmdb)

	//placeholder for special validation rules for synthetic transactions.
	if sub.GetInstruction()&0xFF00 > 0 {
		//need to verify the sender is a legit bvc validator also need the dbvc receipt
		//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
		//not an impostor. Need to figure out how to do this. Right now we just assume the syth request
		//sender is legit.
	}

	//run through the validation routine
	err = app.bvc.Check(currentState, sub.Identitychain, sub.Chainid, 0, 0, sub.Data)

	if err != nil {
		ret.Code = 2
		ret.GasWanted = 0
		ret.GasUsed = 0
		ret.Info = fmt.Sprintf("entry check failed %v for url %s, %v \n", sub.Type, sub.AdiChainPath, err)
		return ret
	}

	//if we get here, the TX, passed reasonable check, so allow for dispatching to everyone else
	return ret
}

//processValidatedSubmissionRequest Figure out what to do with the processed validated transaction.  This may include firing off a synthetic TX or simply
//updating the state of the transaction
func (app *AccumulatorVMApplication) processValidatedSubmissionRequest(vdata *validator.ResponseValidateTX) error {

	for i := range vdata.Submissions {
		//generate a synthetic tx and send to the router.
		//need to track txid to make sure they get processed....
		if app.amLeader {
			//we may want to reconsider making this a go call since using grpc could delay things considerably.
			//we only need to make sure it is processed by the next EndBlock so place in pending queue.
			var sk valacctypes.PrivateKey
			copy(sk[:], app.Key.PrivKey.Bytes())

			//The validator must have created a valid request with the timestamp included
			if vdata.Submissions[i].Timestamp == 0 {
				return fmt.Errorf("invalid synthetic transaction request.  Timestamp not set")
			}

			//derive the ledger to sign the data.
			ledger := types.MarshalBinaryLedgerChainId(vdata.Submissions[i].Chainid, vdata.Submissions[i].Data,
				vdata.Submissions[i].Timestamp)

			///if we are the leader then we are responsible for dispatching the synth tx.
			var err error
			vdata.Submissions[i].Signature, err = app.Key.PrivKey.Sign(ledger)

			if err != nil {
				return fmt.Errorf("error signing validated submission request")
			}

			//using protobuffers grpc is quite slow, so we need to use batch and dispatch
			//buffering these calls up into a batch and send them out at the end of frame instead.
			//this is a good place to experiment with different optimizations
			app.RouterClient.ProcessTx(context.Background(), vdata.Submissions[i])
		}
	}
	return nil
}

// DeliverTx ABCI / block calls
//   <BeginBlock>
//   [CheckTx]
//   [DeliverTx] <---
//   <EndBlock>
//   <Commit>
// Invalid transactions, we again return the non-zero code.
// Otherwise, we add it to the current batch.
func (app *AccumulatorVMApplication) DeliverTx(req abcitypes.RequestDeliverTx) (response abcitypes.ResponseDeliverTx) {

	ret := abcitypes.ResponseDeliverTx{GasWanted: 1, GasUsed: 0, Data: nil, Code: code.CodeTypeUnknownError}

	sub := &pb.Submission{}

	//unpack the request
	err := proto.Unmarshal(req.Tx, sub)

	if err != nil {
		//reject it
		return abcitypes.ResponseDeliverTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
			Log: fmt.Sprintf("Unable to decode transaction")}
	}

	currentState, err := state.NewStateEntry(nil, nil, &app.mmdb)

	//placeholder for special validation rules for synthetic transactions.
	if sub.GetInstruction()&0xFF00 > 0 {
		//need to verify the sender is a legit bvc validator also need the dbvc receipt
		//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
		//not an impostor. Need to figure out how to do this. Right now we just assume the syth request
		//sender is legit.
	}

	//run through the validation routine
	vdata, err := app.bvc.Validate(currentState, sub)

	if err != nil {
		ret.Code = 2
		ret.GasWanted = 0
		ret.GasUsed = 0
		ret.Info = fmt.Sprintf("entry check failed %v on validator %v \n", sub.Type, err)
		return ret
	}
	if vdata == nil {
		ret.Code = 2
		ret.GasWanted = 0
		ret.GasUsed = 0
		ret.Info = fmt.Sprintf("insufficent Entry Data on validator %v \n", err)
		return ret
	}

	/// batch any synthetic tx's generated by the validator
	app.processValidatedSubmissionRequest(vdata)

	/// update the state data for the chain.
	if vdata.StateData != nil {
		for k, v := range vdata.StateData {
			header := state.Chain{}
			err := header.UnmarshalBinary(v)
			if err != nil {
				ret.Code = 2
				ret.GasWanted = 0
				ret.GasUsed = 0
				ret.Info = fmt.Sprintf("invalid state object after submissinon processing")
				return ret
			}
			app.mmdb.AddStateEntry(k[:], v)
		}
	}

	//now we need to store the data returned by the validator and feed into accumulator
	app.txct++

	if err != nil {
		ret.Code = 2
		ret.GasWanted = 0
		return ret
	}

	return response
}

// EndBlock ABCI / block calls
//   BeginBlock
//   [CheckTx]
//   [DeliverTx]
//   EndBlock <---
//   Commit
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
	//end the current batch of transactions in the Stateful Merkle Tree

	mdRoot, err := app.mmdb.WriteStates(app.Height)

	if err != nil {
		//shouldn't get here.
		panic(fmt.Errorf("fatal error, block not set, %v", err))
	}

	if mdRoot != nil {
		resp.Data = mdRoot
	}

	//I think we need to get this from the bpt
	//app.bpt.Bpt.Root.Hash
	//if we have no transactions this block then don't publish anything
	if app.amLeader && app.txct > 0 {

		//now we create a synthetic transaction and publish to the directory block validator
		dbvc := validator.ResponseValidateTX{}
		dbvc.Submissions = make([]*pb.Submission, 1)
		dbvc.Submissions[0] = &pb.Submission{}
		dbvc.Submissions[0].Instruction = 0
		chainAdi := "dbvc"
		chainId := types.GetChainIdFromChainPath(&chainAdi)
		dbvc.Submissions[0].Identitychain = chainId[:] //1 is the chain id of the DBVC
		dbvc.Submissions[0].Chainid = chainId[:]

		dbvc.Submissions[0].Instruction = pb.AccInstruction_Data_Entry //this may be irrelevant...
		dbvc.Submissions[0].Param1 = 0
		dbvc.Submissions[0].Param2 = 0

	}

	//this will truncate what tendermint stores since we only care about current state
	if app.RetainBlocks > 0 && app.Height >= app.RetainBlocks {
		//todo: add this back when done with debugging. right now we are retaining everything for test net until
		//we get bootstrapping sync working...
		//resp.RetainHeight = app.Height - app.RetainBlocks + 1
	}

	//save the state
	app.state.Size += app.txct
	app.state.AppHash = mdRoot
	app.state.Height++
	//saveState(app.state)

	duration := time.Since(app.timer)
	fmt.Printf("TPS: %d in %f for %f\n", app.txct, duration.Seconds(), float64(app.txct)/duration.Seconds())

	return resp
}

//------------------------

func (app *AccumulatorVMApplication) ListSnapshots(
	req abcitypes.RequestListSnapshots) abcitypes.ResponseListSnapshots {
	req.ProtoMessage()
	return abcitypes.ResponseListSnapshots{}
}

func (app *AccumulatorVMApplication) LoadSnapshotChunk(
	req abcitypes.RequestLoadSnapshotChunk) abcitypes.ResponseLoadSnapshotChunk {
	//req.Height
	//resp := abcitypes.ResponseLoadSnapshotChunk{}
	//need to get a block of data between markers.
	//resp.Chunk = app.mm.GetState(req.Height)
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

//Query when the client wants to know whenever a particular key/value exist, it will call Tendermint Core RPC /abci_query endpoint
func (app *AccumulatorVMApplication) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	resQuery.Key = reqQuery.Data

	q := pb.Query{}
	err := proto.Unmarshal(reqQuery.Data, &q)
	if err != nil {
		resQuery.Info = fmt.Sprintf("requst is not an Accumulate Query\n")
		resQuery.Code = code.CodeTypeUnauthorized
		return resQuery
	}

	fmt.Printf("query %s", q.ChainUrl)
	//extract the state for the chain id
	chainState, err := app.mmdb.GetCurrentEntry(q.ChainId)
	if err != nil {
		resQuery.Info = fmt.Sprintf("chain id query, %v", err)
		resQuery.Code = code.CodeTypeUnauthorized
		return resQuery
	}

	chainHeader := state.Chain{}
	err = chainHeader.UnmarshalBinary(chainState.Entry)
	if err != nil {
		resQuery.Info = fmt.Sprintf("unable to extract chain header\n")
		resQuery.Code = code.CodeTypeUnauthorized
		return resQuery
	}
	//if we get here, we have a valid state object, so let's return it.
	resQuery.Code = code.CodeTypeOK
	//return a generic state object for the chain and let the query deal with decoding it
	resQuery.Value = chainState.Entry

	fmt.Printf("Query URI: %s", q.Query)

	///implement lazy sync calls. If a node falls behind it needs to have several query calls
	///1 get current height
	///2 get block data for height X
	///3 get block data for given hash
	return
}

func (app *AccumulatorVMApplication) GetName() string {
	return app.config.ChainID()
}
func (app *AccumulatorVMApplication) Wait() {
	app.waitgroup.Wait()
}

func dialerFunc(ctx context.Context, addr string) (net.Conn, error) {
	return tmnet.Connect(addr)
}

func (app *AccumulatorVMApplication) Start() (*nm.Node, error) {

	// create logger
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
	var err error
	logger, err = tmflags.ParseLogLevel(app.config.LogLevel, logger, cfg.DefaultLogLevel)
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

	fmt.Printf("Public Address: 0x%X\n", pv.Key.PubKey.Address())

	//sk := ed25519.PrivateKey{}

	app.Key = pv.Key
	app.Address = make([]byte, len(pv.Key.PubKey.Address()))
	copy(app.Address, pv.Key.PubKey.Address())

	//this should be done outside of here...
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

	fmt.Println("Accumulate Start" + app.config.ChainID())

	err = node.Start()
	if err != nil {
		panic(err)
	}

	WaitForGRPC(app.config.RPC.GRPCListenAddress)
	WaitForRPC(app.config.RPC.ListenAddress)

	app.LocalClient = local.New(node)

	//makeGRPCServer(app,app.Accrpcaddr )//app.config.RPC.GRPCListenAddress)

	//return &api, nil

	client := GetGRPCClient(app.config.RPC.GRPCListenAddress) //makeGRPCClient(app.Accrpcaddr)//app.config.RPC.GRPCListenAddress)

	app.APIClient = client

	//s := node.Listeners()
	defer func() {
		node.Stop()
		node.Wait()
		fmt.Println("Tendermint Stopped")
	}()

	if node.IsListening() {
		fmt.Print("node is listening")
	}
	app.waitgroup.Done()
	node.Wait()
	return node, nil
}

//updateValidator add, update, or remove a validator
func (app *AccumulatorVMApplication) updateValidator(v abcitypes.ValidatorUpdate) abcitypes.ResponseDeliverTx {
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
