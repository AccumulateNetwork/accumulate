package state

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/smt/managed"
	"github.com/AccumulateNetwork/accumulated/smt/pmt"
	"github.com/AccumulateNetwork/accumulated/smt/storage"
	smtDB "github.com/AccumulateNetwork/accumulated/smt/storage/database"
	"github.com/AccumulateNetwork/accumulated/types"
)

const debugStateDBWrites = false

var blockIndexKey = sha256.Sum256([]byte("BlockIndex"))

var ErrNotFound = errors.New("not found")

type transactionStateInfo struct {
	Object  *Object
	ChainId types.Bytes
	TxId    types.Bytes
}

type transactionLists struct {
	validatedTx []*transactionStateInfo //list of validated transaction chain state objects for block
	pendingTx   []*transactionStateInfo //list of pending transaction chain state objects for block
	synthTxMap  map[types.Bytes32]*[]transactionStateInfo
}

// reset will (re)initialize the transaction lists, this should be done on startup and at the end of each block
func (t *transactionLists) reset() {
	t.pendingTx = nil
	t.validatedTx = nil
	t.synthTxMap = make(map[types.Bytes32]*[]transactionStateInfo)
}

type bucket string

const (
	bucketEntry         = bucket("StateEntries")
	bucketTx            = bucket("Transactions")
	bucketMainToPending = bucket("MainToPending") //main TXID to PendingTXID
	bucketPendingTx     = bucket("PendingTx")     //Store pending transaction
	bucketStagedSynthTx = bucket("StagedSynthTx") //store the staged synthetic transactions
	bucketTxToSynthTx   = bucket("TxToSynthTx")   //TXID to synthetic TXID
)

//bucket SynthTx stores a list of synth tx's derived from a tx

func (b bucket) AsString() string {
	return string(b)
}

type blockUpdates struct {
	bucket    bucket
	txId      []*types.Bytes32
	stateData *Object //the latest chain state object modified from a tx
}

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db    *smtDB.Manager
	debug bool
	// TODO:  Need a couple of things:
	//     ChainState interface
	//     Lets you marshal a ChainState to disk, and lets you unmarshal them
	//     later.  Holds the type of the chain, and all the state about the
	//     chain needed to validate transactions on the chain. (accounts for
	//     example Balance, SigSpecGroup (hash), URL)  All ChainState
	//     instances hold the MerkleManagerState for the chain. MDRoot
	//     .
	//     On the other side, allows a ChainState to be unmarshaled for updates
	//     and access.
	bpt        *pmt.Manager           //pbt is the global patricia trie for the application
	blockIndex int64                  //Index of the current block
	rmm        *managed.MerkleManager //rmm is the merkle manager for root values (unique and shared over all chains)
	mm         *managed.MerkleManager //mm is the merkle manager for a Main Chain.  The salt is set by the appId
	pmm        *managed.MerkleManager //pmm is  merkle manaager for Pending Chain, and its salt is created from appId
	bmm        *managed.MerkleManager //bmm is  merkle manaager for block index Chain, and its salt is created from appId
	appId      []byte                 // appId of a Main Chain

	TimeBucket   float64
	mutex        sync.Mutex
	updates      map[types.Bytes32]*blockUpdates
	transactions transactionLists
	sync         sync.WaitGroup
}

func (sdb *StateDB) init(appId []byte, debug bool) (err error) {
	markPower := int64(8)

	sdb.db.AddBucket(bucketEntry.AsString())
	sdb.db.AddBucket(bucketMainToPending.AsString())
	sdb.db.AddBucket(bucketPendingTx.AsString())
	sdb.db.AddBucket(bucketTx.AsString())
	sdb.db.AddBucket(bucketTxToSynthTx.AsString())
	sdb.db.AddBucket(bucketStagedSynthTx.AsString())
	sdb.debug = debug
	if debug {
		sdb.db.AddBucket("Entries-Debug") //items will bet pushed into this bucket as the state entries change
	}

	sdb.updates = make(map[types.Bytes32]*blockUpdates)
	sdb.transactions.reset()

	sdb.bpt = pmt.NewBPTManager(sdb.db)

	pAppId := managed.Add2AppID(appId, managed.PendingOff)
	bAppId := managed.Add2AppID(appId, managed.BlkIdxOff)

	if sdb.mm, err = managed.NewMerkleManager(sdb.db, appId, markPower); err != nil {
		return err
	}
	if sdb.pmm, err = managed.NewMerkleManager(sdb.db, pAppId, markPower); err != nil {
		return err
	}
	if sdb.bmm, err = managed.NewMerkleManager(sdb.db, bAppId, markPower); err != nil {
		return err
	}
	sdb.rmm = sdb.mm.Copy(nil)
	sdb.appId = appId

	ent, err := sdb.GetPersistentEntry(blockIndexKey[:], false)
	if err == nil {
		sdb.blockIndex, _ = common.BytesInt64(ent.Entry)
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}

	return nil
}

