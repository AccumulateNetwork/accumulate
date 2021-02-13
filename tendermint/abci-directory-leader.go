package tendermint

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/accumulator"
	vadb "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/database"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/node"
	"github.com/AccumulateNetwork/accumulated/example/code"
	//"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/accumulated/factom/varintf"
	//"github.com/Workiva/go-datastructures/threadsafe/err"

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

	//	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/accumulator"
	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/merkleDag"

)

const BanListTrigger = -10000

//(4 bytes)    networkid  //magic number0xACCXXXXX


////bvc entry header:
const BVCEntryMaxSize = 1+32+4+8+32


const(
    DDII_type int = 0
    BVCHeight_type int = 1
    Timestamp_type int = 2
    MDRoot_type int = 3
)
type BVCEntry struct {
	Version byte
	DDII []byte
	BVCHeight uint32          /// (4 bytes) Height of master chain block
	Timestamp uint64
	MDRoot valacctypes.Hash

	rawslices [4][]byte
	cache []byte
}

func (entry *BVCEntry) MarshalBinary()([]byte, error) {
	ret := make([]byte,1+1+len(entry.DDII)+4+8+32)

    offset := 0
    endoffset := 1
    varintf.Put(ret[:endoffset],uint64(entry.Version))
    offset++
    endoffset++

	ret[offset] = byte(len(entry.DDII))

	endoffset += int(ret[offset])
	offset++

	copy(ret[offset:endoffset],entry.DDII);
    offset = endoffset-1
    endoffset += 4

	binary.BigEndian.PutUint32(ret[offset:endoffset],entry.BVCHeight)
    offset += 4
    endoffset += 8

	binary.BigEndian.PutUint64(ret[offset:endoffset],entry.Timestamp)
    offset += 8
    endoffset += 32

    copy(ret[offset:endoffset],entry.MDRoot[:])

	return ret[:],nil
}
func (entry *BVCEntry) UnmarshalBinary(data []byte) ([][]byte, error) {


	version, offset := varintf.Decode(data)
	if offset != 1 {
		return nil, fmt.Errorf("Invalid version")
	}
	entry.Version = byte(version)
	ddiilen := data[offset]
	if ddiilen > 32 && ddiilen > 0 {
		return nil, fmt.Errorf("Invalid DDII Length.  Must be > 0 && <= 32")
	}

	offset++
	endoffset := offset + int(ddiilen)
	if endoffset+4+16+32+1 > len(data) {
		return nil, fmt.Errorf("Insuffient data for parsing BVC Entry")
	}
	entry.DDII = data[offset:endoffset+1]

	ret := make([][]byte,4)

	ret[DDII_type] = entry.DDII

	offset = endoffset
	endoffset = offset + 4
	ret[BVCHeight_type] = data[offset:endoffset+1]
	entry.BVCHeight = binary.LittleEndian.Uint32(ret[BVCHeight_type])


	offset = endoffset
	endoffset = offset + 4
	ret[Timestamp_type] = data[offset:endoffset+1]
	entry.Timestamp = binary.LittleEndian.Uint64(ret[Timestamp_type])

	offset = endoffset
	endoffset = offset + 32

	ret[MDRoot_type] = data[offset:endoffset+1]
	copy(entry.MDRoot[:],ret[MDRoot_type])
	return ret,nil
}


//
//
//entry:
//key : bvcheight | chainaddr (12 bytes) : bvcpubkey (32bytes)
//value: MDRoot
//value: signature



type DirectoryBlockLeader struct {

	abci.BaseApplication
//	BootstrapHeight int64
	Height uint64
//	dblock dbvc.DBlock

	//map chain addr to confirmation count
	//confimrationmap map[BVCConfirmationKey]BVCEntryConfirmation
	////per chain accumulator
	//
	ACC            *accumulator.Accumulator // Accumulators to record hashes
	EntryFeed      chan node.EntryHash
	Control        chan bool
	MDFeed         chan *valacctypes.Hash
	md       merkleDag.MD
	AppMDRoot valacctypes.Hash
	//chainid -> height -> MDRoot -> confirmation count
	//map[chainid]AccumulateConfirmation [height][MDRoot]
    bvcentrymap map[BVCConfirmationKey]BVCEntry
	//bvc_masterchain_acc accumulator.Accumulator//map[factom.Bytes32]accumulator.ChainAcc

	DB vadb.DB
}

