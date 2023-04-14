// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from BVN -> DN

type PartitionAnchor struct{}

func (PartitionAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypeBlockValidatorAnchor
}

func (x PartitionAnchor) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (PartitionAnchor) check(st *StateManager, tx *Delivery) (*protocol.BlockValidatorAnchor, error) {
	// If a block validator anchor somehow makes it past validation on a BVN, reject it immediately
	if st.NetworkType != protocol.PartitionTypeDirectory {
		return nil, errors.InternalError.With("invalid attempt to process a block validator partition")
	}

	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.BlockValidatorAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.BlockValidatorAnchor), tx.Transaction.Body)
	}

	// Verify the source URL and get the partition name
	_, ok = protocol.ParsePartitionUrl(body.Source)
	if !ok {
		return nil, fmt.Errorf("invalid source: not a BVN or the DN")
	}

	return body, nil
}

func (x PartitionAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	st.logger.Info("Received BVN anchor", "module", "anchoring", "source", body.Source, "root", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "bpt", logging.AsHex(body.StateTreeAnchor).Slice(0, 4), "source-block", body.MinorBlockIndex)

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

	return nil, nil
}
