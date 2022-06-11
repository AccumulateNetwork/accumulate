package chain

import (
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type checkValidatorSigner struct{}

type AddValidator struct{ checkValidatorSigner }
type RemoveValidator struct{ checkValidatorSigner }
type UpdateValidatorKey struct{ checkValidatorSigner }

var _ SignerValidator = (*AddValidator)(nil)
var _ SignerValidator = (*RemoveValidator)(nil)
var _ SignerValidator = (*UpdateValidatorKey)(nil)

func (checkValidatorSigner) SignerIsAuthorized(_ AuthDelegate, _ *database.Batch, _ *protocol.Transaction, signer protocol.Signer, _ bool) (fallback bool, err error) {
	_, signerPageIdx, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		return false, errors.Format(errors.StatusBadRequest, "signer is not a key page")
	}

	if signerPageIdx > 1 {
		return false, fmt.Errorf("cannot modify validators with a lower priority key page")
	}

	// Run the normal checks
	return true, nil
}

func (checkValidatorSigner) TransactionIsReady(AuthDelegate, *database.Batch, *protocol.Transaction, *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// Do not override the ready check
	return false, true, nil
}

func (AddValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeAddValidator
}

func (AddValidator) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return nil, addValidator(st, tx)
}

func (RemoveValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeRemoveValidator
}

func (RemoveValidator) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return nil, removeValidator(st, tx)
}

func (UpdateValidatorKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateValidatorKey
}

func (UpdateValidatorKey) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (UpdateValidatorKey{}).Validate(st, tx)
}

func (AddValidator) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	return nil, addValidator(st, env)
}

func addValidator(st *StateManager, env *Delivery) error {
	body := env.Transaction.Body.(*protocol.AddValidator)

	vldPage, err := checkValidatorTransaction(st, env)
	if err != nil {
		return err
	}

	var oprPage *protocol.KeyPage
	err = st.LoadUrlAs(st.Describe.DefaultOperatorPage(), &oprPage)
	if err != nil {
		return fmt.Errorf("unable to load %s: %v", st.Describe.DefaultOperatorPage(), err)
	}

	// Ensure the key does not already exist
	keyHash := sha256.Sum256(body.PubKey)
	_, _, found := vldPage.EntryByKeyHash(keyHash[:])
	if found {
		return fmt.Errorf("key is already a validator")
	}

	// Ensure the key is already an operator
	_, _, found = oprPage.EntryByKeyHash(keyHash[:])
	if !found {
		return fmt.Errorf("key has to be an operator first")
	}

	// Add the key hash to the key page
	vldPage.AddKeySpec(&protocol.KeySpec{PublicKeyHash: keyHash[:]})

	// Record the update
	didUpdateKeyPage(vldPage)
	err = st.Update(vldPage)
	if err != nil {
		return fmt.Errorf("failed to update %v: %v", vldPage.GetUrl(), err)
	}

	// Add the validator
	st.AddValidator(body.PubKey)
	return nil
}

func (RemoveValidator) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	return nil, removeValidator(st, env)
}

func removeValidator(st *StateManager, env *Delivery) error {
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
	page.RemoveKeySpecAt(index)

	/*  This no longer does anything because the signing is now governed by the operator book
	TODO Remove when sure nothing will be governed by validtor books
	// Update the threshold
	ratio := st.Globals.Globals.ValidatorThreshold.GetFloat()
	page.AcceptThreshold = protocol.GetMOfN(len(page.Keys), ratio)
	*/
	// Record the update
	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	// Remove the validator
	st.DisableValidator(body.PubKey)
	return nil
}

func (UpdateValidatorKey) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	return nil, updateValidator(st, env)
}

func updateValidator(st *StateManager, env *Delivery) error {
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
	err = st.Update(page)
	if err != nil {
		return fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	// Update the validator
	st.DisableValidator(body.PubKey)
	st.AddValidator(body.NewPubKey)
	return nil
}

// checkValidatorTransaction implements common checks for validator
// transactions.
func checkValidatorTransaction(st *StateManager, env *Delivery) (*protocol.KeyPage, error) {
	validatorBookUrl := env.Transaction.Header.Principal
	if !st.NodeUrl().Equal(validatorBookUrl.RootIdentity()) {
		return nil, fmt.Errorf("invalid principal: must be %s, got %s", st.NodeUrl(), validatorBookUrl)
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

	return page, nil
}
