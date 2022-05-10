package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateIdentity struct{}

func (CreateIdentity) Type() protocol.TransactionType { return protocol.TransactionTypeCreateIdentity }

func (CreateIdentity) SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) (fallback bool, err error) {
	// Anyone is allowed to sign for a root identity
	if transaction.Header.Principal.IsRootIdentity() {
		return false, nil
	}

	// Fallback to general authorization
	return true, nil
}

func (CreateIdentity) TransactionIsReady(batch *database.Batch, transaction *protocol.Transaction, _ *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// Anyone is allowed to sign for a root identity
	if transaction.Header.Principal.IsRootIdentity() {
		return true, false, nil
	}

	// Fallback to general authorization
	return true, false, nil
}

func (CreateIdentity) AllowMissingPrincipal(transaction *protocol.Transaction) (allow, fallback bool) {
	// The principal can be missing if it is a root identity
	return transaction.Header.Principal.IsRootIdentity(), false
}

func (CreateIdentity) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateIdentity{}).Validate(st, tx)
}

func (CreateIdentity) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateIdentity), tx.Transaction.Body)
	}

	// TODO Require the principal to be the ADI when creating a root identity?
	if !body.Url.IsRootIdentity() {
		if !body.Url.Identity().Equal(tx.Transaction.Header.Principal) {
			return nil, errors.Format(errors.StatusUnauthorized, "cannot use %v to create %v", tx.Transaction.Header.Principal, body.Url)
		}
	}

	if body.KeyBookUrl == nil && len(body.Authorities) == 0 && body.Url.IsRootIdentity() {
		return nil, fmt.Errorf("a root identity cannot be created with an empty authority set")
	}

	err := protocol.IsValidAdiUrl(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	identity := new(protocol.ADI)
	identity.Url = body.Url
	accounts := []protocol.Account{identity}

	// Create a new key book
	if body.KeyBookUrl != nil {
		// Verify the user provided a first key
		if len(body.KeyHash) == 0 {
			return nil, fmt.Errorf("missing PublicKey which is required when creating a new KeyBook/KeyPage pair")
		}

		// Verify the URL is ok
		err = validateKeyBookUrl(body.KeyBookUrl, body.Url)
		if err != nil {
			return nil, err
		}

		// Add it to the authority set
		identity.AddAuthority(body.KeyBookUrl)

		// Create the book
		book := new(protocol.KeyBook)
		book.Url = body.KeyBookUrl
		book.PageCount = 1
		book.AddAuthority(body.KeyBookUrl)
		accounts = append(accounts, book)
		if len(body.KeyHash) != 32 {
			return nil, fmt.Errorf("invalid Key Hash: length must be equal to 32 bytes")
		}

		// Create the page
		page := new(protocol.KeyPage)
		page.Version = 1
		page.Url = protocol.FormatKeyPageUrl(body.KeyBookUrl, 0)
		page.AcceptThreshold = 1 // Require one signature from the Key Page
		keySpec := new(protocol.KeySpec)
		keySpec.PublicKeyHash = body.KeyHash
		page.Keys = append(page.Keys, keySpec)
		accounts = append(accounts, page)
	}

	// Add additional authorities or inherit
	err = st.SetAuth(identity, body.Authorities)
	if err != nil {
		return nil, err
	}

	// If the ADI is remote, use a synthetic transaction
	if !tx.Transaction.Header.Principal.LocalTo(body.Url) {
		st.Submit(body.Url, &protocol.SyntheticCreateIdentity{Accounts: accounts})
		return nil, nil
	}

	// If the ADI is local, create it directly
	err = st.Create(accounts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %v", body.Url, err)
	}

	return nil, nil
}

func validateKeyBookUrl(bookUrl *url.URL, adiUrl *url.URL) error {
	err := protocol.IsValidAdiUrl(bookUrl)
	if err != nil {
		return fmt.Errorf("invalid KeyBook URL %s: %v", bookUrl.String(), err)
	}
	parent, ok := bookUrl.Parent()
	if !ok {
		return fmt.Errorf("invalid URL %s, the KeyBook URL must be adi_path/KeyBook", bookUrl)
	}
	if !parent.Equal(adiUrl) {
		return fmt.Errorf("KeyBook %s must be a direct child of its ADI %s", bookUrl.String(), adiUrl.String())
	}
	return nil
}