// Open database to manage the smt and chain states
func (sdb *StateDB) Open(dbFilename string, appId []byte, useMemDB bool, debug bool) error {
	dbType := "badger"
	if useMemDB {
		dbType = "memory"
	}

	sdb.db = &smtDB.Manager{}
	err := sdb.db.Init(dbType, dbFilename)
	if err != nil {
		return err
	}

	return sdb.init(appId, debug)
}

func (sdb *StateDB) Load(db storage.KeyValueDB, appId []byte, debug bool) error {
	sdb.db = new(smtDB.Manager)
	sdb.db.InitWithDB(db)
	return sdb.init(appId, debug)
}

func (sdb *StateDB) GetDB() *smtDB.Manager {
	return sdb.db
}

func (sdb *StateDB) Sync() {
	sdb.sync.Wait()
}

//GetTx get the transaction by transaction ID
func (sdb *StateDB) GetTx(txId []byte) (tx []byte, pendingTx []byte, syntheticTxIds []byte, err error) {
	tx = sdb.db.Get(bucketTx.AsString(), "", txId)

	pendingTxId := sdb.db.Get(bucketMainToPending.AsString(), "", txId)
	pendingTx = sdb.db.Get(bucketPendingTx.AsString(), "", pendingTxId)

	syntheticTxIds = sdb.db.Get(bucketTxToSynthTx.AsString(), "", txId)

	return tx, pendingTx, syntheticTxIds, nil
}

//AddSynthTx add the synthetic transaction which is mapped to the parent transaction
func (sdb *StateDB) AddSynthTx(parentTxId types.Bytes, synthTxId types.Bytes, synthTxObject *Object) {
	if debugStateDBWrites {
		fmt.Printf("AddSynthTx %X\n", synthTxObject.Entry)
	}
	var val *[]transactionStateInfo
	var ok bool

	parentHash := parentTxId.AsBytes32()
	if val, ok = sdb.transactions.synthTxMap[parentHash]; !ok {
		val = new([]transactionStateInfo)
		sdb.transactions.synthTxMap[parentHash] = val
	}
	*val = append(*val, transactionStateInfo{synthTxObject, nil, synthTxId})
}

//AddPendingTx adds the pending tx raw data and signature of that data to tx,
//signature needs to be a signed hash of the tx.
func (sdb *StateDB) AddPendingTx(chainId *types.Bytes32, txId types.Bytes,
	txPending *Object, txValidated *Object) error {
	_ = chainId
	chainType, _ := binary.Uvarint(txPending.Entry)
	if types.ChainType(chainType) != types.ChainTypePendingTransaction {
		return fmt.Errorf("expecting pending transaction chain type of %s, but received %s",
			types.ChainTypePendingTransaction.Name(), types.TxType(chainType).Name())
	}
	//append the list of pending Tx's, txId's, and validated Tx's.
	sdb.mutex.Lock()
	tsi := transactionStateInfo{txPending, chainId.Bytes(), txId}
	sdb.transactions.pendingTx = append(sdb.transactions.pendingTx, &tsi)
	if txValidated != nil {
		chainType, _ := binary.Uvarint(txValidated.Entry)
		if types.ChainType(chainType) != types.ChainTypeTransaction {
			return fmt.Errorf("expecting pending transaction chain type of %s, but received %s",
				types.ChainTypeTransaction.Name(), types.ChainType(chainType).Name())
		}
		tsi := transactionStateInfo{txValidated, chainId.Bytes(), txId}
		sdb.transactions.validatedTx = append(sdb.transactions.validatedTx, &tsi)
	}
	sdb.mutex.Unlock()
	return nil
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (sdb *StateDB) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	_ = verify
	sdb.Sync()

	if sdb.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data := sdb.db.Get("StateEntries", "", chainId)

	if data == nil {
		return nil, fmt.Errorf("%w: no state defined for %X", ErrNotFound, chainId)
	}

	ret := &Object{}
	err := ret.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("entry in database is not found for %x", chainId)
	}
	//if verify {
	//todo: generate and verify data to make sure the state matches what is in the patricia trie
	//}
	return ret, nil
}

