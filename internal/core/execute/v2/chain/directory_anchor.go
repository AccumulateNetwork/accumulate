// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
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

	// Process receipts
	for i, receipt := range body.Receipts {
		receipt := receipt // See docs/developer/rangevarref.md
		if !bytes.Equal(receipt.RootChainReceipt.Anchor, body.RootChainAnchor[:]) {
			return nil, fmt.Errorf("receipt %d is invalid: result does not match the anchor", i)
		}

		st.logger.Info("Received receipt", "module", "anchoring", "from", logging.AsHex(receipt.RootChainReceipt.Start).Slice(0, 4), "to", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "block", body.MinorBlockIndex, "source", body.Source)
	}

	return nil, nil
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
