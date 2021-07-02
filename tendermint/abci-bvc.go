package tendermint

import (
	//"crypto/ed25519"
	//"crypto/ed25519"
	"crypto/sha256"
	"time"

	//"encoding/base64"

	//"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/accumulator"
	"github.com/AccumulateNetwork/accumulated/example/code"
	"github.com/AdamSLevy/jsonrpc2/v14"
	cryptoenc "github.com/tendermint/tendermint/crypto/encoding"
	//"github.com/Factom-Asset-Tokens/factom/varintf"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	abcicli "github.com/tendermint/tendermint/abci/client"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"
	//"github.com/tendermint/tendermint/types"

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

	"bytes"
	"github.com/AccumulateNetwork/SMT/smt"
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
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
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

	tmvalidators map[string]crypto.PubKey
	//Val *validator.ValidatorContext //change to use chainval below instead
	chainval map[uint64]*validator.ValidatorContext //use this instead to make a group of validators that can be accessed via chain address.


	//begin deprecation
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
	//end deprecation

	config *cfg.Config
	Address crypto.Address
	Key privval.FilePVKey
	RPCContext rpctypes.Context
	server service.Service
	amLeader bool
    dbvc pb.BVCEntry



	mmdb smtdb.Manager
	mm smt.MerkleManager
	lasthash smt.Hash

	txct int32

	timer time.Time



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
		//Val : nil,
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

	//TODO: Revisit chainid to address.
	hash :=sha256.Sum256(val.GetValidatorChainId())
	app.chainval[binary.BigEndian.Uint64(hash[:])] = val
	//tmp
	//app.Val = val
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

	dbfilename := WorkingDir + "/" + "valacc.db"

	dbtype := "badger"
	//dbtype := "memory" ////for kicks just create an in-memory database for now
	app.mmdb.Init(dbtype,dbfilename)


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


	//create a new accumulator
	acc := new(accumulator.Accumulator)
	app.ACCs = append(app.ACCs, acc)

	/*
	//i don't think we want an accumulator for each type... only
	str := "accumulator-" + "rocky" //*app.Val.GetInfo().GetNamespace()// + "_" + *app.Val.GetInfo().GetInstanceName()

	if str != req.ChainId {
		fmt.Printf("Invalid chain validator\n")
		return abcitypes.ResponseInitChain{}
	}

	 */
	app.ChainId = sha256.Sum256([]byte(req.ChainId))
    //hchain := valacctypes.Hash(app.ChainId)

	//entryFeed, control, mdHashes := acc.Init(&app.DB, (*valacctypes.Hash)(&app.ChainId))
	//app.EntryFeeds = append(app.EntryFeeds, entryFeed)
	//app.Controls = append(app.Controls, control)
	//app.MDFeeds = append(app.MDFeeds, mdHashes)
	////spin up the accumulator
	//go acc.Run()

	//app.router.Init(app.EntryFeed, 1)
	//go app.router.Run()
	//app.router.Init(EntryFeed, int(AccNumber))
    //go router.Run()

    //load to current state
   // app.mm.

	app.mm.Init(&app.mmdb,8)


	////an entry bucket --> do do determine if
	//app.mmdb.AddBucket("Entry")
	//
	////commits will be stored here and key'ed via entry hash.
	//app.mmdb.AddBucket("Commit")

	//launch the hash update thread

	//go app.mm.Update()

	for _, v := range req.Validators {
		r := app.updateValidator(v)
		if r.IsErr() {
			//app.logger.Error("Error updating validators", "r", r)
			fmt.Printf("Error updating validators \n")
		}
	}

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
	app.timer = time.Now()

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
	app.amLeader = bytes.Compare( app.Address.Bytes(), req.Header.GetProposerAddress() ) == 0


	//fmt.Printf("Public Address: 0x%X\n",app.Address)
	//fmt.Printf("Public Address: 0x%X\n",req.Header.GetProposerAddress())

	if app.amLeader {
        //TODO: determine if anything needs to be done here.
	}

	//app.lasthash = smt.Hash{}




	//todo: look at changing this to be queried rather than passed to all validators, because they may not need it
	//chainid := req.GetHeader().ChainID
	//for _, v := range app.chainval {
		//v.SetCurrentBlock(req.Header.Height, &req.Header.Time, &chainid)
		//fmt.Printf("Setting current block info for validator %d",k)
	//}
	//app.Val.SetCurrentBlock(req.Header.Height,&req.Header.Time,&chainid)
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

	//create a default response
	ret := abcitypes.ResponseCheckTx{Code: 0, GasWanted: 1}

	sub := &pb.Submission{}

	//unpack the request
	err := proto.Unmarshal(req.Tx,sub)

	if err != nil {
		//reject it
		return abcitypes.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
			Log: fmt.Sprintf("Unable to decode transaction") }
	}

	var val *validator.ValidatorContext
	//resolve the validator's bve to obtain public key for given height
    if v, ok := app.chainval[sub.Address]; ok {
    	//if not ok, then we probably need to assign a generic default entry validator?
        val = v
	} else {
		return abcitypes.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0,
			Log: fmt.Sprintf("Validator not found for chain address %X", sub.Address ) }
	}

	//+ := val.Check(sub.Instruction,sub.Param1, sub.Param2,sub.Data)

	//check the type of transaction
	switch sub.GetType() {
	case pb.Submission_Data_Entry:

	//case pb.Submission_Entry_Reveal:
			//need to check to see if a segwit for the data exists
			//compute entry hash
			//ask validator to do a quick check on command.
			err := val.Check(sub.Instruction, sub.Param1, sub.Param2, sub.Data)
			if err != nil {
				ret.Code = 2
				ret.GasWanted = 0
				ret.GasUsed = 0
				ret.Info = fmt.Sprintf("Entry check failed %v on validator %s \n",sub.Type, val.GetInfo().GetNamespace())
				return ret
			}
	case pb.Submission_Token_Transaction:
		err := val.Check(sub.Instruction, sub.Param1, sub.Param2, sub.Data)
		if err != nil {
			ret.Code = 2
			ret.GasWanted = 0
			ret.GasUsed = 0
			ret.Info = fmt.Sprintf("Entry check failed %v on validator %s \n",sub.Type, val.GetInfo().GetNamespace())
			return ret
		}
		//verify chain commit signature checks out
		//verify EC has a balance
	case pb.Submission_Data_Chain_Creation:

	//case pb.Submission_Data_Entry:
		//val.
	//case pb.Submission_SyntheticTransaction:
	case pb.Submission_Key_Update:
			//do nothing for now, is this even needed?
	default:
			ret.Code = 1
			ret.Info = fmt.Sprintf("Unknown message type %v on address %v \n",sub.Type, sub.Address)
			return ret
	}
	if err != nil {
		ret.Code = 2
		ret.GasWanted = 0
		return ret
	}


	//if we get here, the TX, passed reasonable check, so allow for dispatching to everyone else
	return ret
}