// GetCurrentEntry retrieves the current state object from the database based upon chainId.  Current state either comes
// from a previously saves state for the current block, or it is from the database
func (sdb *StateDB) GetCurrentEntry(chainId []byte) (*Object, error) {
	if chainId == nil {
		return nil, fmt.Errorf("chain id is invalid, thus unable to retrieve current entry")
	}
	var ret *Object
	var err error
	var key types.Bytes32

	copy(key[:32], chainId[:32])

	sdb.mutex.Lock()
	currentState := sdb.updates[key]
	sdb.mutex.Unlock()
	if currentState != nil {
		ret = currentState.stateData
	} else {
		currentState := blockUpdates{}
		currentState.bucket = bucketEntry
		//pull current state entry from the database.
		currentState.stateData, err = sdb.GetPersistentEntry(chainId, false)
		if err != nil {
			return nil, err
		}
		//if we have valid data, store off the state
		ret = currentState.stateData
	}

	return ret, nil
}

// AddStateEntry append the entry to the chain, the subChainId is if the chain upon which
// the transaction is against touches another chain. One example would be an account type chain
// may change the state of the sigspecgroup chain (i.e. a sub/secondary chain) based on the effect
// of a transaction.  The entry is the state object associated with
func (sdb *StateDB) AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *Object) {
	if debugStateDBWrites {
		fmt.Printf("AddStateEntry chainId=%X txHash=%X entry=%X\n", *chainId, *txHash, object.Entry)
	}
	begin := time.Now()

	sdb.TimeBucket = sdb.TimeBucket + float64(time.Since(begin))*float64(time.Nanosecond)*1e-9

	sdb.mutex.Lock()
	updates := sdb.updates[*chainId]
	sdb.mutex.Unlock()

	if updates == nil {
		updates = new(blockUpdates)
		sdb.updates[*chainId] = updates
	}

	updates.txId = append(updates.txId, txHash)
	updates.stateData = object
}

func (sdb *StateDB) writeTxs(mutex *sync.Mutex, group *sync.WaitGroup) error {
	defer group.Done()
	//record transactions
	for _, tx := range sdb.transactions.validatedTx {
		data, _ := tx.Object.MarshalBinary()
		//store the transaction

		txHash := tx.TxId.AsBytes32()
		if val, ok := sdb.transactions.synthTxMap[txHash]; ok {
			var synthData []byte
			for _, synthTxInfo := range *val {
				synthData = append(synthData, synthTxInfo.TxId...)
				synthTxData, err := synthTxInfo.Object.MarshalBinary()
				if err != nil {
					return err
				}
				sdb.rmm.Manager.PutBatch(bucketStagedSynthTx.AsString(), "",
					synthTxInfo.TxId, synthTxData)
				//store the hash of th synthObject in the bpt, will be removed after synth tx is processed
				sdb.bpt.Bpt.Insert(synthTxInfo.TxId.AsBytes32(), sha256.Sum256(synthTxData))
			}
			//store a list of txid to list of synth txid's
			sdb.rmm.Manager.PutBatch(bucketTxToSynthTx.AsString(), "", tx.TxId, synthData)
		}

		mutex.Lock()
		//store the transaction in the transaction bucket by txid
		sdb.rmm.Manager.PutBatch(bucketTx.AsString(), "", tx.TxId, data)
		//insert the hash of the tx object in the BPT
		sdb.bpt.Bpt.Insert(txHash, sha256.Sum256(data))
		mutex.Unlock()
	}

	// record pending transactions
	for _, tx := range sdb.transactions.pendingTx {
		//marshal the pending transaction state
		data, _ := tx.Object.MarshalBinary()
		//hash it and add to the merkle state for the pending chain
		pendingHash := sha256.Sum256(data)

		mutex.Lock()
		//Store the mapping of the Transaction hash to the pending transaction hash which can be used for
		// validation so we can find the pending transaction
		sdb.rmm.Manager.PutBatch("MainToPending", "", tx.TxId, pendingHash[:])

		sdb.mm.Copy(tx.ChainId)
		//store the pending transaction by the pending tx hash
		sdb.rmm.Manager.PutBatch(bucketPendingTx.AsString(), "", pendingHash[:], data)
		mutex.Unlock()
	}

	//clear out the transactions after they have been processed
	sdb.transactions.validatedTx = nil
	sdb.transactions.pendingTx = nil
	sdb.transactions.synthTxMap = make(map[types.Bytes32]*[]transactionStateInfo)
	return nil
}

