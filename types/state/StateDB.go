package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/smt/pmt"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/tendermint/tendermint/libs/log"
)

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
	bucketEntry            = bucket("StateEntries")
	bucketTx               = bucket("Transactions")
	bucketMainToPending    = bucket("MainToPending") //main TXID to PendingTXID
	bucketPendingTx        = bucket("PendingTx")     //Store pending transaction
	bucketStagedSynthTx    = bucket("StagedSynthTx") //store the staged synthetic transactions
	bucketTxToSynthTx      = bucket("TxToSynthTx")   //TXID to synthetic TXID
	bucketMinorAnchorChain = bucket("MinorAnchorChain")

	markPower = int64(8)
)

//bucket SynthTx stores a list of synth tx's derived from a tx

func (b bucket) String() string { return string(b) }
func (b bucket) Bytes() []byte  { return []byte(b) }

type blockUpdates struct {
	bucket    bucket
	txId      []*types.Bytes32
	stateData *Object //the latest chain state object modified from a tx
}

// StateDB the state DB will only retrieve information out of the database.  To store stuff use PersistentStateDB instead
type StateDB struct {
	db           *database.Manager
	mm           *managed.MerkleManager
	debug        bool
	bpt          *pmt.Manager //pbt is the global patricia trie for the application
	TimeBucket   float64
	mutex        sync.Mutex
	updates      map[types.Bytes32]*blockUpdates
	writes       map[storage.Key][]byte
	transactions transactionLists
	sync         sync.WaitGroup
	logger       log.Logger
}

func (s *StateDB) SetLogger(logger log.Logger) {
	if logger != nil {
		logger = logger.With("module", "db")
	}
	s.logger = logger
}

func (s *StateDB) logInfo(msg string, keyVals ...interface{}) {
	if s.logger != nil {
		// TODO Maybe this should be Debug?
		s.logger.Info(msg, keyVals...)
	}
}

func (s *StateDB) init(debug bool) {
	s.debug = debug
	s.updates = make(map[types.Bytes32]*blockUpdates)
	s.writes = map[storage.Key][]byte{}
	s.transactions.reset()

	s.bpt = pmt.NewBPTManager(s.db)
	managed.NewMerkleManager(s.db, markPower)
}

// Open database to manage the smt and chain states
func (s *StateDB) Open(dbFilename string, useMemDB bool, debug bool) (err error) {
	dbType := "badger"
	if useMemDB {
		dbType = "memory"
	}

	s.db, err = database.NewDBManager(dbType, dbFilename)
	if err != nil {
		return err
	}

	s.mm, err = managed.NewMerkleManager(s.db, markPower)
	if err != nil {
		return err
	}

	s.init(debug)
	return nil
}

func (s *StateDB) Load(db storage.KeyValueDB, debug bool) (err error) {
	s.db = new(database.Manager)
	s.db.InitWithDB(db)
	s.mm, err = managed.NewMerkleManager(s.db, markPower)
	if err != nil {
		return err
	}

	s.init(debug)
	return nil
}

func (s *StateDB) GetDB() *database.Manager {
	return s.db
}

func (s *StateDB) Sync() {
	s.sync.Wait()
}

//GetTxRange get the transaction id's in a given range
func (s *StateDB) GetTxRange(chainId *types.Bytes32, start int64, end int64) (hashes []types.Bytes32, maxAvailable int64, err error) {
	s.mutex.Lock()
	h, err := s.mm.GetRange(chainId[:], start, end)
	s.mm.SetChainID(chainId[:])
	maxAvailable = s.mm.GetElementCount()
	s.mutex.Unlock()
	if err != nil {
		return nil, 0, err
	}
	for i := range h {
		hashes = append(hashes, h[i].Bytes32())
	}

	return hashes, maxAvailable, nil
}