func (app *AccumulatorVMApplication) processValidatedSubmissionRequest(vdata *validator.ResponseValidateTX) error {
	for i := range vdata.Submissions {

		hash := smt.Hash(sha256.Sum256(vdata.Submissions[i].Data))
		switch vdata.Submissions[i].Type {
		case pb.Submission_Scratch_Entry:
			//generate a key for the chain entry
			//store to scratch DB.
			app.mm.AddHash(hash)

		case pb.Submission_Data_Entry:

            //if we get to this point we can move scratch chain to this chain perhaps and remove scratch chain?
            //remove from scratch DB
			app.mm.AddHash(hash)
		default:
			//generate a synthetic tx and pass to the next round. keep doing that until validators in subsiquent rounds
			//reduce Submissions to Data Entries on their appropriate chains
			//

            //txid stack
            chash := valacctypes.Hash(hash)
			commit, _ /*txid*/ := GenerateCommit(vdata.Submissions[i].Data,&chash,false)


			//need to track txid to make sure they get processed....
			if app.amLeader {

				var sk valacctypes.PrivateKey
				copy(sk[:],app.Key.PrivKey.Bytes())

                err := SignCommit(sk,commit)

                //now we need to make a new submission that has the segwit commit block added.
                //revisit this...  probably need to
                //store the offset to the segwit
                vdata.Submissions[i].Param1 = uint64(len(vdata.Submissions[i].Data)) //signed
				vdata.Submissions[i].Data = append(vdata.Submissions[i].Data, commit...)

                if err != nil {
                	return fmt.Errorf("Error signing validated submission request")
				}

				var c jsonrpc2.Client

				var result int
				err = c.Request(nil, "http://localhost:26611", "broadcast_tx_sync", vdata.Submissions[i], &result)

				if err != nil {
					return fmt.Errorf("Error dispatching request")
				}
			}

		}
	}
	return nil
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

	ret := abcitypes.ResponseDeliverTx{GasWanted: 1, GasUsed: 0, Data: nil, Code: code.CodeTypeUnknownError}

	sub := &pb.Submission{}

	//unpack the request
	err := proto.Unmarshal(req.Tx,sub)

	if err != nil {
		//reject it
		return abcitypes.ResponseDeliverTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
			Log: fmt.Sprintf("Unable to decode transaction") }
	}
	//resolve the validator's bve to obtain public key for given height
	if val, ok := app.chainval[sub.Address]; ok {
		//check the type of transaction
		switch sub.Type {
		case pb.Submission_Token_Transaction:
			//ask validator to do a quick check on command.
            //
			//data, err := val.Validate(sub.Instruction, sub.Param1, sub.Param2, sub.Data)
			vdata, err := val.Validate(sub.Instruction, sub.Param1, sub.Param2, sub.Data)

			if err != nil {
				ret.Code = 2
				ret.GasWanted = 0
				ret.GasUsed = 0
				ret.Info = fmt.Sprintf("Entry check failed %v on validator %v \n",sub.Type, app.chainval[sub.Address])
				return ret
			}
			if vdata == nil {

				ret.Code = 2
				ret.GasWanted = 0
				ret.GasUsed = 0
				ret.Info = fmt.Sprintf("Insufficent Entry Data on validator %v \n", app.chainval[sub.Address])
				return ret
			}
			// if we have vdata, then we need to figure out what to do with it.
			app.processValidatedSubmissionRequest(vdata)

			//now we need to store the data returned by the validator and feed into accumulator
			//app.mmdb.Get
			app.txct++

		case pb.Submission_Key_Update:
			//do nothing for now
		case pb.Submission_Identity_Creation:
			//do nothing fo rnow
		default:
			ret.Code = 1
			ret.Info = fmt.Sprintf("Unknown message type %v on address %v \n",sub.Type, sub.Address)
			return ret
		}
		if err != nil {
			ret.Code = 2
			ret.GasWanted = 0
			return ret
		}
	}


	code := 0
	addr := binary.BigEndian.Uint64(req.Tx)
	//addr := req.GetType()
	if val, ok := app.chainval[addr]; ok {
		val.Validate(sub.GetInstruction(),sub.Param1,sub.Param2, sub.GetData())
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
	//		response.Info = fmt.Sprintf("Unknown message type %v \n",req.Tx[msgTypePtr])
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

	//this can be dangerous,  probably need a better way to sync explicitly
//	for len(app.mm.HashFeed) > 0 {
//		//intentional busy wait
//	}
//	time.Sleep(time.Millisecond)


	app.mmdb.EndBatch()

	mdroot := app.mm.MS.GetMDRoot()

	if mdroot == nil {
		mdroot = new(smt.Hash)
	}

	state := app.mm.GetState(app.mm.GetElementCount())
	if state != nil {
		if state.GetMDRoot() != nil {
			copy(mdroot.Bytes(), state.GetMDRoot().Bytes())
		}
	}

	//this seems a bit hacky... TODO: revisit...
	//if len(app.lasthash) != 0 {
	//	idx := int64(-1)
	//	for ok := true; ok; ok = idx<0 {
	//		idx := app.mm.GetIndex(app.lasthash)
	//		if idx >= 0 {
	//			state := app.mm.GetState(idx)
	//			state.EndBlock()
	//		}
	//	}
	//}

	if app.amLeader {
		//app.dbvc.Header.BvcMasterChainDDII = DDII_type
		//app.dbvc.Header.BvcValidatorDDII = me
		//app.dbvc.Header.Instruction = app.dbvc.Header.Instruction.BVCEntry
		//app.dbvc.Header.Version = 1
		///build the entry
		bve := BVCEntry{}
		bve.Version = 1
		bve.BVCHeight = app.Height
	//	bve.DDII = me
		copy(bve.MDRoot.Bytes(), mdroot.Bytes())
	}
	duration := time.Since(app.timer)
	fmt.Printf("TPS: %d in %f for %f\n", app.txct, duration.Seconds(), float64(app.txct) / duration.Seconds() )
	//return resp
	return abcitypes.ResponseCommit{Data: mdroot.Bytes()}
}


