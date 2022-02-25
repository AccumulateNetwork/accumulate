package chain

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateIdentity struct{}

func (CreateIdentity) Type() types.TxType { return types.TxTypeCreateIdentity }

func (CreateIdentity) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
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
		return nil, fmt.Errorf("missing key book URL")
	}
	err = protocol.IsValidAdiUrl(bookUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid key book URL %q: %v", bookUrl.String(), err)
	}

	if body.KeyPageName == "" {
		return nil, fmt.Errorf("missing key page name")
	}
	bookParentUrl := bookUrl.String()
	bookParentUrl = bookParentUrl[:strings.LastIndex(bookParentUrl, "/")]
	pageUrl, err := url.Parse(bookParentUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid key book parent URL %q: %v", pageUrl.String(), err)
	}
	pageUrl = pageUrl.JoinPath(body.KeyPageName)

	keySpec := new(protocol.KeySpec)
	keySpec.PublicKey = body.PublicKey

	page := protocol.NewKeyPage()
	page.Url = pageUrl // TODO Allow override?
	page.Keys = append(page.Keys, keySpec)
	page.KeyBook = bookUrl
	page.Threshold = 1 // Require one signature from the Key Page

	book := protocol.NewKeyBook()
	book.Url = bookUrl
	book.Pages = append(book.Pages, pageUrl)

	identity := protocol.NewADI()

	identity.Url = body.Url
	identity.KeyBook = bookUrl
	identity.ManagerKeyBook = body.Manager

	st.Create(identity, book, page)
	return nil, nil
}
