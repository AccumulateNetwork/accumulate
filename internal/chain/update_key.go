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

func (UpdateKey) validate(st *StateManager, tx *Delivery) (*protocol.UpdateKey, *protocol.KeyPage, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKey)
	if !ok {
		return nil, nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKey), tx.Transaction.Body)
	}
	if len(body.NewKeyHash) != 32 {
		return nil, nil, errors.New("key hash is not a valid length for a SHA-256 hash")
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, nil, fmt.Errorf("invalid principal: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	return body, page, nil
}

func (UpdateKey) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, _, err := UpdateKey{}.validate(st, tx)
	return nil, err
}

func (UpdateKey) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, page, err := UpdateKey{}.validate(st, tx)
	if err != nil {
		return nil, err
	}

	// Do not update the key page version. Do not reset LastUsedOn.

	// Find the first signature
	txObj := st.batch.Transaction(tx.Transaction.GetHash())
	status, err := txObj.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction status: %w", err)
	}

	if status.Initiator == nil {
		// This should be impossible
		return nil, fmt.Errorf("transaction does not have an initiator")
	}

	sigs, err := txObj.Signatures(status.Initiator).Get()
	if err != nil {
		return nil, fmt.Errorf("load signatures for %v: %w", status.Initiator, err)
	}

	if len(sigs) == 0 {
		// This should never happen, because this transaction is not multisig.
		// But it's possible (maybe) for the initiator to be invalidated by the
		// key page version changing.
		return nil, fmt.Errorf("no valid signatures found for %v", status.Initiator)
	}

	e := sigs[0]
	sigOrTxn, err := st.batch.Transaction(e.SignatureHash[:]).GetState()
	if err != nil {
		return nil, fmt.Errorf("load first signature from %v: %w", status.Initiator, err)
	}
	if sigOrTxn.Signature == nil {
		// This should be impossible
		return nil, fmt.Errorf("invalid signature state")
	}
	keysig, ok := sigOrTxn.Signature.(protocol.KeySignature)
	if !ok {
		return nil, fmt.Errorf("signature is not a key signature")
	}

	oldPos, entry, found := findKeyPageEntry(page, &protocol.KeySpecParams{KeyHash: keysig.GetPublicKeyHash()})
	if !found {
		return nil, fmt.Errorf("the signing key does not exist on %v", st.OriginUrl)
	}

	// Update the entry
	entry.PublicKeyHash = body.NewKeyHash

	// Relocate the entry
	page.RemoveKeySpecAt(oldPos)
	page.AddKeySpec(entry)

	// Store the update, but do not change the page version
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}
	return nil, nil
}
