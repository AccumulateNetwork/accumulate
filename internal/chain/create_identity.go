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

func (CreateIdentity) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	// *protocol.IdentityCreate, *url.URL, state.Chain
	body := new(protocol.CreateIdentity)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	identityUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	err = protocol.IsValidAdiUrl(identityUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	switch st.Origin.(type) {
	case *protocol.LiteTokenAccount, *protocol.ADI:
		// OK
	default:
		return nil, fmt.Errorf("account type %d cannot be the origininator of ADIs", st.Origin.Header().Type)
	}

	var pageUrl, bookUrl *url.URL
	if body.KeyBookName == "" {
		return nil, fmt.Errorf("missing key book name")
	} else {
		bookUrl = identityUrl.JoinPath(body.KeyBookName)
	}
	if body.KeyPageName == "" {
		return nil, fmt.Errorf("missing key page name")
	} else {
		pageUrl = identityUrl.JoinPath(body.KeyPageName)
	}

	keySpec := new(protocol.KeySpec)
	keySpec.PublicKey = body.PublicKey

	page := protocol.NewKeyPage()
	page.ChainUrl = types.String(pageUrl.String()) // TODO Allow override
	page.Keys = append(page.Keys, keySpec)
	page.KeyBook = types.String(bookUrl.String())
	page.Threshold = 1 // Require one signature from the Key Page

	book := protocol.NewKeyBook()
	book.ChainUrl = types.String(bookUrl.String()) // TODO Allow override
	book.Pages = append(book.Pages, pageUrl.String())

	identity := protocol.NewADI()
	identity.ChainUrl = types.String(identityUrl.String())
	identity.KeyBook = types.String(bookUrl.String())

	st.Create(identity, book, page)
	return nil, nil
}
