package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKey struct{}

func (UpdateKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKey
}

func (UpdateKey) validate(st *StateManager, tx *protocol.Envelope) (*protocol.UpdateKey, *protocol.KeyPage, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKey)
	if !ok {
		return nil, nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKey), tx.Transaction.Body)
	}
	if len(body.NewKeyHash) != 32 {
		return nil, nil, errors.New("key hash is not a valid length for a SHA-256 hash")
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	return body, page, nil
}

func (UpdateKey) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	_, _, err := UpdateKey{}.validate(st, tx)
	return nil, err
}

func (UpdateKey) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, page, err := UpdateKey{}.validate(st, tx)
	if err != nil {
		return nil, err
	}

	// Do not update the key page version. Do not reset LastUsedOn.

	// Find the first signature
	txObj := st.batch.Transaction(tx.GetTxHash())
	status, err := txObj.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction status: %w", err)
	}

	if status.Initiator == nil {
		// This should be impossible
		return nil, fmt.Errorf("transaction does not have an initiator")
	}

	sigs, err := txObj.ReadSignatures(status.Initiator)
	if err != nil {
		return nil, fmt.Errorf("load signatures for %v: %w", status.Initiator, err)
	}

	if sigs.Count() == 0 {
		// This should never happen, because this transaction is not multisig.
		// But it's possible (maybe) for the initiator to be invalidated by the
		// key page version changing.
		return nil, fmt.Errorf("no valid signatures found for %v", status.Initiator)
	}

	sigHash := sigs.EntryHashes()[0][:]
	sigEnv, err := st.batch.Transaction(sigHash).GetState()
	if err != nil {
		return nil, fmt.Errorf("load first signature from %v: %w", status.Initiator, err)
	}
	if len(sigEnv.Signatures) == 0 {
		// This should be impossible
		return nil, fmt.Errorf("invalid signature state")
	}

	_, entry, ok := page.EntryByKey(sigEnv.Signatures[0].GetPublicKey())
	if !ok {
		return nil, fmt.Errorf("the signing key does not exist on %v", st.OriginUrl)
	}
	keySpec, ok := entry.(*protocol.KeySpec)
	if !ok {
		// This should be impossible
		return nil, errors.New("key page entry is not a key spec")
	}
	keySpec.PublicKeyHash = body.NewKeyHash

	st.Update(page)
	return nil, nil
}
