package chain

import (
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type AddValidator struct{}
type RemoveValidator struct{}
type UpdateValidatorKey struct{}

func (AddValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeAddValidator
}

func (RemoveValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeRemoveValidator
}

func (UpdateValidatorKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateValidatorKey
}

func (AddValidator) Validate(st *StateManager, env *protocol.Envelope) (protocol.TransactionResult, error) {
	body := env.Transaction.Body.(*protocol.AddValidator)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return nil, err
	}

	// Ensure the key does not already exist
	keyHash := sha256.Sum256(body.Key)
	_, _, found := page.EntryByKeyHash(keyHash[:])
	if found {
		return nil, fmt.Errorf("key is already a validator")
	}

	// Add the key hash to the key page
	key := &protocol.KeySpec{PublicKey: keyHash[:]}
	page.Keys = append(page.Keys, key)

	// Record the update
	didUpdateKeyPage(page)
	st.Update(page)

	// Add the validator
	st.AddValidator(body.Key)
	return nil, nil
}

func (RemoveValidator) Validate(st *StateManager, env *protocol.Envelope) (protocol.TransactionResult, error) {
	body := env.Transaction.Body.(*protocol.RemoveValidator)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return nil, err
	}

	// Find the key
	keyHash := sha256.Sum256(body.Key)
	index, _, found := page.EntryByKeyHash(keyHash[:])
	if !found {
		return nil, fmt.Errorf("key is not a validator")
	}

	// Make sure it's not the last key
	if len(page.Keys) == 1 {
		return nil, fmt.Errorf("cannot remove the last validator!")
	}

	// Remove the key hash from the key page
	page.Keys = append(page.Keys[:index], page.Keys[index+1:]...)

	// Update the threshold
	if page.Threshold > uint64(len(page.Keys)) {
		page.Threshold = uint64(len(page.Keys))
	}

	// Record the update
	didUpdateKeyPage(page)
	st.Update(page)

	// Remove the validator
	st.DisableValidator(body.Key)
	return nil, nil
}

func (UpdateValidatorKey) Validate(st *StateManager, env *protocol.Envelope) (protocol.TransactionResult, error) {
	body := env.Transaction.Body.(*protocol.UpdateValidatorKey)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return nil, err
	}

	// Find the old key
	oldKeyHash := sha256.Sum256(body.OldKey)
	_, entry, found := page.EntryByKeyHash(oldKeyHash[:])
	if !found {
		return nil, fmt.Errorf("old key is not a validator")
	}

	// Ensure the new key does not already exist
	newKeyHash := sha256.Sum256(body.NewKey)
	_, _, found = page.EntryByKeyHash(newKeyHash[:])
	if found {
		return nil, fmt.Errorf("new key is already a validator")
	}

	// Update the key hash
	entry.PublicKey = newKeyHash[:]

	// Record the update
	didUpdateKeyPage(page)
	st.Update(page)

	// Update the validator
	st.DisableValidator(body.OldKey)
	st.AddValidator(body.NewKey)
	return nil, nil
}

// checkValidatorTransaction implements common checks for validator
// transactions.
func checkValidatorTransaction(st *StateManager, env *protocol.Envelope) (*protocol.KeyPage, error) {
	if !st.nodeUrl.Equal(env.Transaction.Header.Principal) {
		return nil, fmt.Errorf("invalid origin: must be %s, got %s", st.nodeUrl, env.Transaction.Header.Principal)
	}

	bookUrl := st.nodeUrl.JoinPath(protocol.ValidatorBook)
	pageUrl := protocol.FormatKeyPageUrl(bookUrl, 0)
	var page *protocol.KeyPage
	err := st.LoadUrlAs(pageUrl, &page)
	if err != nil {
		return nil, fmt.Errorf("unable to load %s: %v", pageUrl, err)
	}

	signerPriority, ok := getKeyPageIndex(st.SignatorUrl)
	if !ok {
		return nil, fmt.Errorf("cannot parse key page URL: %v", st.SignatorUrl)
	}

	if signerPriority > 0 {
		return nil, fmt.Errorf("cannot modify %v with a lower priority key page", pageUrl)
	}

	return page, nil
}
