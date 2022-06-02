package block

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// EndBlock implements ./Chain
func (m *Executor) EndBlock(block *Block) error {
	// Check for missing synthetic transactions. Load the ledger synchronously,
	// request transactions asynchronously.
	var synthLedger *protocol.SyntheticLedger
	err := block.Batch.Account(m.Network.Synthetic()).GetStateAs(&synthLedger)
	if err != nil {
		return err
	}
	go m.requestMissingSyntheticTransactions(synthLedger)

	// Do nothing if the block is empty
	if block.State.Empty() {
		return nil
	}

	// Load the ledger
	ledgerUrl := m.Network.NodeUrl(protocol.Ledger)
	ledger := block.Batch.Account(ledgerUrl)
	var ledgerState *protocol.SystemLedger
	err = ledger.GetStateAs(&ledgerState)
	if err != nil {
		return err
	}

	// Set active oracle from pending
	ledgerState.ActiveOracle = ledgerState.PendingOracle

	m.logger.Debug("Committing",
		"height", block.Index,
		"delivered", block.State.Delivered,
		"signed", block.State.Signed,
		"updated", len(block.State.ChainUpdates.Entries),
		"produced", len(block.State.ProducedTxns))
	t := time.Now()

	// Load the main chain of the minor root
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
		accountSeen[u.Account.String()] = true

		// Do not create root chain or BPT entries for the ledger
		if ledgerUrl.Equal(u.Account) {
			continue
		}

		// Anchor and index the chain
		m.logger.Debug("Updated a chain", "url", fmt.Sprintf("%s#chain/%s", u.Account, u.Name))
		account := block.Batch.Account(u.Account)
		indexIndex, didIndex, err := addChainAnchor(rootChain, account, u.Account, u.Name, u.Type)
		if err != nil {
			return err
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
	if accountSeen[protocol.PriceOracleAuthority] && m.Network.SubnetId == protocol.Directory {
		// If things go south here, don't return and error, instead, just log one
		newOracleValue, err := m.updateOraclePrice(block)
		if err != nil {
			m.logger.Error("Failed to update oracle", "error", err)
		} else {
			ledgerState.PendingOracle = newOracleValue
		}
	}

	// Add the synthetic transaction chain to the root chain
	var synthIndexIndex uint64
	var synthAnchorIndex uint64
	if len(block.State.ProducedTxns) > 0 {
		synthAnchorIndex = uint64(rootChain.Height())
		synthIndexIndex, err = m.anchorSynthChain(block, rootChain)
		if err != nil {
			return err
		}
	}

	// Write the updated ledger
	err = ledger.PutState(ledgerState)
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
			Account:     m.Network.Synthetic(),
			Chain:       protocol.MainChain,
			ChainIndex:  synthIndexIndex,
			AnchorIndex: rootIndexIndex,
		})
		if err != nil {
			return err
		}
	}

	if len(block.State.ProducedTxns) > 0 {
		// Build synthetic receipts on Directory nodes
		if m.Network.NetworkType == config.Directory {
			err = m.createLocalDNReceipt(block, rootChain, synthAnchorIndex)
			if err != nil {
				return err
			}
		}

		// Build synthetic receipts
		err = m.buildSynthReceipt(block.Batch, block.State.ProducedTxns, rootChain.Height()-1, int64(synthAnchorIndex))
		if err != nil {
			return err
		}
	}

	// Update major index chains if it's a major block
	err = m.updateMajorIndexChains(block, rootIndexIndex)
	if err != nil {
		return err
	}

	err = block.Batch.CommitBpt()
	if err != nil {
		return err
	}

	m.logger.Debug("Committed", "height", block.Index, "duration", time.Since(t))
	return nil
}

// updateOraclePrice reads the oracle from the oracle account and updates the
// value on the ledger.
func (m *Executor) updateOraclePrice(block *Block) (uint64, error) {
	e, err := indexing.Data(block.Batch, protocol.PriceOracle()).GetLatestEntry()
	if err != nil {
		return 0, fmt.Errorf("cannot retrieve latest oracle data entry: %v", err)
	}

	o := protocol.AcmeOracle{}
	if e.GetData() == nil {
		return 0, fmt.Errorf("no data in oracle data account")
	}
	err = json.Unmarshal(e.GetData()[0], &o)
	if err != nil {
		return 0, fmt.Errorf("cannot unmarshal oracle data entry %x", e.GetData()[0])
	}

	if o.Price == 0 {
		return 0, fmt.Errorf("invalid oracle price, must be > 0")
	}

	return o.Price, nil
}

