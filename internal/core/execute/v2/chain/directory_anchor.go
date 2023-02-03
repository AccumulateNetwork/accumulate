// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"bytes"
	"fmt"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from DN -> BVN

type DirectoryAnchor struct{}

func (DirectoryAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypeDirectoryAnchor
}

func (DirectoryAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (DirectoryAnchor{}).Validate(st, tx)
}

func (DirectoryAnchor) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.DirectoryAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.DirectoryAnchor), tx.Transaction.Body)
	}

	st.logger.Info("Received anchor", "module", "anchoring", "source", body.Source, "root", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "bpt", logging.AsHex(body.StateTreeAnchor).Slice(0, 4), "block", body.MinorBlockIndex)

	// Verify the origin
	if _, ok := st.Origin.(*protocol.AnchorLedger); !ok {
		return nil, fmt.Errorf("invalid principal: want %v, got %v", protocol.AccountTypeAnchorLedger, st.Origin.Type())
	}

	// Verify the source URL is from the DN
	if !protocol.IsDnUrl(body.Source) {
		return nil, fmt.Errorf("invalid source: not the DN")
	}

	// Trigger a major block?
	if st.NetworkType != config.Directory {
		st.State.MakeMajorBlock = body.MakeMajorBlock
		st.State.MakeMajorBlockTime = body.MakeMajorBlockTime
	}

	// Add the anchor to the chain - use the partition name as the chain name
	record := st.batch.Account(st.OriginUrl).AnchorChain(protocol.Directory)
	index, err := st.State.ChainUpdates.AddChainEntry2(st.batch, record.Root(), body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex, false)
	if err != nil {
		return nil, err
	}
	st.State.DidReceiveAnchor(protocol.Directory, body, index)

	// And the BPT root
	_, err = st.State.ChainUpdates.AddChainEntry2(st.batch, record.BPT(), body.StateTreeAnchor[:], 0, 0, false)
	if err != nil {
		return nil, err
	}

	// Process updates when present
	if len(body.Updates) > 0 && st.NetworkType != config.Directory {
		err := processNetworkAccountUpdates(st, body.Updates)
		if err != nil {
			return nil, err
		}
	}

	if st.NetworkType != config.Directory {
		err = processReceiptsFromDirectory(st, body)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func processReceiptsFromDirectory(st *StateManager, body *protocol.DirectoryAnchor) error {
	var sequence = map[messaging.Message]int{}

	// Process pending transactions from the DN
	messages, err := loadSynthTxns(st, body.RootChainAnchor[:], body.Source, nil, sequence)
	if err != nil {
		return err
	}

	// Process receipts
	for i, receipt := range body.Receipts {
		receipt := receipt // See docs/developer/rangevarref.md
		if !bytes.Equal(receipt.RootChainReceipt.Anchor, body.RootChainAnchor[:]) {
			return fmt.Errorf("receipt %d is invalid: result does not match the anchor", i)
		}

		st.logger.Info("Received receipt", "module", "anchoring", "from", logging.AsHex(receipt.RootChainReceipt.Start).Slice(0, 4), "to", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "block", body.MinorBlockIndex, "source", body.Source)

		msg, err := loadSynthTxns(st, receipt.RootChainReceipt.Start, body.Source, receipt.RootChainReceipt, sequence)
		if err != nil {
			return err
		}
		messages = append(messages, msg...)
	}

	// Submit the receipts, sorted
	sort.Slice(messages, func(i, j int) bool {
		return sequence[messages[i]] < sequence[messages[j]]
	})
	st.State.AdditionalMessages = append(st.State.AdditionalMessages, messages...)
	return nil
}

func loadSynthTxns(st *StateManager, anchor []byte, source *url.URL, receipt *merkle.Receipt, sequence map[messaging.Message]int) ([]messaging.Message, error) {
	synth, err := st.batch.Account(st.Ledger()).GetSyntheticForAnchor(*(*[32]byte)(anchor))
	if err != nil {
		return nil, fmt.Errorf("failed to load pending synthetic transactions for anchor %X: %w", anchor[:4], err)
	}

	var messages []messaging.Message
	for _, txid := range synth {
		h := txid.Hash()
		sig, err := getSyntheticSignature(st.batch, st.batch.Transaction(h[:]))
		if err != nil {
			return nil, err
		}

		var d messaging.Message
		if receipt != nil {
			d = &messaging.UserSignature{
				TransactionHash: txid.Hash(),
				Signature: &protocol.ReceiptSignature{
					SourceNetwork:   source,
					Proof:           *receipt,
					TransactionHash: txid.Hash(),
				},
			}
		} else {
			d = &internal.SyntheticMessage{TxID: txid}
		}
		sequence[d] = int(sig.SequenceNumber)
		messages = append(messages, d)
	}
	return messages, nil
}

func processNetworkAccountUpdates(st *StateManager, updates []protocol.NetworkAccountUpdate) error {
	for _, update := range updates {
		var account *url.URL
		switch update.Name {
		case protocol.Operators:
			account = st.OperatorsPage()
		default:
			account = st.NodeUrl(update.Name)
		}
		st.State.ProcessNetworkUpdate(st.txHash, account, update.Body)
	}
	return nil
}

func getSyntheticSignature(batch *database.Batch, transaction *database.Transaction) (*protocol.PartitionSignature, error) {
	status, err := transaction.GetStatus()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	for _, signer := range status.Signers {
		sigset, err := transaction.ReadSignaturesForSigner(signer)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load signature set %v: %w", signer.GetUrl(), err)
		}

		for _, entry := range sigset.Entries() {
			state, err := batch.Transaction(entry.SignatureHash[:]).GetState()
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load signature %x: %w", entry.SignatureHash[:8], err)
			}

			sig, ok := state.Signature.(*protocol.PartitionSignature)
			if ok {
				return sig, nil
			}
		}
	}
	return nil, errors.InternalError.With("cannot find synthetic signature")
}