//GetTx get the transaction by transaction ID
func (s *StateDB) GetTx(txId []byte) (tx []byte, err error) {
	tx, err = s.db.Key(bucketTx, txId).Get()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

//GetPendingTx get the pending transactions by primary transaction ID
func (s *StateDB) GetPendingTx(txId []byte) (pendingTx []byte, err error) {

	pendingTxId, e := s.db.Key(bucketMainToPending, txId).Get()
	if e != nil {
		return nil, err
	}
	pendingTx, err = s.db.Key(bucketPendingTx, pendingTxId).Get()
	if err != nil {
		return nil, err
	}

	return pendingTx, nil
}

// GetSyntheticTxIds get the transaction id list by the transaction ID that spawned the synthetic transactions
func (s *StateDB) GetSyntheticTxIds(txId []byte) (syntheticTxIds []byte, err error) {

	syntheticTxIds, err = s.db.Key(bucketTxToSynthTx, txId).Get()
	if err != nil {
		//this is not a significant error. Synthetic transactions don't usually have other synth tx's.
		//TODO: Fixme, this isn't an error
		return nil, err
	}

	return syntheticTxIds, nil
}

//AddSynthTx add the synthetic transaction which is mapped to the parent transaction
func (s *StateDB) AddSynthTx(parentTxId types.Bytes, synthTxId types.Bytes, synthTxObject *Object) {
	s.logInfo("AddSynthTx", "txid", logging.AsHex(synthTxId), "entry", logging.AsHex(synthTxObject.Entry))
	var val *[]transactionStateInfo
	var ok bool

	parentHash := parentTxId.AsBytes32()
	if val, ok = s.transactions.synthTxMap[parentHash]; !ok {
		val = new([]transactionStateInfo)
		s.transactions.synthTxMap[parentHash] = val
	}
	*val = append(*val, transactionStateInfo{synthTxObject, nil, synthTxId})
}

// AddTransaction queues (pending) transaction signatures and (optionally) an
// accepted transaction for storage to their respective chains.
func (s *StateDB) AddTransaction(chainId *types.Bytes32, txId types.Bytes, txPending, txAccepted *Object) error {
	var txAcceptedEntry []byte
	if txAccepted != nil {
		txAcceptedEntry = txAccepted.Entry
	}
	s.logInfo("AddTransaction", "chainId", logging.AsHex(chainId), "txid", logging.AsHex(txId), "pending", logging.AsHex(txPending.Entry), "accepted", logging.AsHex(txAcceptedEntry))

	chainType, _ := binary.Uvarint(txPending.Entry)
	if types.ChainType(chainType) != types.ChainTypePendingTransaction {
		return fmt.Errorf("expecting pending transaction chain type of %s, but received %s",
			types.ChainTypePendingTransaction.Name(), types.TxType(chainType).Name())
	}

	if txAccepted != nil {
		chainType, _ = binary.Uvarint(txAccepted.Entry)
		if types.ChainType(chainType) != types.ChainTypeTransaction {
			return fmt.Errorf("expecting pending transaction chain type of %s, but received %s",
				types.ChainTypeTransaction.Name(), types.ChainType(chainType).Name())
		}
	}

	//append the list of pending Tx's, txId's, and validated Tx's.
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tsi := transactionStateInfo{txPending, chainId.Bytes(), txId}
	s.transactions.pendingTx = append(s.transactions.pendingTx, &tsi)

	if txAccepted != nil {
		tsi := transactionStateInfo{txAccepted, chainId.Bytes(), txId}
		s.transactions.validatedTx = append(s.transactions.validatedTx, &tsi)
	}
	return nil
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (s *StateDB) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	_ = verify
	s.Sync()

	if s.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data, err := s.db.Key("StateEntries", chainId).Get()
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no state defined for %X", storage.ErrNotFound, chainId)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get state entry %X: %v", chainId, err)
	}

	ret := &Object{}
	err = ret.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal state for %x", chainId)
	}
	//if verify {
	//todo: generate and verify data to make sure the state matches what is in the patricia trie
	//}
	return ret, nil
}

