package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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
	st.State.DidReceiveAnchor(body)

	if st.NetworkType == config.Directory {
		st.logger.Info("Received anchor", "module", "anchoring", "source", body.Source, "root", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "bpt", logging.AsHex(body.StateTreeAnchor).Slice(0, 4), "block", body.MinorBlockIndex)
	}

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
	err := st.AddChainEntry(record.Root(), body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex)
	if err != nil {
		return nil, err
	}

	// And the BPT root
	err = st.AddChainEntry(record.BPT(), body.StateTreeAnchor[:], 0, 0)
	if err != nil {
		return nil, err
	}

	// Process updates when present
	if len(body.Updates) > 0 && st.NetworkType != config.Directory {
		err := processNetworkAccountUpdates(st, tx, body.Updates)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func processNetworkAccountUpdates(st *StateManager, delivery *Delivery, updates []protocol.NetworkAccountUpdate) error {
	for _, update := range updates {
		txn := new(protocol.Transaction)
		txn.Body = update.Body

		switch update.Name {
		case protocol.Operators:
			txn.Header.Principal = st.OperatorsPage()
		default:
			txn.Header.Principal = st.NodeUrl(update.Name)
		}

		st.State.ProcessAdditionalTransaction(delivery.NewInternal(txn))
	}
	return nil
}

func getSyntheticSignature(batch *database.Batch, transaction *database.Transaction) (*protocol.PartitionSignature, error) {
	status, err := transaction.GetStatus()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load status: %w", err)
	}

	for _, signer := range status.Signers {
		sigset, err := transaction.ReadSignaturesForSigner(signer)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "load signature set %v: %w", signer.GetUrl(), err)
		}

		for _, entry := range sigset.Entries() {
			state, err := batch.Transaction(entry.SignatureHash[:]).GetState()
			if err != nil {
				return nil, errors.Format(errors.StatusUnknownError, "load signature %x: %w", entry.SignatureHash[:8], err)
			}

			sig, ok := state.Signature.(*protocol.PartitionSignature)
			if ok {
				return sig, nil
			}
		}
	}
	return nil, errors.New(errors.StatusInternalError, "cannot find synthetic signature")
}
