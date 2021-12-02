package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
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
	synthTxMap  map[types.Bytes32][]transactionStateInfo
}

// reset will (re)initialize the transaction lists, this should be done on startup and at the end of each block
func (t *transactionLists) reset() {
	t.pendingTx = nil
	t.validatedTx = nil
	t.synthTxMap = make(map[types.Bytes32][]transactionStateInfo)
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
	bucketSynthTxnSigs     = bucket("SyntheticTransactionSignatures")

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
	dbMgr      *database.Manager
	merkleMgr  *managed.MerkleManager
	debug      bool
	bptMgr     *pmt.Manager //pbt is the global patricia trie for the application
	TimeBucket float64
	mutex      sync.Mutex
	sync       sync.WaitGroup
	logger     log.Logger
}

func (s *StateDB) logInfo(msg string, keyVals ...interface{}) {
	if s.logger != nil {
		// TODO Maybe this should be Debug?
		s.logger.Info(msg, keyVals...)
	}
}

func (s *StateDB) init(debug bool) {
	s.debug = debug

	s.bptMgr = pmt.NewBPTManager(s.dbMgr)
	managed.NewMerkleManager(s.dbMgr, markPower)
}

// Open database to manage the smt and chain states
func (s *StateDB) Open(dbFilename string, useMemDB bool, debug bool, logger log.Logger) (err error) {
	if logger != nil {
		s.logger = logger.With("module", "dbMgr")
	}

	dbType := "badger"
	if useMemDB {
		dbType = "memory"
	}

	if logger != nil {
		logger = logger.With("module", dbType)
	}

	s.dbMgr, err = database.NewDBManager(dbType, dbFilename, logger)
	if err != nil {
		return err
	}

	s.merkleMgr, err = managed.NewMerkleManager(s.dbMgr, markPower)
	if err != nil {
		return err
	}

	s.init(debug)
	return nil
}

func (s *StateDB) Load(db storage.KeyValueDB, debug bool) (err error) {
	s.dbMgr = new(database.Manager)
	s.dbMgr.InitWithDB(db)
	s.merkleMgr, err = managed.NewMerkleManager(s.dbMgr, markPower)
	if err != nil {
		return err
	}

	s.init(debug)
	return nil
}

func (s *StateDB) GetDB() *database.Manager {
	return s.dbMgr
}

func (s *StateDB) Sync() {
	s.sync.Wait()
}

//GetChainRange get the transaction id's in a given range
func (s *StateDB) GetChainRange(chainId []byte, start int64, end int64) (resultHashes []types.Bytes32, maxAvailable int64, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	err = s.merkleMgr.SetChainID(chainId)
	if err != nil {
		return nil, 0, err
	}

	if end > s.merkleMgr.MS.Count {
		end = s.merkleMgr.MS.Count
	}

	// GetRange will not cross mark point boundaries, so we may need to call it
	// multiple times
	resultHashes = make([]types.Bytes32, 0, end-start)
	for start < end {
		hashes, err := s.merkleMgr.GetRange(chainId, start, end)
		if err != nil {
			return nil, 0, err
		}

		for i := range hashes {
			resultHashes = append(resultHashes, hashes[i].Bytes32())
		}
		start += int64(len(hashes))
	}

	return resultHashes, s.merkleMgr.MS.Count, nil
}

