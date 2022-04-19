package block

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

type Executor struct {
	ExecutorOptions

	executors map[protocol.TransactionType]TransactionExecutor
	governor  *governor
	logger    logging.OptionalLogger

	// oldBlockMeta blockMetadata
}

type ExecutorOptions struct {
	Logger  log.Logger
	Key     ed25519.PrivateKey
	Router  routing.Router
	Network config.Network

	isGenesis bool
}

func newExecutor(opts ExecutorOptions, db *database.Database, executors ...TransactionExecutor) (*Executor, error) {
	m := new(Executor)
	m.ExecutorOptions = opts
	m.executors = map[protocol.TransactionType]TransactionExecutor{}

	if opts.Logger != nil {
		m.logger.L = opts.Logger.With("module", "executor")
	}

	if !m.isGenesis {
		m.governor = newGovernor(opts, db)
	}

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(fmt.Errorf("duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x
	}

	batch := db.Begin(false)
	defer batch.Discard()

	var height int64
	var ledger *protocol.InternalLedger
	err := batch.Account(m.Network.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		height = ledger.Index
	case errors.Is(err, storage.ErrNotFound):
		height = 0
	default:
		return nil, err
	}

	anchor, err := batch.GetMinorRootChainAnchor(&m.Network)
	if err != nil {
		return nil, err
	}

	m.logInfo("Loaded", "height", height, "hash", logging.AsHex(anchor))
	return m, nil
}

// PingGovernor_TESTONLY pings the governor. If runDidCommit is running, this
// will block until runDidCommit completes.
func (m *Executor) PingGovernor_TESTONLY() {
	select {
	case m.governor.messages <- govPing{}:
	case <-m.governor.done:
	}
}

func (m *Executor) logDebug(msg string, keyVals ...interface{}) {
	m.logger.Debug(msg, keyVals...)
}

func (m *Executor) logInfo(msg string, keyVals ...interface{}) {
	m.logger.Info(msg, keyVals...)
}

func (m *Executor) logError(msg string, keyVals ...interface{}) {
	m.logger.Error(msg, keyVals...)
}

func (m *Executor) Start() error {
	return m.governor.Start()
}

func (m *Executor) Stop() error {
	return m.governor.Stop()
}

func (m *Executor) Genesis(block *Block, callback func(st *chain.StateManager) error) error {
	var err error

	if !m.isGenesis {
		panic("Cannot call Genesis on a node txn executor")
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AcmeUrl()
	txn.Body = new(protocol.InternalGenesis)

	st := chain.NewStateManager(block.Batch.Begin(true), m.Network.NodeUrl(), m.Network.NodeUrl(), nil, nil, txn, m.logger.With("operation", "Genesis"))
	defer st.Discard()

	err = putSyntheticTransaction(
		block.Batch, txn,
		&protocol.TransactionStatus{Delivered: true},
		&protocol.InternalSignature{Network: m.Network.NodeUrl()})
	if err != nil {
		return err
	}

	err = indexing.BlockState(block.Batch, m.Network.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return err
	}

	err = callback(st)
	if err != nil {
		return err
	}

	state, err := st.Commit()
	if err != nil {
		return err
	}

	block.State.MergeTransaction(state)

	err = m.ProduceSynthetic(block.Batch, txn, state.ProducedTxns)
	if err != nil {
		return protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}

	err = m.EndBlock(block)
	if err != nil {
		return protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}

	return nil
}

func (m *Executor) InitChain(block *Block, data []byte) ([]byte, error) {
	if m.isGenesis {
		panic("Cannot call InitChain on a genesis txn executor")
	}

	// Check if InitChain already happened
	var anchor []byte
	var err error
	err = block.Batch.View(func(batch *database.Batch) error {
		anchor, err = batch.GetMinorRootChainAnchor(&m.Network)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(anchor) > 0 {
		return anchor, nil
	}

	// Load the genesis state (JSON) into an in-memory key-value store
	src := memory.New(nil)
	err = src.UnmarshalJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal app state: %v", err)
	}

	// Load the root anchor chain so we can verify the system state
	srcBatch := database.New(src, nil).Begin(false)
	defer srcBatch.Discard()
	srcAnchor, err := srcBatch.GetMinorRootChainAnchor(&m.Network)
	if err != nil {
		return nil, fmt.Errorf("failed to load root anchor chain from app state: %v", err)
	}

	// Dump the genesis state into the key-value store
	batch := block.Batch.Begin(true)
	defer batch.Discard()
	err = batch.Import(src)
	if err != nil {
		return nil, fmt.Errorf("failed to import database: %v", err)
	}

	// Commit the database batch
	err = batch.Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to load app state into database: %v", err)
	}

	// Recreate the batch to reload the BPT
	batch = block.Batch.Begin(false)
	defer batch.Discard()

	anchor, err = batch.GetMinorRootChainAnchor(&m.Network)
	if err != nil {
		return nil, err
	}

	// Make sure the database BPT root hash matches what we found in the genesis state
	if !bytes.Equal(srcAnchor, anchor) {
		panic(fmt.Errorf("Root chain anchor from state DB does not match the app state\nWant: %X\nGot:  %X", srcAnchor, anchor))
	}

	return anchor, nil
}

// BeginBlock implements ./Chain
func (m *Executor) BeginBlock(block *Block) (err error) {
	m.logDebug("Begin block", "height", block.Index, "leader", block.IsLeader, "time", block.Time)

	// Reset the block state
	err = indexing.BlockState(block.Batch, m.Network.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return err
	}

	// Load the ledger state
	ledger := block.Batch.Account(m.Network.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.InternalLedger
	err = ledger.GetStateAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if ledgerState.Index >= block.Index {
			panic(fmt.Errorf("Current height is %d but the next block height is %d!", ledgerState.Index, block.Index))
		}

	case m.isGenesis && errors.Is(err, storage.ErrNotFound):
		// OK

	default:
		return fmt.Errorf("cannot load ledger: %w", err)
	}

	// Reset transient values
	ledgerState.Index = block.Index
	ledgerState.Timestamp = block.Time

	err = ledger.PutState(ledgerState)
	if err != nil {
		return fmt.Errorf("cannot write ledger: %w", err)
	}

	//store votes from previous block, choosing to marshal as json to make it easily viewable by explorers
	data, err := json.Marshal(block.CommitInfo)
	if err != nil {
		m.logger.Error("cannot marshal voting info data as json")
	} else {
		wd := protocol.WriteData{}
		wd.Entry.Data = append(wd.Entry.Data, data)

		err := m.processInternalDataTransaction(block, protocol.Votes, &wd)
		if err != nil {
			m.logger.Error(fmt.Sprintf("error processing internal vote transaction, %v", err))
		}
	}

	//capture evidence of maleficence if any occurred
	if block.Evidence != nil {
		data, err := json.Marshal(block.Evidence)
		if err != nil {
			m.logger.Error("cannot marshal evidence as json")
		} else {
			wd := protocol.WriteData{}
			wd.Entry.Data = append(wd.Entry.Data, data)

			err := m.processInternalDataTransaction(block, protocol.Evidence, &wd)
			if err != nil {
				m.logger.Error(fmt.Sprintf("error processing internal evidence transaction, %v", err))
			}
		}
	}

	return nil
}

func (m *Executor) processInternalDataTransaction(block *Block, internalAccountPath string, wd *protocol.WriteData) error {
	dataAccountUrl := m.Network.NodeUrl(internalAccountPath)

	if wd == nil {
		return fmt.Errorf("no internal data transaction provided")
	}

	var signer protocol.Signer
	signerUrl := m.Network.ValidatorPage(0)
	err := block.Batch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return err
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = m.Network.NodeUrl()
	txn.Body = wd
	txn.Header.Initiator = signerUrl.AccountID32()

	sw := protocol.SegWitDataEntry{}
	sw.Cause = *(*[32]byte)(txn.GetHash())
	sw.EntryHash = *(*[32]byte)(wd.Entry.Hash())
	sw.EntryUrl = txn.Header.Principal
	txn.Body = &sw

	st := chain.NewStateManager(block.Batch.Begin(true), m.Network.NodeUrl(), signerUrl, signer, nil, txn, m.logger)
	defer st.Discard()

	var da *protocol.DataAccount
	va := block.Batch.Account(dataAccountUrl)
	err = va.GetStateAs(&da)
	if err != nil {
		return err
	}

	st.UpdateData(da, wd.Entry.Hash(), &wd.Entry)

	err = putSyntheticTransaction(
		block.Batch, txn,
		&protocol.TransactionStatus{Delivered: true},
		&protocol.InternalSignature{Network: signerUrl})
	if err != nil {
		return err
	}

	_, err = st.Commit()
	return err
}

// EndBlock implements ./Chain
func (m *Executor) EndBlock(block *Block) error {
	// Load the ledger
	ledger := block.Batch.Account(m.Network.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.InternalLedger
	err := ledger.GetStateAs(&ledgerState)
	if err != nil {
		return err
	}

	//set active oracle from pending
	ledgerState.ActiveOracle = ledgerState.PendingOracle

	m.logInfo("Committing",
		"height", block.Index,
		"delivered", block.State.Delivered,
		"signed", block.State.SynthSigned,
		"sent", block.State.SynthSent,
		"updated", len(block.State.ChainUpdates.Entries),
		"submitted", len(block.State.ProducedTxns))
	t := time.Now()

	err = m.doEndBlock(block, ledgerState)
	if err != nil {
		return err
	}

	// Write the updated ledger
	err = ledger.PutState(ledgerState)
	if err != nil {
		return err
	}

	m.logInfo("Committed", "height", block.Index, "duration", time.Since(t))
	return nil
}

// DidCommit implements ./Chain
func (m *Executor) DidCommit(block *Block, batch *database.Batch) error {
	return m.governor.DidCommit(batch, false, block)
}

// updateOraclePrice reads the oracle from the oracle account and updates the
// value on the ledger.
func (m *Executor) updateOraclePrice(block *Block, ledgerState *protocol.InternalLedger) error {
	data, err := block.Batch.Account(protocol.PriceOracle()).Data()
	if err != nil {
		return fmt.Errorf("cannot retrieve oracle data entry: %v", err)
	}
	_, e, err := data.GetLatest()
	if err != nil {
		return fmt.Errorf("cannot retrieve latest oracle data entry: data batch at height %d: %v", data.Height(), err)
	}

	o := protocol.AcmeOracle{}
	if e.Data == nil {
		return fmt.Errorf("no data in oracle data account")
	}
	err = json.Unmarshal(e.Data[0], &o)
	if err != nil {
		return fmt.Errorf("cannot unmarshal oracle data entry %x", e.Data)
	}

	if o.Price == 0 {
		return fmt.Errorf("invalid oracle price, must be > 0")
	}

	ledgerState.PendingOracle = o.Price
	return nil
}

func (m *Executor) doEndBlock(block *Block, ledgerState *protocol.InternalLedger) error {
	// Load the main chain of the minor root
	ledgerUrl := m.Network.NodeUrl(protocol.Ledger)
	ledger := block.Batch.Account(ledgerUrl)
	rootChain, err := ledger.Chain(protocol.MinorRootChain, protocol.ChainTypeAnchor)
	if err != nil {
		return err
	}

	// Pending transaction-chain index entries
	type txChainIndexEntry struct {
		indexing.TransactionChainEntry
		Txid []byte
	}
	txChainEntries := make([]*txChainIndexEntry, 0, len(block.State.ChainUpdates.Entries))

	// Process chain updates
	accountSeen := map[string]bool{}
	for _, u := range block.State.ChainUpdates.Entries {
		// Do not create root chain or BPT entries for the ledger
		if ledgerUrl.Equal(u.Account) {
			continue
		}

		// Anchor and index the chain
		m.logDebug("Updated a chain", "url", fmt.Sprintf("%s#chain/%s", u.Account, u.Name))
		account := block.Batch.Account(u.Account)
		indexIndex, didIndex, err := addChainAnchor(rootChain, account, u.Account, u.Name, u.Type)
		if err != nil {
			return err
		}

		// Once for each account
		s := strings.ToLower(u.Account.String())
		if !accountSeen[s] {
			accountSeen[s] = true
			err = account.PutBpt()
			if err != nil {
				return err
			}
		}

		// Add a pending transaction-chain index update
		if didIndex && u.Type == protocol.ChainTypeTransaction {
			e := new(txChainIndexEntry)
			e.Txid = u.Entry
			e.Account = u.Account
			e.Chain = u.Name
			e.ChainIndex = indexIndex
			txChainEntries = append(txChainEntries, e)
		}
	}

	// Create a BlockChainUpdates Index
	err = indexing.BlockChainUpdates(block.Batch, &m.Network, uint64(block.Index)).Set(block.State.ChainUpdates.Entries)
	if err != nil {
		return err
	}

	// If dn/oracle was updated, update the ledger's oracle value, but only if
	// we're on the DN - mirroring can cause dn/oracle to be updated on the BVN
	if accountSeen[protocol.PriceOracleAuthority] && m.Network.LocalSubnetID == protocol.Directory {
		// If things go south here, don't return and error, instead, just log one
		err := m.updateOraclePrice(block, ledgerState)
		if err != nil {
			m.logError(fmt.Sprintf("%v", err))
		}
	}

	// Add the synthetic transaction chain to the root chain
	var synthIndexIndex uint64
	var synthAnchorIndex uint64
	if len(block.State.ProducedTxns) > 0 {
		synthAnchorIndex = uint64(rootChain.Height())
		synthIndexIndex, err = m.anchorSynthChain(block, ledger, ledgerUrl, ledgerState, rootChain)
		if err != nil {
			return err
		}
	}

	// Add the BPT to the root chain
	err = m.anchorBPT(block, ledgerState, rootChain)
	if err != nil {
		return err
	}

	// Index the root chain
	rootIndexIndex, err := addIndexChainEntry(ledger, protocol.MinorRootIndexChain, &protocol.IndexEntry{
		Source:     uint64(rootChain.Height() - 1),
		BlockIndex: uint64(block.Index),
		BlockTime:  &block.Time,
	})
	if err != nil {
		return err
	}

	// Update the transaction-chain index
	for _, e := range txChainEntries {
		e.AnchorIndex = rootIndexIndex
		err = indexing.TransactionChain(block.Batch, e.Txid).Add(&e.TransactionChainEntry)
		if err != nil {
			return err
		}
	}

	// Add transaction-chain index entries for synthetic transactions
	blockState, err := indexing.BlockState(block.Batch, ledgerUrl).Get()
	if err != nil {
		return err
	}

	for _, e := range blockState.ProducedSynthTxns {
		err = indexing.TransactionChain(block.Batch, e.Transaction).Add(&indexing.TransactionChainEntry{
			Account:     ledgerUrl,
			Chain:       protocol.SyntheticChain,
			ChainIndex:  synthIndexIndex,
			AnchorIndex: rootIndexIndex,
		})
		if err != nil {
			return err
		}
	}

	// Build synthetic receipts on Directory nodes
	if m.Network.Type == config.Directory {
		err = m.createLocalDNReceipt(block, rootChain, synthAnchorIndex)
		if err != nil {
			return err
		}
	}

	err = m.buildSynthReceipts(block, rootChain.Anchor(), int64(synthIndexIndex), int64(rootIndexIndex))
	if err != nil {
		return err
	}

	return m.buildAnchorTxn(block, ledgerState, rootChain)
}

func (m *Executor) createLocalDNReceipt(block *Block, rootChain *database.Chain, synthAnchorIndex uint64) error {
	rootReceipt, err := rootChain.Receipt(int64(synthAnchorIndex), rootChain.Height()-1)
	if err != nil {
		return err
	}

	synthChain, err := block.Batch.Account(m.Network.Ledger()).ReadChain(protocol.SyntheticChain)
	if err != nil {
		return fmt.Errorf("unable to load synthetic transaction chain: %w", err)
	}

	height := synthChain.Height()
	offset := height - int64(len(block.State.ProducedTxns))
	for i, txn := range block.State.ProducedTxns {
		if txn.Type() == protocol.TransactionTypeSyntheticAnchor || txn.Type() == protocol.TransactionTypeSyntheticMirror {
			// Do not generate a receipt for the anchor
			continue
		}

		synthReceipt, err := synthChain.Receipt(offset+int64(i), height-1)
		if err != nil {
			return err
		}

		receipt, err := synthReceipt.Combine(rootReceipt)
		if err != nil {
			return err
		}

		// This should be the second signature (SyntheticSignature should be first)
		sig := new(protocol.ReceiptSignature)
		sig.SourceNetwork = m.Network.NodeUrl()
		sig.TransactionHash = *(*[32]byte)(txn.GetHash())
		sig.Receipt = *protocol.ReceiptFromManaged(receipt)
		_, err = block.Batch.Transaction(txn.GetHash()).AddSignature(sig)
		if err != nil {
			return err
		}
	}
	return nil
}

// anchorSynthChain anchors the synthetic transaction chain.
func (m *Executor) anchorSynthChain(block *Block, ledger *database.Account, ledgerUrl *url.URL, ledgerState *protocol.InternalLedger, rootChain *database.Chain) (indexIndex uint64, err error) {
	indexIndex, _, err = addChainAnchor(rootChain, ledger, ledgerUrl, protocol.SyntheticChain, protocol.ChainTypeTransaction)
	if err != nil {
		return 0, err
	}

	block.State.ChainUpdates.DidUpdateChain(indexing.ChainUpdate{
		Name:    protocol.SyntheticChain,
		Type:    protocol.ChainTypeTransaction,
		Account: ledgerUrl,
		// Index:   uint64(synthChain.Height() - 1),
	})

	return indexIndex, nil
}

// anchorBPT anchors the BPT after ensuring any pending changes have been flushed.
func (m *Executor) anchorBPT(block *Block, ledgerState *protocol.InternalLedger, rootChain *database.Chain) error {
	root, err := block.Batch.CommitBpt()
	if err != nil {
		return err
	}

	m.logger.Debug("Anchoring BPT", "root", logging.AsHex(root).Slice(0, 4))
	block.State.ChainUpdates.DidUpdateChain(indexing.ChainUpdate{
		Name:    "bpt",
		Account: m.Network.NodeUrl(),
		Index:   uint64(block.Index - 1),
	})

	return rootChain.AddEntry(root, false)
}

// buildSynthReceipts builds partial receipts for produced synthetic
// transactions and stores them in the synthetic transaction ledger.
func (m *Executor) buildSynthReceipts(block *Block, rootAnchor []byte, synthIndexIndex, rootIndexIndex int64) error {
	ledger := block.Batch.Account(m.Network.SyntheticLedger())
	ledgerState := new(protocol.InternalSyntheticLedger)
	err := ledger.GetStateAs(&ledgerState)
	if err != nil {
		return fmt.Errorf("unable to load the synthetic transaction ledger: %w", err)
	}

	synthChain, err := block.Batch.Account(m.Network.Ledger()).ReadChain(protocol.SyntheticChain)
	if err != nil {
		return fmt.Errorf("unable to load synthetic transaction chain: %w", err)
	}

	height := synthChain.Height()
	offset := height - int64(len(block.State.ProducedTxns))
	for i, txn := range block.State.ProducedTxns {
		if txn.Type() == protocol.TransactionTypeSyntheticAnchor || txn.Type() == protocol.TransactionTypeSyntheticMirror {
			// Do not generate a receipt for the anchor
			continue
		}

		entry := new(protocol.SyntheticLedgerEntry)
		entry.TransactionHash = *(*[32]byte)(txn.GetHash())
		entry.RootAnchor = *(*[32]byte)(rootAnchor)
		entry.SynthIndex = uint64(offset) + uint64(i)
		entry.RootIndexIndex = uint64(rootIndexIndex)
		entry.SynthIndexIndex = uint64(synthIndexIndex)
		if m.Network.Type != config.Directory {
			entry.NeedsReceipt = true
		}
		ledgerState.Pending = append(ledgerState.Pending, entry)
		m.logDebug("Adding synthetic transaction to the ledger", "hash", logging.AsHex(txn.GetHash()), "type", txn.Type(), "anchor", logging.AsHex(rootAnchor), "module", "synthetic")
	}

	err = ledger.PutState(ledgerState)
	if err != nil {
		return fmt.Errorf("unable to store the synthetic transaction ledger: %w", err)
	}

	return nil
}

// buildAnchorTxn builds the anchor transaction for the block.
func (m *Executor) buildAnchorTxn(block *Block, ledger *protocol.InternalLedger, rootChain *database.Chain) error {
	txn := new(protocol.SyntheticAnchor)
	txn.Source = m.Network.NodeUrl()
	txn.RootIndex = uint64(rootChain.Height() - 1)
	txn.RootAnchor = *(*[32]byte)(rootChain.Anchor())
	txn.Block = uint64(block.Index)
	txn.AcmeBurnt, ledger.AcmeBurnt = ledger.AcmeBurnt, *big.NewInt(int64(0))
	if m.Network.Type == config.Directory {
		txn.AcmeOraclePrice = ledger.PendingOracle
	}

	// TODO This is pretty inefficient; we're constructing a receipt for every
	// anchor. If we were more intelligent about it, we could send just the
	// Merkle state and a list of transactions, though we would need that for
	// the root chain and each anchor chain.

	anchorUrl := m.Network.NodeUrl(protocol.AnchorPool)
	anchor := block.Batch.Account(anchorUrl)
	for _, update := range block.State.ChainUpdates.Entries {
		// Is it an anchor chain?
		if update.Type != protocol.ChainTypeAnchor {
			continue
		}

		// Does it belong to our anchor pool?
		if !update.Account.Equal(anchorUrl) {
			continue
		}

		indexChain, err := anchor.ReadIndexChain(update.Name, false)
		if err != nil {
			return fmt.Errorf("unable to load minor index chain of intermediate anchor chain %s: %w", update.Name, err)
		}

		from, to, anchorIndex, err := getRangeFromIndexEntry(indexChain, uint64(indexChain.Height())-1)
		if err != nil {
			return fmt.Errorf("unable to load range from minor index chain of intermediate anchor chain %s: %w", update.Name, err)
		}

		rootReceipt, err := rootChain.Receipt(int64(anchorIndex), rootChain.Height()-1)
		if err != nil {
			return fmt.Errorf("unable to build receipt for the root chain: %w", err)
		}

		anchorChain, err := anchor.ReadChain(update.Name)
		if err != nil {
			return fmt.Errorf("unable to load intermediate anchor chain %s: %w", update.Name, err)
		}

		for i := from; i <= to; i++ {
			anchorReceipt, err := anchorChain.Receipt(int64(i), int64(to))
			if err != nil {
				return fmt.Errorf("unable to build receipt for intermediate anchor chain %s: %w", update.Name, err)
			}

			receipt, err := anchorReceipt.Combine(rootReceipt)
			if err != nil {
				return fmt.Errorf("unable to build receipt for intermediate anchor chain %s: %w", update.Name, err)
			}

			r := protocol.ReceiptFromManaged(receipt)
			txn.Receipts = append(txn.Receipts, *r)
			m.logDebug("Build receipt for an anchor", "chain", update.Name, "anchor", logging.AsHex(r.Start), "block", block.Index, "height", i, "module", "synthetic")
		}
	}

	block.Anchor = txn
	return nil
}
