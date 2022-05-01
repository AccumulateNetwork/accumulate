package chain

import (
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type checkValidatorSigner struct{}

type AddValidator struct{ checkValidatorSigner }
type RemoveValidator struct{ checkValidatorSigner }
type UpdateValidatorKey struct{ checkValidatorSigner }

func (checkValidatorSigner) SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) (fallback bool, err error) {
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

func (checkValidatorSigner) TransactionIsReady(*database.Batch, *protocol.Transaction, *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// Do not override the ready check
	return false, true, nil
}

func (AddValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeAddValidator
}

func (AddValidator) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (AddValidator{}).Validate(st, tx)
}

func (RemoveValidator) Type() protocol.TransactionType {
	return protocol.TransactionTypeRemoveValidator
}

func (RemoveValidator) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (RemoveValidator{}).Validate(st, tx)
}

func (UpdateValidatorKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateValidatorKey
}

func (UpdateValidatorKey) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (UpdateValidatorKey{}).Validate(st, tx)
}

func (AddValidator) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	body := env.Transaction.Body.(*protocol.AddValidator)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return nil, err
	}

	// Ensure the key does not already exist
	keyHash := sha256.Sum256(body.PubKey)
	_, _, found := page.EntryByKeyHash(keyHash[:])
	if found {
		return nil, fmt.Errorf("key is already a validator")
	}

	// Add the key hash to the key page
	key := &protocol.KeySpec{PublicKeyHash: keyHash[:]}
	page.Keys = append(page.Keys, key)

	// Update the threshold
	ratio := loadValidatorsThresholdRatio(st, st.nodeUrl.JoinPath(protocol.Globals))
	page.AcceptThreshold = protocol.GetValidatorsMOfN(len(page.Keys), ratio)
	// Record the update
	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	// Add the validator
	st.AddValidator(body.PubKey)
	return nil, nil
}

func (RemoveValidator) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	body := env.Transaction.Body.(*protocol.RemoveValidator)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return nil, err
	}
	// Find the key
	keyHash := sha256.Sum256(body.PubKey)
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

	ratio := loadValidatorsThresholdRatio(st, st.nodeUrl.JoinPath(protocol.Globals))
	page.AcceptThreshold = protocol.GetValidatorsMOfN(len(page.Keys), ratio)
	// Record the update
	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	// Remove the validator
	st.DisableValidator(body.PubKey)
	return nil, nil
}

func (UpdateValidatorKey) Validate(st *StateManager, env *Delivery) (protocol.TransactionResult, error) {
	body := env.Transaction.Body.(*protocol.UpdateValidatorKey)

	page, err := checkValidatorTransaction(st, env)
	if err != nil {
		return nil, err
	}

	// Find the old key
	oldKeyHash := sha256.Sum256(body.PubKey)
	_, entry, found := page.EntryByKeyHash(oldKeyHash[:])
	if !found {
		return nil, fmt.Errorf("old key is not a validator")
	}

	// Ensure the new key does not already exist
	newKeyHash := sha256.Sum256(body.NewPubKey)
	_, _, found = page.EntryByKeyHash(newKeyHash[:])
	if found {
		return nil, fmt.Errorf("new key is already a validator")
	}

	// Update the key hash
	entry.(*protocol.KeySpec).PublicKeyHash = newKeyHash[:]

	// Record the update
	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	// Update the validator
	st.DisableValidator(body.PubKey)
	st.AddValidator(body.NewPubKey)
	return nil, nil
}

// checkValidatorTransaction implements common checks for validator
// transactions.
func checkValidatorTransaction(st *StateManager, env *Delivery) (*protocol.KeyPage, error) {
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

	return page, nil
}

func loadValidatorsThresholdRatio(st *StateManager, url *url.URL) float64 {
	acc := st.stateCache.batch.Account(url)

	data, err := acc.Data()
	if err != nil {
		st.logger.Error("Failed to get globals data chain", "error", err)
		return protocol.FallbackValidatorThreshold
	}

	_, entry, err := data.GetLatest()
	if err != nil {
		st.logger.Error("Failed to get latest globals entry", "error", err)
		return protocol.FallbackValidatorThreshold
	}

	globals := new(protocol.NetworkGlobals)
	err = globals.UnmarshalBinary(entry.Data[0])
	if err != nil {
		st.logger.Error("Failed to decode latest globals entry", "error", err)
		return protocol.FallbackValidatorThreshold
	}

	return globals.ValidatorThreshold.GetFloat()
}