//GetTx get the transaction by transaction ID
func (s *StateDB) GetTx(txId []byte) (tx []byte, err error) {
	tx, err = s.dbMgr.Key(bucketTx, txId).Get()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

//GetPendingTx get the pending transactions by primary transaction ID
func (s *StateDB) GetPendingTx(txId []byte) (pendingTx []byte, err error) {

	pendingTxId, err := s.dbMgr.Key(bucketMainToPending, txId).Get()
	if err != nil {
		return nil, err
	}
	pendingTx, err = s.dbMgr.Key(bucketPendingTx, pendingTxId).Get()
	if err != nil {
		return nil, err
	}

	return pendingTx, nil
}

// GetSyntheticTxIds get the transaction id list by the transaction ID that spawned the synthetic transactions
func (s *StateDB) GetSyntheticTxIds(txId []byte) (syntheticTxIds []byte, err error) {

	syntheticTxIds, err = s.dbMgr.Key(bucketTxToSynthTx, txId).Get()
	if err != nil {
		//this is not a significant error. Synthetic transactions don't usually have other synth tx's.
		//TODO: Fixme, this isn't an error
		return nil, err
	}

	return syntheticTxIds, nil
}

// AddSynthTx add the synthetic transaction which is mapped to the parent transaction
func (tx *DBTransaction) AddSynthTx(parentTxId types.Bytes, synthTxId types.Bytes, synthTxObject *Object) {
	// TODO: Move the transaction related functions to stateTxn at a time where the impact on other branches is minimal

	tx.state.logInfo("AddSynthTx", "txid", logging.AsHex(synthTxId), "entry", logging.AsHex(synthTxObject.Entry))
	tx.dirty = true

	parentHash := parentTxId.AsBytes32()
	txMap := tx.transactions.synthTxMap
	txMap[parentHash] = append(txMap[parentHash], transactionStateInfo{synthTxObject, nil, synthTxId})
}

// AddTransaction queues (pending) transaction signatures and (optionally) an
// accepted transaction for storage to their respective chains.
func (tx *DBTransaction) AddTransaction(chainId *types.Bytes32, txId types.Bytes, txPending, txAccepted *Object) error {
	var txAcceptedEntry []byte
	if txAccepted != nil {
		txAcceptedEntry = txAccepted.Entry
	}
	tx.state.logInfo("AddTransaction", "chainId", logging.AsHex(chainId), "txid", logging.AsHex(txId), "pending", logging.AsHex(txPending.Entry), "accepted", logging.AsHex(txAcceptedEntry))
	tx.dirty = true

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
	tx.state.mutex.Lock()
	defer tx.state.mutex.Unlock()

	tsi := transactionStateInfo{txPending, chainId.Bytes(), txId}
	tx.transactions.pendingTx = append(tx.transactions.pendingTx, &tsi)

	if txAccepted != nil {
		tsi := transactionStateInfo{txAccepted, chainId.Bytes(), txId}
		tx.transactions.validatedTx = append(tx.transactions.validatedTx, &tsi)
	}
	return nil
}

//GetPersistentEntry will pull the data from the database for the StateEntries bucket.
func (s *StateDB) GetPersistentEntry(chainId []byte, verify bool) (*Object, error) {
	_ = verify
	s.Sync()

	if s.dbMgr == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data, err := s.dbMgr.Key("StateEntries", chainId).Get()
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

func (s *StateDB) getObject(keys ...interface{}) (*Object, error) {
	s.Sync()

	if s.dbMgr == nil {
		return nil, fmt.Errorf("database has not been initialized")
	}

	data, err := s.dbMgr.Key(keys...).Get()
	if err != nil {
		return nil, err
	}

	ret := &Object{}
	err = ret.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %v", err)
	}

	return ret, nil
}

// GetTransaction loads the state of the given transaction.
func (s *StateDB) GetTransaction(txid []byte) (*Object, error) {
	obj, err := s.getObject(bucketTx, txid)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no transaction defined for %X", storage.ErrNotFound, txid)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %X: %v", txid, err)
	}
	return obj, nil
}

// GetSynthTxn loads the state of the given staged synthetic transaction.
func (s *StateDB) GetSynthTxn(txid [32]byte) (*Object, error) {
	obj, err := s.getObject(bucketStagedSynthTx, "", txid)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no transaction defined for %X", storage.ErrNotFound, txid)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %X: %v", txid, err)
	}

	s.logInfo("GetSynthTxn", "txid", logging.AsHex(txid), "entry", logging.AsHex(obj.Entry))
	return obj, nil
}

