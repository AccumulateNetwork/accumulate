package block

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
