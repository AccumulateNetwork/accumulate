package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type CreateIdentity struct{}

func (CreateIdentity) Type() protocol.TransactionType { return protocol.TransactionTypeCreateIdentity }

func (CreateIdentity) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateIdentity), tx.Transaction.Body)
	}

	err := validateAdiUrl(body, st.Origin)
	if err != nil {
		return nil, err
	}

	bookUrl := selectBookUrl(body)
	err = validateKeyBookUrl(bookUrl, body.Url)
	if err != nil {
		return nil, err
	}

	identity := new(protocol.ADI)
	identity.Url = body.Url
	identity.KeyBook = bookUrl
	identity.ManagerKeyBook = body.Manager

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
		accounts = append(accounts, book)
		if len(body.KeyHash) != 32 {
			return nil, fmt.Errorf("invalid Key Hash: length must be equal to 32 bytes")
		}
		page := new(protocol.KeyPage)
		page.KeyBook = bookUrl
		page.Version = 1
		page.Url = protocol.FormatKeyPageUrl(bookUrl, 0)
		page.Threshold = 1 // Require one signature from the Key Page
		keySpec := new(protocol.KeySpec)
		keySpec.PublicKeyHash = body.KeyHash
		page.Keys = append(page.Keys, keySpec)
		accounts = append(accounts, page)
	default:
		return nil, err
	}

	st.Create(accounts...)
	return nil, nil
}

func validateAdiUrl(body *protocol.CreateIdentity, origin protocol.Account) error {
	err := protocol.IsValidAdiUrl(body.Url)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	switch v := origin.(type) {
	case *protocol.LiteTokenAccount:
		// OK
	case *protocol.ADI:
		if len(body.Url.Path) > 0 {
			parent, _ := body.Url.Parent()
			if !parent.Equal(v.Url) {
				return fmt.Errorf("a sub ADI %s must be a direct child of its origin ADI %s", body.Url.String(), v.Url.String())
			}
		}
	default:
		return fmt.Errorf("account type %d cannot be the origininator of ADIs", origin.Type())
	}

	return nil
}

func selectBookUrl(body *protocol.CreateIdentity) *url.URL {
	if body.KeyBookUrl == nil {
		return body.Url.JoinPath(protocol.DefaultKeyBook)
	}
	return body.KeyBookUrl
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
