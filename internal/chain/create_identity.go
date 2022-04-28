package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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

	err := protocol.IsValidAdiUrl(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	bookUrl := body.KeyBookUrl
	if bookUrl != nil {
		err = validateKeyBookUrl(bookUrl, body.Url)
		if err != nil {
			return nil, err
		}
	}

	identity := new(protocol.ADI)
	identity.Url = body.Url
	identity.AddAuthority(bookUrl)

	if body.Manager != nil {
		err = st.AddAuthority(identity, body.Manager)
		if err != nil {
			return nil, err
		}
	}

	if bookUrl == nil {
		return nil, fmt.Errorf("key book url is required to create identity")
	}

	accounts := []protocol.Account{identity}
	var book *protocol.KeyBook
	err = st.LoadUrlAs(bookUrl, &book)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, storage.ErrNotFound):
		if len(body.KeyHash) == 0 {
			return nil, fmt.Errorf("missing PublicKey which is required when creating a new KeyBook/KeyPage pair")
		}

		book = new(protocol.KeyBook)
		book.Url = bookUrl
		book.PageCount = 1
		book.AddAuthority(bookUrl)
		accounts = append(accounts, book)
		if len(body.KeyHash) != 32 {
			return nil, fmt.Errorf("invalid Key Hash: length must be equal to 32 bytes")
		}
		page := new(protocol.KeyPage)
		page.Version = 1
		page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)
		page.AcceptThreshold = 1 // Require one signature from the Key Page
		keySpec := new(protocol.KeySpec)
		keySpec.PublicKeyHash = body.KeyHash
		page.Keys = append(page.Keys, keySpec)
		accounts = append(accounts, page)
	default:
		return nil, err
	}

	if !tx.Transaction.Header.Principal.LocalTo(body.Url) {
		st.Submit(body.Url, &protocol.SyntheticCreateIdentity{Accounts: accounts})
		return nil, nil
	}

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
