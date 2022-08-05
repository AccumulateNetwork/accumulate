package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NodeStatusUpdate struct{}

var _ SignerValidator = (*NodeStatusUpdate)(nil)

func (NodeStatusUpdate) Type() protocol.TransactionType {
	return protocol.TransactionTypeNodeStatusUpdate
}

func (NodeStatusUpdate) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error) {
	if !delegate.DescribeNetwork().OperatorsPage().Equal(signer.GetUrl()) {
		return false, errors.Format(errors.StatusUnauthorized, "%v is not authorized to sign NodeStatusUpdates", signer.GetUrl())
	}

	return false, nil
}

func (NodeStatusUpdate) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// Do not require multisig for node status updates
	if len(status.Signers) == 0 {
		return false, false, nil
	}
	return true, false, nil
}

func validateNodeStatusUpdate(st *StateManager, tx *Delivery) (*protocol.NodeStatusUpdate, error) {
	body, ok := tx.Transaction.Body.(*protocol.NodeStatusUpdate)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.NodeStatusUpdate), tx.Transaction.Body)
	}

	if body.Address == nil {
		return nil, errors.Format(errors.StatusBadRequest, "missing address")
	}

	if !protocol.DnUrl().JoinPath(protocol.AddressBook).Equal(st.OriginUrl) {
		return nil, errors.Format(errors.StatusBadRequest, "invalid principal: want address book, got %v", st.OriginUrl)
	}

	return body, nil
}

func (NodeStatusUpdate) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := validateNodeStatusUpdate(st, tx)
	return nil, errors.Wrap(errors.StatusUnknownError, err)
}

func (NodeStatusUpdate) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := validateNodeStatusUpdate(st, tx)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	account, ok := st.Origin.(*protocol.DataAccount)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "address book has wrong type: want %v, got %v", protocol.AccountTypeDataAccount, st.Origin.Type())
	}

	entries := new(protocol.AddressBookEntries)
	err = entries.UnmarshalBinary(account.Entry.GetData()[0])
	if err != nil {
		return nil, errors.Format(errors.StatusInternalError, "unmarshal address book: %w", err)
	}

	signatures, err := st.batch.Transaction(st.txHash[:]).ReadSignatures(st.OperatorsPage())
	if err != nil {
		return nil, errors.Format(errors.StatusInternalError, "read signatures: %w", err)
	}

	if signatures.Count() == 0 {
		// This should never happen, because this transaction is not multisig.
		// But it's possible (maybe) for the initiator to be invalidated by the
		// key page version changing.
		return nil, errors.Format(errors.StatusInternalError, "no valid signatures found")
	}

	for i, entry := range signatures.Entries() {
		sigOrTxn, err := st.batch.Transaction(entry.SignatureHash[:]).GetState()
		if err != nil {
			return nil, errors.Format(errors.StatusInternalError, "load signature %d: %w", i, err)
		}
		if sigOrTxn.Signature == nil {
			// This should be impossible
			return nil, errors.Format(errors.StatusInternalError, "signature entry points to a non-signature")
		}

		signature, ok := sigOrTxn.Signature.(protocol.KeySignature)
		if !ok {
			continue
		}

		switch body.Status {
		case protocol.NodeStatusUp:
			entries.Set(&protocol.AddressBookEntry{
				PublicKeyHash: *(*[32]byte)(signature.GetPublicKeyHash()),
				Address:       body.Address,
			})
		case protocol.NodeStatusDown:
			entries.Remove(signature.GetPublicKeyHash())
		}
	}

	b, err := entries.MarshalBinary()
	if err != nil {
		return nil, errors.Format(errors.StatusInternalError, "marshal address book: %w", err)
	}

	result, err := executeWriteFullDataAccount(st, &protocol.AccumulateDataEntry{Data: [][]byte{b}}, true)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return result, nil
}
