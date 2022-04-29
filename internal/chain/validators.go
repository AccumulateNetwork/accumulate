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

func (AddValidator) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return nil, addValidator(st, tx, true)
}

func (RemoveValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeRemoveValidator
}

func (RemoveValidator) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return nil, removeValidator(st, tx, true)
}

func (UpdateValidatorKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateValidatorKey
}

func (UpdateValidatorKey) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (UpdateValidatorKey{}).Validate(st, tx)
}

func (AddValidator) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	return nil, addValidator(st, env, false)
}

func addValidator(st *StateManager, env *Delivery, execute bool) error {
	body := env.Transaction.Body.(*protocol.AddValidator)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return err
	}

	// Ensure the key does not already exist
	keyHash := sha256.Sum256(body.PubKey)
	_, _, found := page.EntryByKeyHash(keyHash[:])
	if found {
		return fmt.Errorf("key is already a validator")
	}

	// Add the key hash to the key page
	key := &protocol.KeySpec{PublicKeyHash: keyHash[:]}
	page.Keys = append(page.Keys, key)

	// Update the threshold
	page.AcceptThreshold = protocol.GetValidatorsMOfN(len(page.Keys))

	// Record the update
	didUpdateKeyPage(page)
	st.Update(page)

	// Add the validator
	st.AddValidator(body.PubKey)
	return nil
}

func (RemoveValidator) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	return nil, removeValidator(st, env, false)
}

func removeValidator(st *StateManager, env *Delivery, execute bool) error {
	body := env.Transaction.Body.(*protocol.RemoveValidator)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return err
	}

	// Find the key
	keyHash := sha256.Sum256(body.PubKey)
	index, _, found := page.EntryByKeyHash(keyHash[:])
	if !found {
		return fmt.Errorf("key is not a validator")
	}

	// Make sure it's not the last key
	if len(page.Keys) == 1 {
		return fmt.Errorf("cannot remove the last validator!")
	}

	// Remove the key hash from the key page
	page.Keys = append(page.Keys[:index], page.Keys[index+1:]...)

	// Update the threshold
	page.AcceptThreshold = protocol.GetValidatorsMOfN(len(page.Keys))

	// Record the update
	didUpdateKeyPage(page)
	st.Update(page)

	// Remove the validator
	st.DisableValidator(body.PubKey)
	return nil
}

func (UpdateValidatorKey) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	return nil, updateValidator(st, env, false)
}

func updateValidator(st *StateManager, env *Delivery, execute bool) error {
	body := env.Transaction.Body.(*protocol.UpdateValidatorKey)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return err
	}

	// Find the old key
	oldKeyHash := sha256.Sum256(body.PubKey)
	_, entry, found := page.EntryByKeyHash(oldKeyHash[:])
	if !found {
		return fmt.Errorf("old key is not a validator")
	}

	// Ensure the new key does not already exist
	newKeyHash := sha256.Sum256(body.NewPubKey)
	_, _, found = page.EntryByKeyHash(newKeyHash[:])
	if found {
		return fmt.Errorf("new key is already a validator")
	}

	// Update the key hash
	entry.(*protocol.KeySpec).PublicKeyHash = newKeyHash[:]

	// Record the update
	didUpdateKeyPage(page)
	st.Update(page)

	// Update the validator
	st.DisableValidator(body.PubKey)
	st.AddValidator(body.NewPubKey)
	return nil
}

// checkValidatorTransaction implements common checks for validator
// transactions.
func checkValidatorTransaction(st *StateManager, env *Delivery) (*protocol.KeyPage, error) {
	validatorBookUrl := env.Transaction.Header.Principal
	if !st.nodeUrl.Equal(validatorBookUrl.RootIdentity()) {
		return nil, fmt.Errorf("invalid origin: must be %s, got %s", st.nodeUrl, validatorBookUrl)
	}

	var book *protocol.KeyBook
	err := st.LoadUrlAs(validatorBookUrl, &book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	if book.BookType != protocol.BookTypeValidator {
		return nil, fmt.Errorf("the key book is not of a validator book type")
	}

	pageUrl := protocol.FormatKeyPageUrl(validatorBookUrl, 0)
	var page *protocol.KeyPage
	err = st.LoadUrlAs(pageUrl, &page)
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