//lookup by bvcheight | chainaddress
type BVCConfirmationKey struct {
	Height uint64
	ChainAddr uint64
	//ChainId factom.Bytes32
}

//type BVCEntryCandidate struct {
//	Count uint32
//	DDII [][32]byte
//}
//
//type BVCEntryKey struct {
//	Height uint32
//	MDRoot valacctypes.Hash
//}
//
//type BVCEntryConfirmation struct {
//	//should be map[valacctypes.Hash]map[uint32]BVCEntryCandidate
//	candidates map[BVCEntryKey]BVCEntryCandidate
//}
//
//func (candidate *BVCEntryCandidate) Add(ddii []byte) {
//	candidate.Count++
//	candidate.DDII = append(candidate.DDII, ddii)
//}
//
//func NewBVCEntryConfirmation() *BVCEntryConfirmation {
//	conf := BVCEntryConfirmation{}
//	conf.Reset()
//	return &conf
//}
//func (entryconf *BVCEntryConfirmation) Submit(bvcheight uint32, chainaddr uint64, MDRoot *valacctypes.Hash, ddii []byte) {
//	key := BVCEntryKey{bvcheight, *chainaddr }
//
//	entryconf.candidates[key].Add(ddii)
//
//}
//
//func (entryconf *BVCEntryConfirmation) Reset() {
//    entryconf.candidates = make(map[BVCEntryKey]BVCEntryCandidate)
//}
//
//func (entryconf *BVCEntryConfirmation) Winners(threshold int32) (winners []BVCEntryConfirmation, losers []BVCEntryConfirmation) {
//    //split the winners and losers - there should never be any losers.
//	//threshold sets the number of winners required to achieve consensus.
//	//only winning hash needs to be stored
//
//	for k, v := range entryconf.candidates {
//		winners = append(winners,BVCEntryCandidate)
//	}
//
//	entryconf.candidates
//	return nil, nil
//}
//type BVCValidator struct {
//	bvcentrymap map[uint32]BVCEntry
//}

func NewDirectoryBlockLeader() *DirectoryBlockLeader {
	app := DirectoryBlockLeader{
//		db: db,
		//router: new(router2.Router),

		//EntryFeed : make(chan node.EntryHash, 10000),
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
	//only temporary... create a valid key
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


	err := proto.Unmarshal(req.GetTx(),&header)
	if err != nil {
		return abci.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0}
	}

	err = app.verifyBVCMasterChain(header.GetBvcMasterChainAddr())
	if err != nil { //add validation here.
		//quick filter to see if the request if from a valid master chain
		return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0}
	}

	switch header.GetInstruction() {
	case pb.DBVCInstructionHeader_BVCEntry:
		//Step 2: resolve DDII of BVC against VBC validator
		bvcreq := pb.BVCEntry{}

		err = proto.Unmarshal(req.GetTx(),&bvcreq)

		if err != nil {
			return abci.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
				Log: fmt.Sprintf("Unable to decode BVC Protobuf Transaction") }
		}

		bve := BVCEntry{}
		bve.UnmarshalBinary(bvcreq.GetEntry())

		//resolve the validator's bve to obtain public key for given height
		pub, err := app.resolveDDIIatHeight(bve.DDII, bve.BVCHeight)
		if err != nil {
			return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0,
			    Log: fmt.Sprintf("Unable to resolve DDII at Height %d", bve.BVCHeight) }
		}

		//Step 3: validate signature of signed accumulated merkle dag root

		if !ed25519.Verify(pub, bvcreq.GetEntry(), bvcreq.GetSignature()) {
			println("Invalid Signature")
			return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0,
			                            Log: "Invalid Signature" }
		}
	default:
		return abci.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0, Log : "Bad Instruction Header"}

	}
	//Step 4: if signature is valid send dispatch to accumulator directory block
	return abci.ResponseCheckTx{Code: code.CodeTypeOK, GasWanted: 1}
}


