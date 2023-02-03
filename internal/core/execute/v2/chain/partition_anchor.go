// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from BVN -> DN

type PartitionAnchor struct{}

func (PartitionAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypeBlockValidatorAnchor
}

func (PartitionAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (PartitionAnchor{}).Validate(st, tx)
}

func (PartitionAnchor) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// If a block validator anchor somehow makes it past validation on a BVN, reject it immediately
	if st.NetworkType != protocol.PartitionTypeDirectory {
		return nil, errors.InternalError.With("invalid attempt to process a block validator partition")
	}

	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.BlockValidatorAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.BlockValidatorAnchor), tx.Transaction.Body)
	}

	st.logger.Info("Received anchor", "module", "anchoring", "source", body.Source, "root", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "bpt", logging.AsHex(body.StateTreeAnchor).Slice(0, 4), "block", body.MinorBlockIndex)

	// Verify the origin
	ledger, ok := st.Origin.(*protocol.AnchorLedger)
	if !ok {
		return nil, fmt.Errorf("invalid principal: want %v, got %v", protocol.AccountTypeAnchorLedger, st.Origin.Type())
	}

	// Verify the source URL and get the partition name
	name, ok := protocol.ParsePartitionUrl(body.Source)
	if !ok {
		return nil, fmt.Errorf("invalid source: not a BVN or the DN")
	}

	// Return ACME burnt by buying credits to the supply - but only if the
	// amount is non-zero.
	if !st.Globals.ExecutorVersion.SignatureAnchoringEnabled() || body.AcmeBurnt.Sign() > 0 {
		var issuerState *protocol.TokenIssuer
		err := st.LoadUrlAs(protocol.AcmeUrl(), &issuerState)
		if err != nil {
			return nil, fmt.Errorf("unable to load acme ledger")
		}

		issuerState.Issued.Sub(&issuerState.Issued, &body.AcmeBurnt)
		err = st.Update(issuerState)
		if err != nil {
			return nil, fmt.Errorf("failed to update issuer state: %v", err)
		}
	}

	// Add the anchor to the chain - use the partition name as the chain name
	record := st.batch.Account(st.OriginUrl).AnchorChain(name)
	index, err := st.State.ChainUpdates.AddChainEntry2(st.batch, record.Root(), body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex, false)
	if err != nil {
		return nil, err
	}
	st.State.DidReceiveAnchor(name, body, index)

	// And the BPT root
	_, err = st.State.ChainUpdates.AddChainEntry2(st.batch, record.BPT(), body.StateTreeAnchor[:], 0, 0, false)
	if err != nil {
		return nil, err
	}

	// Did the partition complete a major block?
	if body.MajorBlockIndex > 0 {
		found := -1
		for i, u := range ledger.PendingMajorBlockAnchors {
			if u.Equal(body.Source) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, errors.InternalError.WithFormat("partition %v is not in the pending list", body.Source)
		}
		ledger.PendingMajorBlockAnchors = append(ledger.PendingMajorBlockAnchors[:found], ledger.PendingMajorBlockAnchors[found+1:]...)
		err = st.Update(ledger)
		if err != nil {
			return nil, err
		}

		// If every partition has done the major block thing, do the major block
		// thing on the DN
		if len(ledger.PendingMajorBlockAnchors) == 0 {
			st.logger.Info("Completed major block", "major-index", ledger.MajorBlockIndex, "minor-index", body.MinorBlockIndex)
			st.State.MakeMajorBlock = ledger.MajorBlockIndex
			st.State.MakeMajorBlockTime = ledger.MajorBlockTime
		}
		return nil, nil
	}

	// Process pending synthetic transactions sent to the DN
	var txids []*url.TxID
	var sequence = map[*url.TxID]int{}
	synth, err := st.batch.Account(st.Ledger()).GetSyntheticForAnchor(body.RootChainAnchor)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synth txns for anchor %x: %w", body.RootChainAnchor[:8], err)
	}
	for _, txid := range synth {
		h := txid.Hash()
		s, err := st.batch.Transaction(h[:]).Main().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}
		if s.Transaction == nil {
			return nil, errors.InternalError.WithFormat("synthetic transaction %v is not a transaction", txid)
		}

		sequence[txid] = int(s.Transaction.Header.SequenceNumber)
		txids = append(txids, txid)
	}

	// Submit the transactions, sorted
	sort.Slice(txids, func(i, j int) bool {
		return sequence[txids[i]] < sequence[txids[j]]
	})
	for _, id := range txids {
		st.State.ProcessSynthetic(id)
	}

	return nil, nil
}
