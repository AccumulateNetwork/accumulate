package chain

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateIdentity struct{}

func (CreateIdentity) Type() types.TxType { return types.TxTypeCreateIdentity }

func (ci CreateIdentity) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	// *protocol.IdentityCreate, *url.URL, state.Chain
	body, ok := tx.Transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateIdentity), tx.Transaction.Body)
	}
	switch st.Origin.(type) {
	case *protocol.LiteTokenAccount, *protocol.ADI:
		// OK
	default:
		return nil, fmt.Errorf("account type %d cannot be the origininator of ADIs", st.Origin.GetType())
	}

	err := protocol.IsValidAdiUrl(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	bookUrl := body.KeyBookUrl
	if bookUrl == nil {
		return nil, fmt.Errorf("missing KeyBook URL")
	}
	err = protocol.IsValidAdiUrl(bookUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid KeyBook URL %q: %v", bookUrl.String(), err)
	}

	pageUrl, err := validateKeyPageUrl(bookUrl, body.KeyPageUrl)
	if err != nil {
		return nil, err
	}
	page := protocol.NewKeyPage()
	var pageExists = st.LoadUrlAs(pageUrl, page) == nil
	if !pageExists {
		page.Url = pageUrl
		page.KeyBook = bookUrl
		page.Threshold = 1 // Require one signature from the Key Page

		if len(body.PublicKey) == 0 {
			return nil, fmt.Errorf("missing PublicKey which is required when creating a new KeyPage")
		}
		keySpec := new(protocol.KeySpec)
		keySpec.PublicKey = body.PublicKey
		page.Keys = append(page.Keys, keySpec)
	}

	book := protocol.NewKeyBook()
	bookExists := st.LoadUrlAs(bookUrl, book) == nil
	if !bookExists {
		book.Url = bookUrl
		book.Pages = append(book.Pages, pageUrl)
	}

	identity := protocol.NewADI()
	identity.Url = body.Url
	identity.KeyBook = bookUrl
	identity.ManagerKeyBook = body.Manager

	st.Create(identity)
	if !bookExists {
		st.Create(book)
	}
	if !pageExists {
		st.Create(page)
	}
	return nil, nil
}

func validateKeyPageUrl(keyBookUrl *url.URL, keyPageUrl *url.URL) (*url.URL, error) {
	var err error
	bkParentUrl, err := keyBookUrl.Parent()
	if err != nil {
		return nil, fmt.Errorf("invalid KeyBook URL: %w\nthe KeyBook URL should be \"adi_path/<KeyBook>\"", err)
	}
	if keyPageUrl == nil {
		return bkParentUrl.JoinPath(protocol.DefaultKeyPage), nil
	} else {
		kpParentUrl, err := keyPageUrl.Parent()
		if err != nil {
			return nil, fmt.Errorf("invalid KeyPage URL: %w\nthe KeyPage URL should be \"adi_path/<KeyPage>\"", err)
		}

		if !bkParentUrl.Equal(kpParentUrl) {
			return nil, fmt.Errorf("KeyPage %q should have the same ADI parent path as its KeyBook %q", keyPageUrl, keyBookUrl)
		}
	}

	return keyPageUrl, nil
}