func (app *DirectoryBlockLeader) InitChain(req abci.RequestInitChain) abci.ResponseInitChain {
	fmt.Printf("Initalizing Accumulator Router\n")
	//app.router.Init(EntryFeed, int(AccNumber))
    //go router.Run()
    //allocate all BVC chains
    //initialize all chain ids
//	app.confimrationmap = make(map[BVCConfirmationKey]BVCEntryConfirmation)

	//TODO query something to resolve all BVC Master Chains
	app.ACC = new(accumulator.Accumulator)
	//app.ACCs = append(app.ACCs, acc)


	chainid := sha256.Sum256([]byte("dbvc"))

	app.EntryFeed, app.Control, app.MDFeed = app.ACC.Init(&app.DB, (*valacctypes.Hash)(&chainid))

	//need to set
	//app.AppMDRoot //load state
	return abci.ResponseInitChain{}
}

// ------ BeginBlock -> DeliverTx -> EndBlock -> Commit
// When Tendermint Core has decided on the block, it's transferred to the application in 3 parts:
// BeginBlock, one DeliverTx per transaction and EndBlock in the end.

//Here we create a batch, which will store block's transactions.
func (app *DirectoryBlockLeader) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	app.AppMDRoot.Extract(req.Hash)
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

	bve := BVCEntry{}
	slices, _ := bve.UnmarshalBinary(bvcreq.GetEntry())

	bvcheight := bve.BVCHeight

	//resolve the validator's bve to obtain public key for given height
	bvcpubkey, err := app.resolveDDIIatHeight(bve.DDII, bvcheight)
	if err != nil {
		return abci.ResponseDeliverTx{Code: 2, GasWanted: 0}
	}
	//everyone verify...

	if ed25519.Verify(bvcpubkey, bvcreq.GetEntry(), bvcreq.GetSignature()) {
		println("Invalid Signature")
		return abci.ResponseDeliverTx{Code: 3, GasWanted: 0}
	}

	var chain []byte
    //TODO. find out if we need full chain or if we can just use address.
	binary.BigEndian.PutUint64(chain,bvcreq.GetHeader().BvcMasterChainAddr)

	app.md.AddToChain(bve.MDRoot)

	//index the events to let BVC know MDRoot has been secured so that consensus can be achieved
	response.Events = []abci.Event{
		{
			Type: "bvc",
			Attributes: []abci.EventAttribute{
				//want to be able to search by BVC chain.
				{Key: []byte("chain"), Value: chain, Index: true},
				//want to be able to search by height, but probably should be AND'ed with the chain
				{Key: []byte("height"), Value: slices[BVCHeight_type], Index: true},
				//want to be able to search by ddii (optional AND'ed with chain or height)
				{Key: []byte("ddii"), Value: slices[DDII_type], Index: true},
				//don't care about searching by bvc timestamp or valacc hash
				{Key: []byte("timestamp"), Value: slices[Timestamp_type], Index: false},
				{Key: []byte("mdroot"), Value: slices[MDRoot_type], Index: false},
			},
		},
	}
	response.Code = code.CodeTypeOK
	return response
}

//Commit instructs the application to persist the new state.
func (app *DirectoryBlockLeader) Commit() abci.ResponseCommit {
    //is folding in prev block hash necessary
	var hash valacctypes.Hash
	hash.Extract(app.md.GetMDRoot().Bytes())
	app.AppMDRoot = *hash.Combine(app.AppMDRoot)
	return abci.ResponseCommit{Data: app.AppMDRoot.Bytes()}
}


func (app *DirectoryBlockLeader) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
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