//------------------------


func (app *AccumulatorVMApplication) ListSnapshots(
	req abcitypes.RequestListSnapshots) abcitypes.ResponseListSnapshots {
	req.ProtoMessage()
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

	fmt.Printf("Public Address: 0x%X\n", pv.Key.PubKey.Address())

	//sk := ed25519.PrivateKey{}

	app.Key = pv.Key //.PrivKey
	app.Address = make([]byte, len(pv.Key.PubKey.Address()))
	copy(app.Address, pv.Key.PubKey.Address())
	//initialize the accumulator database
	//TODO: fix the following.  there will be 1 to many validators in a single accumulator, so it should be tagged via chainid of accumulator i.e "accumulator_<chain_ddii>"

	str := "accumulator_" + app.config.ChainID() //*app.Val.GetInfo().GetNamespace()// + "_" + *app.Val.GetInfo().GetInstanceName()
	fmt.Printf("Creating %s\n", str)
	//
	//db2, err := nm.DefaultDBProvider(&nm.DBContext{str, app.config})
	//if err != nil {
	//	return nil,fmt.Errorf("failed to create node accumulator database: %w", err)
	//}


    //app.DB.db2 = db2
	//accumulator database
	//dir := WorkingDir + "/" + str + ".db"

	//app.AccumulatorDB, err := dbm.NewDB(str,dbm.BadgerDBBackend,dir)
	//if err != nil {
	//	return nil,fmt.Errorf("failed to create node accumulator database: %w", err)
	//}

	//app.DB.InitDB(db2)
	//db.Init(i)


	//initialize the validator databases
	//if app.Val.InitDBs(app.config, nm.DefaultDBProvider ) !=nil {
	//	fmt.Println("DB Error")
	//	return nil,nil //TODO
	//}


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


// add, update, or remove a validator
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