func (m *Executor) createLocalDNReceipt(block *Block, rootChain *database.Chain, synthAnchorIndex uint64) error {
	rootReceipt, err := rootChain.Receipt(int64(synthAnchorIndex), rootChain.Height()-1)
	if err != nil {
		return err
	}

	synthChain, err := block.Batch.Account(m.Network.Synthetic()).ReadChain(protocol.MainChain)
	if err != nil {
		return fmt.Errorf("unable to load synthetic transaction chain: %w", err)
	}

	height := synthChain.Height()
	offset := height - int64(len(block.State.ProducedTxns))
	for i, txn := range block.State.ProducedTxns {
		if txn.Body.Type().IsSystem() {
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
		sig.Proof = *protocol.ReceiptFromManaged(receipt)
		_, err = block.Batch.Transaction(txn.GetHash()).AddSignature(0, sig)
		if err != nil {
			return err
		}
	}
	return nil
}

// anchorSynthChain anchors the synthetic transaction chain.
func (m *Executor) anchorSynthChain(block *Block, rootChain *database.Chain) (indexIndex uint64, err error) {
	url := m.Network.Synthetic()
	indexIndex, _, err = addChainAnchor(rootChain, block.Batch.Account(url), url, protocol.MainChain, protocol.ChainTypeTransaction)
	if err != nil {
		return 0, err
	}

	block.State.ChainUpdates.DidUpdateChain(indexing.ChainUpdate{
		Name:    protocol.MainChain,
		Type:    protocol.ChainTypeTransaction,
		Account: url,
	})

	return indexIndex, nil
}

func (x *Executor) requestMissingSyntheticTransactions(ledger *protocol.SyntheticLedger) {
	// Setup
	wg := new(sync.WaitGroup)
	localSubnet := x.Network.NodeUrl()
	dispatcher := newDispatcher(x.ExecutorOptions)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For each subnet
	var sent bool
	for _, subnet := range ledger.Subnets {
		// Get the subnet ID
		id, ok := protocol.ParseSubnetUrl(subnet.Url)
		if !ok {
			// If this happens we're kind of screwed
			panic(errors.Format(errors.StatusInternalError, "synthetic ledger has an invalid subnet URL: %v", subnet.Url))
		}

		// For each pending synthetic transaction
		var batch jsonrpc2.BatchRequest
		for i, hash := range subnet.Pending {
			// If we know the hash we must have a local copy (so we don't need
			// to fetch it)
			if hash != ([32]byte{}) {
				continue
			}

			seqNum := subnet.Delivered + uint64(i) + 1
			x.logger.Info("Missing synthetic transaction", "seq-num", seqNum, "source", subnet.Url)

			// Request the transaction by sequence number
			batch = append(batch, jsonrpc2.Request{
				ID:     i + 1,
				Method: "query-synth",
				Params: &api.SyntheticTransactionRequest{
					Source:         subnet.Url,
					Destination:    localSubnet,
					SequenceNumber: seqNum,
				},
			})
		}

		if len(batch) == 0 {
			continue
		}

		sent = true
		subnet := subnet // See docs/developer/rangevarref.md
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Send the requests
			var resp []*api.TransactionQueryResponse
			err := x.Router.RequestAPIv2(ctx, id, "", batch, &resp)
			if err != nil {
				x.logger.Error("Failed to request synthetic transactions", "error", err, "from", subnet.Url)
				return
			}

			// Broadcast each transaction locally
			for _, resp := range resp {
				// Put the synthetic signature first
				var gotSynth, gotReceipt, gotKey bool
				for i, signature := range resp.Signatures {
					if _, ok := signature.(*protocol.SyntheticSignature); ok && i > 0 {
					}
					switch signature.(type) {
					case *protocol.SyntheticSignature:
						gotSynth = true
						resp.Signatures[0], resp.Signatures[i] = resp.Signatures[i], resp.Signatures[0]
					case *protocol.ReceiptSignature:
						gotReceipt = true
					case *protocol.ED25519Signature:
						gotKey = true
					}
				}

				var missing []string
				if !gotSynth {
					missing = append(missing, "synthetic")
				}
				if !gotReceipt {
					missing = append(missing, "receipt")
				}
				if !gotKey {
					missing = append(missing, "key")
				}
				if len(missing) > 0 {
					err := fmt.Errorf("missing %s signature(s)", strings.Join(missing, ", "))
					x.logger.Error("Invalid synthetic transaction", "error", err, "hash", logging.AsHex(resp.Transaction.GetHash()).Slice(0, 4), "type", resp.Transaction.Body.Type())
					continue
				}

				err = dispatcher.BroadcastTxLocal(ctx, &protocol.Envelope{
					Signatures:  resp.Signatures,
					Transaction: []*protocol.Transaction{resp.Transaction},
				})
				if err != nil {
					x.logger.Error("Failed to dispatch synthetic transaction", "error", err, "from", subnet.Url)
					continue
				}
			}
		}()
	}

	if !sent {
		return
	}

	wg.Wait()
	for err := range dispatcher.Send(ctx) {
		x.checkDispatchError(err, func(err error) {
			x.logger.Error("Failed to dispatch missing synthetic transactions", "error", err)
		})
	}
}

// updateMajorIndexChains updates major index chains.
func (x *Executor) updateMajorIndexChains(block *Block, rootIndexIndex uint64) error {
	if block.State.MakeMajorBlock == 0 {
		return nil
	}

	// Load the chain
	account := block.Batch.Account(x.Network.AnchorPool())
	mainChain, err := account.ReadChain(protocol.MainChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load anchor ledger main chain: %w", err)
	}

	_, err = addIndexChainEntry(account, protocol.IndexChain(protocol.MainChain, true), &protocol.IndexEntry{
		Source:         uint64(mainChain.Height() - 1),
		RootIndexIndex: rootIndexIndex,
		BlockIndex:     block.State.MakeMajorBlock,
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}