// GetTransaction loads the state of the given transaction.
func (s *StateDB) GetTransaction(txid []byte) (*Object, error) {
	s.Sync()

	if s.db == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data, err := s.db.Key(bucketTx, txid).Get()
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no transaction defined for %X", storage.ErrNotFound, txid)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %X: %v", txid, err)
	}

	ret := &Object{}
	err = ret.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal state for %x", txid)
	}

	return ret, nil
}

// GetCurrentEntry retrieves the current state object from the database based upon chainId.  Current state either comes
// from a previously saves state for the current block, or it is from the database
func (s *StateDB) GetCurrentEntry(chainId []byte) (*Object, error) {
	if chainId == nil {
		return nil, fmt.Errorf("chain id is invalid, thus unable to retrieve current entry")
	}
	var ret *Object
	var err error
	var key types.Bytes32

	copy(key[:32], chainId[:32])

	s.mutex.Lock()
	currentState := s.updates[key]
	s.mutex.Unlock()
	if currentState != nil {
		ret = currentState.stateData
	} else {
		currentState := blockUpdates{}
		currentState.bucket = bucketEntry
		//pull current state entry from the database.
		currentState.stateData, err = s.GetPersistentEntry(chainId, false)
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
func (s *StateDB) AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *Object) {
	s.logInfo("AddStateEntry", "chainId", logging.AsHex(chainId), "txHash", logging.AsHex(txHash), "entry", logging.AsHex(object.Entry))
	begin := time.Now()

	s.TimeBucket = s.TimeBucket + float64(time.Since(begin))*float64(time.Nanosecond)*1e-9

	s.mutex.Lock()
	updates := s.updates[*chainId]
	s.mutex.Unlock()

	if updates == nil {
		updates = new(blockUpdates)
		s.updates[*chainId] = updates
	}

	updates.txId = append(updates.txId, txHash)
	updates.stateData = object
}

func (s *StateDB) writeTxs(mutex *sync.Mutex, group *sync.WaitGroup) error {
	defer group.Done()
	//record transactions
	for _, tx := range s.transactions.validatedTx {
		data, _ := tx.Object.MarshalBinary()
		//store the transaction

		txHash := tx.TxId.AsBytes32()
		if synthTxInfos, ok := s.transactions.synthTxMap[txHash]; ok {
			var synthData []byte
			for _, synthTxInfo := range *synthTxInfos {
				synthData = append(synthData, synthTxInfo.TxId...)
				synthTxData, err := synthTxInfo.Object.MarshalBinary()
				if err != nil {
					return err
				}

				s.db.Key(bucketStagedSynthTx, "", synthTxInfo.TxId).PutBatch(synthTxData)

				//store the hash of th synthObject in the bpt, will be removed after synth tx is processed
				s.bpt.Bpt.Insert(synthTxInfo.TxId.AsBytes32(), sha256.Sum256(synthTxData))
			}
			//store a list of txid to list of synth txid's
			s.db.Key(bucketTxToSynthTx, tx.TxId).PutBatch(synthData)
		}

		mutex.Lock()
		//store the transaction in the transaction bucket by txid
		s.db.Key(bucketTx, tx.TxId).PutBatch(data)
		//insert the hash of the tx object in the BPT
		s.bpt.Bpt.Insert(txHash, sha256.Sum256(data))
		mutex.Unlock()
	}

	// record pending transactions
	for _, tx := range s.transactions.pendingTx {
		//marshal the pending transaction state
		data, _ := tx.Object.MarshalBinary()
		//hash it and add to the merkle state for the pending chain
		pendingHash := sha256.Sum256(data)

		mutex.Lock()
		//Store the mapping of the Transaction hash to the pending transaction hash which can be used for
		// validation so we can find the pending transaction
		s.db.Key("MainToPending", tx.TxId).PutBatch(pendingHash[:])

		//store the pending transaction by the pending tx hash
		s.db.Key(bucketPendingTx, pendingHash[:]).PutBatch(data)
		mutex.Unlock()
	}

	//clear out the transactions after they have been processed
	s.transactions.validatedTx = nil
	s.transactions.pendingTx = nil
	s.transactions.synthTxMap = make(map[types.Bytes32]*[]transactionStateInfo)
	return nil
}

func (s *StateDB) writeChainState(group *sync.WaitGroup, mutex *sync.Mutex, mm *managed.MerkleManager, chainId types.Bytes32) error {
	defer group.Done()

	err := s.mm.SetChainID(chainId[:])
	if err != nil {
		return err
	}

	// We get ChainState objects here, instead. And THAT will hold
	//       the MerkleStateManager for the chain.
	//mutex.Lock()
	currentState := s.updates[chainId]
	//mutex.Unlock()

	if currentState == nil {
		panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
	}

	//add all the transaction states that occurred during this block for this chain (in order of appearance)
	for _, tx := range currentState.txId {
		//store the txHash for the chains, they will be mapped back to the above recorded tx's
		s.logInfo("AddHash", "hash", logging.AsHex(tx))
		mm.AddHash(managed.Hash((*tx)[:]))
	}

	if currentState.stateData != nil {
		//store the MD root for the state
		mdRoot := mm.MS.GetMDRoot()
		if mdRoot == nil {
			//shouldn't get here, but will reject if I do
			panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		}

		//store the state of the main chain in the state object
		currentState.stateData.MDRoot = mdRoot.Bytes32()

		//now store the state object
		chainStateObject, err := currentState.stateData.MarshalBinary()
		if err != nil {
			panic("failed to marshal binary for state data")
		}

		mutex.Lock()
		s.GetDB().Key(bucketEntry, chainId.Bytes()).PutBatch(chainStateObject)
		// The bpt stores the hash of the ChainState object hash.
		s.bpt.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
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
	//	s.bpt.Bpt.Insert(chainId, *mdRoot)
	//}

	return nil
}

func (s *StateDB) writeAnchors(mutex *sync.Mutex, blockIndex int64, timestamp time.Time, chainsThatUpdated []types.Bytes32) error {
	// Load the previous anchor chain head
	prevHead, err := s.getAnchorHead()
	if errors.Is(err, storage.ErrNotFound) {
		prevHead = &AnchorMetadata{Index: -1}
	} else if err != nil {
		return err
	}

	// Make sure the block index is increasing
	if prevHead.Index >= blockIndex {
		panic(fmt.Errorf("Current height is %d but the next block height is %d!", prevHead.Index, blockIndex))
	}

	// Metadata
	head := new(AnchorMetadata)
	head.Index = blockIndex
	head.PreviousHeight = s.mm.MS.Count
	head.Timestamp = timestamp
	head.Chains = make([][32]byte, len(chainsThatUpdated))

	// Add an anchor for each updated chain to the anchor chain
	for i, chainId := range chainsThatUpdated {
		head.Chains[i] = chainId

		err := s.mm.SetChainID(chainId[:])
		if err != nil {
			return err
		}
		root := s.mm.MS.GetMDRoot()

		err = s.mm.SetChainID([]byte(bucketMinorAnchorChain))
		if err != nil {
			return err
		}
		s.mm.AddHash(root)
	}

	data, err := head.MarshalBinary()
	if err != nil {
		return err
	}

	// Add the anchor head to the anchor chain
	s.mm.AddHash(data)

	// Index the anchor chain against the block index
	s.GetDB().Key(bucketMinorAnchorChain, "Index", blockIndex).PutBatch(common.Int64Bytes(s.mm.MS.Count))

	// Update the Patricia tree
	var id [32]byte
	copy(id[:], []byte(bucketMinorAnchorChain.String()))
	s.bpt.Bpt.Insert(id, sha256.Sum256(data))
	return nil
}

func (s *StateDB) writeBatches() {
	defer s.sync.Done()
	s.db.EndBatch()
	s.bpt.DBManager.EndBatch()
}

func (s *StateDB) getAnchorHead() (*AnchorMetadata, error) {
	err := s.mm.SetChainID([]byte(bucketMinorAnchorChain))
	if err != nil {
		return nil, err
	}

	data, err := s.mm.Get(s.mm.MS.Count - 1)
	if err != nil {
		return nil, err
	}

	head := new(AnchorMetadata)
	err = head.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return head, nil
}

func (s *StateDB) BlockIndex() (int64, error) {
	head, err := s.getAnchorHead()
	if err != nil {
		return 0, err
	}

	return head.Index, nil
}

// WriteStates will push the data to the database and update the patricia trie
func (s *StateDB) WriteStates(blockHeight int64, timestamp time.Time) ([]byte, error) {
	if len(s.updates) == 0 {
		// Do not write anything if there are no updates
		return s.bpt.Bpt.Root.Hash[:], nil
	}

	group := new(sync.WaitGroup)
	group.Add(1)
	group.Add(len(s.updates))

	mutex := new(sync.Mutex)
	//to try the multi-threading add "go" in front of the next line
	err := s.writeTxs(mutex, group)
	if err != nil {
		return s.bpt.Bpt.Root.Hash[:], nil
	}

	// Create an ordered list of chain IDs that need updating. The iteration
	// order of maps in Go is random. Randomly ordering database writes is bad,
	// because that leads to consensus errors between nodes, since each node
	// will have a different random order. So we need updates to have some
	// consistent order, regardless of what it is.
	updateOrder := make([]types.Bytes32, 0, len(s.updates))
	for id := range s.updates {
		updateOrder = append(updateOrder, id)
	}
	sort.Slice(updateOrder, func(i, j int) bool {
		return bytes.Compare(updateOrder[i][:], updateOrder[j][:]) < 0
	})

	for _, chainId := range updateOrder {
		err = s.writeChainState(group, mutex, s.mm, chainId)
		if err != nil {
			return nil, err
		}

		//TODO: figure out how to do this with new way state is derived
		//if len(currentState.pendingTx) != 0 {
		//	mdRoot := v.PendingChain.MS.GetMDRoot()
		//	if mdRoot == nil {
		//		//shouldn't get here, but will reject if I do
		//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		//	}
		//	//todo:  Determine how we purge pending tx's after 2 weeks.
		//	s.bpt.Bpt.Insert(chainId, *mdRoot)
		//}
	}

	// Process pending writes
	writeOrder := make([]storage.Key, 0, len(s.writes))
	for k := range s.writes {
		writeOrder = append(writeOrder, k)
	}
	sort.Slice(writeOrder, func(i, j int) bool {
		return bytes.Compare(writeOrder[i][:], writeOrder[j][:]) < 0
	})
	for _, k := range writeOrder {
		s.GetDB().Key(k).PutBatch(s.writes[k])
	}
	// The compiler optimizes this into a constant-time operation
	for k := range s.writes {
		delete(s.writes, k)
	}

	// Don't write the anchor during the genesis TX
	err = s.writeAnchors(mutex, blockHeight, timestamp, updateOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to make anchor: %v", err)
	}

	group.Wait()

	s.bpt.Bpt.Update()

	//reset out block update buffer to get ready for the next round
	s.sync.Add(1)
	//to enable threaded batch writes, put go in front of next line.
	s.writeBatches()

	s.updates = make(map[types.Bytes32]*blockUpdates)

	//return the state of the BPT for the state of the block
	rh := types.Bytes(s.RootHash()).AsBytes32()
	s.logInfo("WriteStates", "height", blockHeight, "hash", logging.AsHex(rh))
	return s.RootHash(), nil
}

func (s *StateDB) RootHash() []byte {
	h := s.bpt.Bpt.Root.Hash // Make a copy
	return h[:]              // Return a reference to the copy
}

func (s *StateDB) EnsureRootHash() []byte {
	s.bpt.Bpt.EnsureRootHash()
	return s.RootHash()
}