// GetCurrentEntry retrieves the current state object from the database based upon chainId.  Current state either comes
// from a previously saves state for the current block, or it is from the database
func (tx *DBTransaction) GetCurrentEntry(chainId []byte) (*Object, error) {
	if chainId == nil {
		return nil, fmt.Errorf("chain id is invalid, thus unable to retrieve current entry")
	}
	var ret *Object
	var err error
	var key types.Bytes32

	copy(key[:32], chainId[:32])

	tx.state.mutex.Lock()
	currentState := tx.updates[key]
	tx.state.mutex.Unlock()
	if currentState != nil {
		ret = currentState.stateData
	} else {
		currentState := blockUpdates{}
		currentState.bucket = bucketEntry
		//pull current state entry from the database.
		currentState.stateData, err = tx.GetPersistentEntry(chainId, false)
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
// may change the state of the KeyBook chain (i.e. a sub/secondary chain) based on the effect
// of a transaction.  The entry is the state object associated with
func (tx *DBTransaction) AddStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *Object) {
	tx.state.logInfo("AddStateEntry", "chainId", logging.AsHex(chainId), "txHash", logging.AsHex(txHash), "entry", logging.AsHex(object.Entry))

	if txHash == nil {
		panic("Cannot add state entry without a transaction ID!")
	}

	tx.addStateEntry(chainId, txHash, object)
}

// UpdateNonce updates the state of a signator record _without adding to the
// transaction chain_.
func (tx *DBTransaction) UpdateNonce(chainId *types.Bytes32, object *Object) {
	tx.addStateEntry(chainId, nil, object)
}

func (tx *DBTransaction) addStateEntry(chainId *types.Bytes32, txHash *types.Bytes32, object *Object) {
	tx.dirty = true
	begin := time.Now()

	tx.state.TimeBucket += float64(time.Since(begin)) * float64(time.Nanosecond) * 1e-9

	tx.state.mutex.Lock()
	updates := tx.updates[*chainId]
	tx.state.mutex.Unlock()

	if updates == nil {
		updates = new(blockUpdates)
		tx.updates[*chainId] = updates
	}

	if txHash != nil {
		updates.txId = append(updates.txId, txHash)
	}
	updates.stateData = object
}

func (tx *DBTransaction) writeTxs(mutex *sync.Mutex, group *sync.WaitGroup) error {
	defer group.Done()
	//record transactions
	for _, txn := range tx.transactions.validatedTx {
		data, _ := txn.Object.MarshalBinary()
		//store the transaction

		txHash := txn.TxId.AsBytes32()
		if synthTxInfos, ok := tx.transactions.synthTxMap[txHash]; ok {
			var synthData []byte
			for _, synthTxInfo := range synthTxInfos {
				synthData = append(synthData, synthTxInfo.TxId...)
				synthTxData, err := synthTxInfo.Object.MarshalBinary()
				if err != nil {
					return err
				}

				tx.state.dbMgr.Key(bucketStagedSynthTx, "", synthTxInfo.TxId).PutBatch(synthTxData)

				//store the hash of th synthObject in the bpt, will be removed after synth tx is processed
				tx.state.bptMgr.Bpt.Insert(synthTxInfo.TxId.AsBytes32(), sha256.Sum256(synthTxData))
			}
			//store a list of txid to list of synth txid's
			tx.state.dbMgr.Key(bucketTxToSynthTx, txn.TxId).PutBatch(synthData)
		}

		mutex.Lock()
		//store the transaction in the transaction bucket by txid
		tx.state.dbMgr.Key(bucketTx, txn.TxId).PutBatch(data)
		//insert the hash of the tx object in the BPT
		tx.state.bptMgr.Bpt.Insert(txHash, sha256.Sum256(data))
		mutex.Unlock()
	}

	// record pending transactions
	for _, txn := range tx.transactions.pendingTx {
		//marshal the pending transaction state
		data, _ := txn.Object.MarshalBinary()
		//hash it and add to the merkle state for the pending chain
		pendingHash := sha256.Sum256(data)

		mutex.Lock()
		//Store the mapping of the Transaction hash to the pending transaction hash which can be used for
		// validation so we can find the pending transaction
		tx.state.dbMgr.Key("MainToPending", txn.TxId).PutBatch(pendingHash[:])

		//store the pending transaction by the pending tx hash
		tx.state.dbMgr.Key(bucketPendingTx, pendingHash[:]).PutBatch(data)
		mutex.Unlock()
	}

	return nil
}

func (tx *DBTransaction) writeChainState(group *sync.WaitGroup, mutex *sync.Mutex, mm *managed.MerkleManager, chainId types.Bytes32) error {
	defer group.Done()

	err := tx.state.merkleMgr.SetChainID(chainId[:])
	if err != nil {
		return err
	}

	// We get ChainState objects here, instead. And THAT will hold
	//       the MerkleStateManager for the chain.
	//mutex.Lock()
	currentState := tx.updates[chainId]
	//mutex.Unlock()

	if currentState == nil {
		panic(fmt.Sprintf("Chain state is nil meaning no updates were stored on chain %X for the block. Should not get here!", chainId[:]))
	}

	//add all the transaction states that occurred during this block for this chain (in order of appearance)
	for _, txn := range currentState.txId {
		//store the txHash for the chains, they will be mapped back to the above recorded tx's
		tx.state.logInfo("AddHash", "hash", logging.AsHex(tx))
		mm.AddHash(managed.Hash((*txn)[:]))
	}

	if currentState.stateData != nil {
		//store the state of the main chain in the state object
		count := uint64(mm.MS.Count)
		currentState.stateData.Height = count
		currentState.stateData.Roots = make([][]byte, 64-bits.LeadingZeros64(count))
		for i := range currentState.stateData.Roots {
			if count&(1<<i) == 0 {
				// Only store the hashes we need
				continue
			}
			currentState.stateData.Roots[i] = mm.MS.Pending[i].Copy()
		}

		//now store the state object
		chainStateObject, err := currentState.stateData.MarshalBinary()
		if err != nil {
			panic("failed to marshal binary for state data")
		}

		mutex.Lock()
		tx.GetDB().Key(bucketEntry, chainId.Bytes()).PutBatch(chainStateObject)
		// The bpt stores the hash of the ChainState object hash.
		tx.state.bptMgr.Bpt.Insert(chainId, sha256.Sum256(chainStateObject))
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
	//	s.bptMgr.Bpt.Insert(chainId, *mdRoot)
	//}

	return nil
}

func (s *StateDB) GetSynthTxnSigs() ([]SyntheticSignature, error) {
	b, err := s.GetDB().Key(bucketSynthTxnSigs).Get()
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	sigs := new(SyntheticSignatures)
	err = sigs.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return sigs.Signatures, nil
}

func (tx *DBTransaction) writeSynthTxnSigs() error {
	sigMap := map[[32]byte]*SyntheticSignature{}

	// Get the current set of signatures
	sigs, err := tx.state.GetSynthTxnSigs()
	if err != nil {
		return err
	}
	for _, sig := range sigs {
		sigMap[sig.Txid] = &sig
	}

	// Remove any deleted entries
	for _, txid := range tx.delSynthSigs {
		delete(sigMap, txid)
	}

	// Add new entries
	for _, sig := range tx.addSynthSigs {
		sigMap[sig.Txid] = sig
	}

	// Create a sorted array of the transaction IDs
	txids := make([][32]byte, 0, len(sigMap))
	for txid := range sigMap {
		txids = append(txids, txid)
	}
	sort.Slice(txids, func(i, j int) bool {
		return bytes.Compare(txids[i][:], txids[j][:]) < 0
	})

	// Create a sorted array from the map
	sigs = make([]SyntheticSignature, 0, len(txids))
	for _, sig := range sigMap {
		sigs = append(sigs, *sig)
	}
	sort.Slice(sigs, func(i, j int) bool {
		return bytes.Compare(sigs[i].Txid[:], sigs[j].Txid[:]) < 0
	})

	// Marshal it
	b, err := (&SyntheticSignatures{Signatures: sigs}).MarshalBinary()
	if err != nil {
		return err
	}

	// Store it in SyntheticTransactionSignatures
	tx.GetDB().Key(bucketSynthTxnSigs).PutBatch(b)
	hash := sha256.Sum256(b)

	// Add its hash to the BPT
	var id [32]byte
	copy(id[:], []byte(bucketSynthTxnSigs))
	tx.state.bptMgr.Bpt.Insert(id, hash)
	return nil
}

func (tx *DBTransaction) writeAnchors(blockIndex int64, timestamp time.Time) error {
	// Collect and sort a list of all chain IDs
	chains := make([][32]byte, 0, len(tx.updates))
	for id := range tx.updates {
		chains = append(chains, id)
	}
	sort.Slice(chains, func(i, j int) bool {
		return bytes.Compare(chains[i][:], chains[j][:]) < 0
	})

	// Collect and sort a list of all synthetic transaction IDs
	var txids [][32]byte
	seen := map[[32]byte]bool{}
	for _, txns := range tx.transactions.synthTxMap {
		for _, txn := range txns {
			txid := txn.TxId.AsBytes32()
			if seen[txid] {
				continue
			}
			seen[txid] = true
			txids = append(txids, txid)
		}
	}
	sort.Slice(txids, func(i, j int) bool {
		return bytes.Compare(txids[i][:], txids[j][:]) < 0
	})

	// Load the previous anchor chain head
	prevHead, prevCount, err := tx.state.GetAnchorHead()
	if errors.Is(err, storage.ErrNotFound) {
		prevHead = &AnchorMetadata{Index: 0}
	} else if err != nil {
		return err
	}

	// Make sure the block index is increasing
	if prevHead.Index >= blockIndex {
		panic(fmt.Errorf("Current height is %d but the next block height is %d!", prevHead.Index, blockIndex))
	}

	// Add an anchor for each updated chain to the anchor chain
	for _, chainId := range chains {
		err := tx.state.merkleMgr.SetChainID(chainId[:])
		if err != nil {
			return err
		}
		root := tx.state.merkleMgr.MS.GetMDRoot()

		err = tx.state.merkleMgr.SetChainID([]byte(bucketMinorAnchorChain))
		if err != nil {
			return err
		}
		tx.state.merkleMgr.AddHash(root)
	}

	// Add all of the synth txids
	for _, txid := range txids {
		tx.state.merkleMgr.AddHash(txid[:])
	}

	// Add the anchor head to the anchor chain
	head := new(AnchorMetadata)
	head.Index = blockIndex
	head.PreviousHeight = prevCount
	head.Timestamp = timestamp
	head.Chains = chains
	data, err := head.MarshalBinary()
	if err != nil {
		return err
	}
	tx.state.merkleMgr.AddHash(data)

	// Index the anchor chain against the block index
	tx.GetDB().Key(bucketMinorAnchorChain, "Index", blockIndex).PutBatch(common.Int64Bytes(tx.state.merkleMgr.MS.Count))

	// Update the Patricia tree
	var id [32]byte
	copy(id[:], []byte(bucketMinorAnchorChain))
	tx.state.bptMgr.Bpt.Insert(id, tx.state.merkleMgr.MS.GetMDRoot().Bytes32())
	return nil
}

func (tx *DBTransaction) writeBatches() {
	defer tx.state.sync.Done()
	tx.state.dbMgr.EndBatch()
	tx.state.bptMgr.DBManager.EndBatch()
}

func (s *StateDB) GetAnchorHead() (*AnchorMetadata, int64, error) {
	err := s.merkleMgr.SetChainID([]byte(bucketMinorAnchorChain))
	if err != nil {
		return nil, 0, err
	}

	if s.merkleMgr.MS.Count == 0 {
		return nil, 0, storage.ErrNotFound
	}

	data, err := s.merkleMgr.Get(s.merkleMgr.MS.Count - 1)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read anchor chain element %d", s.merkleMgr.MS.Count-1)
	}

	head := new(AnchorMetadata)
	err = head.UnmarshalBinary(data)
	if err != nil {
		return nil, 0, err
	}

	return head, s.merkleMgr.MS.Count, nil
}

func (s *StateDB) GetAnchors(start, end int64) ([]types.Bytes32, error) {
	h, _, err := s.GetChainRange([]byte(bucketMinorAnchorChain), start, end)
	return h, err
}

func (s *StateDB) SubnetID() (string, error) {
	b, err := s.GetDB().Key("SubnetID").Get()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *StateDB) BlockIndex() (int64, error) {
	head, _, err := s.GetAnchorHead()
	if err != nil {
		return 0, err
	}

	return head.Index, nil
}

// Commit will push the data to the database and update the patricia trie
func (tx *DBTransaction) Commit(blockHeight int64, timestamp time.Time) ([]byte, error) {
	//build a list of keys from the map
	if !tx.dirty {
		//only attempt to record the block if we have any data.
		return tx.RootHash(), nil
	}

	group := new(sync.WaitGroup)
	group.Add(1)
	group.Add(len(tx.updates))

	mutex := new(sync.Mutex)
	//to try the multi-threading add "go" in front of the next line
	err := tx.writeTxs(mutex, group)
	if err != nil {
		return nil, err
	}

	orderedUpdates := orderUpdateList(tx.updates)
	err = tx.commitUpdates(orderedUpdates, err, group, mutex)
	if err != nil {
		return nil, err
	}

	tx.commitTxWrites()

	err = tx.writeSynthTxnSigs()
	if err != nil {
		return nil, fmt.Errorf("failed to save synth txn signatures: %v", err)
	}

	err = tx.writeAnchors(blockHeight, timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to make anchor: %v", err)
	}

	group.Wait()

	tx.state.bptMgr.Bpt.Update()

	//reset out block update buffer to get ready for the next round
	tx.state.sync.Add(1)
	//to enable threaded batch writes, put go in front of next line.
	tx.writeBatches()
	tx.cleanupTx()

	//return the state of the BPT for the state of the block
	rh := types.Bytes(tx.state.RootHash()).AsBytes32()
	tx.state.logInfo("WriteStates", "height", blockHeight, "hash", logging.AsHex(rh))
	return tx.state.RootHash(), nil
}

func orderUpdateList(updateList map[types.Bytes32]*blockUpdates) []types.Bytes32 {
	// Create an ordered list of chain IDs that need updating. The iteration
	// order of maps in Go is random. Randomly ordering database writes is bad,
	// because that leads to consensus errors between nodes, since each node
	// will have a different random order. So we need updates to have some
	// consistent order, regardless of what it is.
	updateOrder := make([]types.Bytes32, 0, len(updateList))
	for id := range updateList {
		updateOrder = append(updateOrder, id)
	}
	sort.Slice(updateOrder, func(i, j int) bool {
		return bytes.Compare(updateOrder[i][:], updateOrder[j][:]) < 0
	})
	return updateOrder
}

func (tx *DBTransaction) commitUpdates(orderedUpdates []types.Bytes32, err error, group *sync.WaitGroup, mutex *sync.Mutex) error {
	for _, chainId := range orderedUpdates {
		err = tx.writeChainState(group, mutex, tx.state.merkleMgr, chainId)
		if err != nil {
			return err
		}

		//TODO: figure out how to do this with new way state is derived
		//if len(currentState.pendingTx) != 0 {
		//	mdRoot := v.PendingChain.MS.GetMDRoot()
		//	if mdRoot == nil {
		//		//shouldn't get here, but will reject if I do
		//		panic(fmt.Sprintf("shouldn't get here on writeState() on chain id %X obtaining merkle state", chainId))
		//	}
		//	//todo:  Determine how we purge pending tx's after 2 weeks.
		//	s.bptMgr.Bpt.Insert(chainId, *mdRoot)
		//}
	}
	return nil
}

func (tx *DBTransaction) commitTxWrites() {
	// Process pending writes
	writeOrder := make([]storage.Key, 0, len(tx.writes))
	for k := range tx.writes {
		writeOrder = append(writeOrder, k)
	}
	sort.Slice(writeOrder, func(i, j int) bool {
		return bytes.Compare(writeOrder[i][:], writeOrder[j][:]) < 0
	})
	for _, k := range writeOrder {
		tx.state.GetDB().Key(k).PutBatch(tx.writes[k])
	}
}

func (s *StateDB) RootHash() []byte {
	h := s.bptMgr.Bpt.Root.Hash // Make a copy
	return h[:]                 // Return a reference to the copy
}

func (tx *DBTransaction) cleanupTx() {
	tx.updates = make(map[types.Bytes32]*blockUpdates)

	// The compiler optimizes this into a constant-time operation
	for k := range tx.writes {
		delete(tx.writes, k)
	}

	//clear out the transactions after they have been processed
	tx.transactions.validatedTx = nil
	tx.transactions.pendingTx = nil
	tx.transactions.synthTxMap = make(map[types.Bytes32][]transactionStateInfo)
}

func (s *StateDB) EnsureRootHash() []byte {
	s.bptMgr.Bpt.EnsureRootHash()
	return s.RootHash()
}