func (sdb *StateDB) writeChainState(group *sync.WaitGroup, mutex *sync.Mutex, mm *managed.MerkleManager, chainId types.Bytes32) {
	defer group.Done()

	// We get ChainState objects here, instead. And THAT will hold
	//       the MerkleStateManager for the chain.
	//mutex.Lock()
	currentState := sdb.updates[chainId]
	//mutex.Unlock()

	if currentState == nil {
		panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
	}

	//add all the transaction states that occurred during this block for this chain (in order of appearance)
	for _, tx := range currentState.txId {
		//store the txHash for the chains, they will be mapped back to the above recorded tx's
		mm.AddHash(managed.Hash(*tx))
	}

	if currentState.stateData != nil {
		//store the MD root for the state
		mdRoot := mm.MS.GetMDRoot()
		if mdRoot == nil {
			//shouldn't get here, but will reject if I do
			panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		}

		//store the state of the main chain in the state object
		currentState.stateData.MDRoot = types.Bytes32(*mdRoot)

		//now store the state object
		chainStateObject, err := currentState.stateData.MarshalBinary()
		if err != nil {
			panic("failed to marshal binary for state data")
		}

		mutex.Lock()
		sdb.GetDB().PutBatch(bucketEntry.AsString(), "", chainId.Bytes(), chainStateObject)
		// The bpt stores the hash of the ChainState object hash.
		sdb.bpt.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
		mutex.Unlock()
	}
	//TODO: figure out how to do this with new way state is derived
	//if len(currentState.pendingTx) != 0 {
	//	mdRoot := v.PendingChain.MS.GetMDRoot()
	//	if mdRoot == nil {
	//		//shouldn't get here, but will reject if I do
	//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
	//	}
	//	//todo:  Determine how we purge pending tx's after 2 weeks.
	//	sdb.bpt.Bpt.Insert(chainId, *mdRoot)
	//}
}

func (sdb *StateDB) writeBatches() {
	defer sdb.sync.Done()
	sdb.rmm.Manager.EndBatch()
	sdb.bpt.DBManager.EndBatch()
}

func (sdb *StateDB) BlockIndex() int64 {
	return sdb.blockIndex
}

// WriteStates will push the data to the database and update the patricia trie
func (sdb *StateDB) WriteStates(blockHeight int64) ([]byte, int, error) {
	//build a list of keys from the map
	currentStateCount := len(sdb.updates)
	if currentStateCount == 0 {
		//only attempt to record the block if we have any data.
		return sdb.bpt.Bpt.Root.Hash[:], 0, nil
	}

	sdb.blockIndex = blockHeight
	// TODO MainIndex and PendingIndex?
	sdb.AddStateEntry((*types.Bytes32)(&blockIndexKey), new(types.Bytes32), &Object{Entry: common.Int64Bytes(blockHeight)})

	group := new(sync.WaitGroup)
	group.Add(1)
	group.Add(len(sdb.updates))

	mutex := new(sync.Mutex)
	//to try the multi-threading add "go" in front of the next line
	err := sdb.writeTxs(mutex, group)
	if err != nil {
		return sdb.bpt.Bpt.Root.Hash[:], 0, nil
	}

	//then run through the list and record them
	//loop through everything and write out states to the database.
	merkleMgrMap := make(map[types.Bytes32]*managed.MerkleManager)
	for chainId := range sdb.updates {
		merkleMgrMap[chainId] = sdb.mm.Copy(chainId[:])
	}
	for chainId := range sdb.updates {
		//to enable multi-threading put "go" in front
		sdb.writeChainState(group, mutex, merkleMgrMap[chainId], chainId)

		//TODO: figure out how to do this with new way state is derived
		//if len(currentState.pendingTx) != 0 {
		//	mdRoot := v.PendingChain.MS.GetMDRoot()
		//	if mdRoot == nil {
		//		//shouldn't get here, but will reject if I do
		//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		//	}
		//	//todo:  Determine how we purge pending tx's after 2 weeks.
		//	sdb.bpt.Bpt.Insert(chainId, *mdRoot)
		//}
	}
	group.Wait()

	sdb.bpt.Bpt.Update()

	//reset out block update buffer to get ready for the next round
	sdb.sync.Add(1)
	//to enable threaded batch writes, put go in front of next line.
	sdb.writeBatches()

	sdb.updates = make(map[types.Bytes32]*blockUpdates)

	//return the state of the BPT for the state of the block
	if debugStateDBWrites {
		fmt.Printf("WriteStates height=%d hash=%X\n", blockHeight, sdb.RootHash())
	}
	return sdb.RootHash(), currentStateCount, nil
}

func (sdb *StateDB) RootHash() []byte {
	h := sdb.bpt.Bpt.Root.Hash // Make a copy
	return h[:]                // Return a reference to the copy
}

func (sdb *StateDB) EnsureRootHash() []byte {
	sdb.bpt.Bpt.EnsureRootHash()
	return sdb.RootHash()
}
