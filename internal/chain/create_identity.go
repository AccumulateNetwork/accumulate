package chain

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"strings"
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
		return nil, fmt.Errorf("missing key book URL")
	}
	err = protocol.IsValidAdiUrl(bookUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid key book URL %q: %v", bookUrl.String(), err)
	}

	keySpec := new(protocol.KeySpec)
	keySpec.PublicKey = body.PublicKey

	pageUrl, err := ci.selectPageUrl(st, body, err, bookUrl)
	if err != nil {
		return nil, err
	}
	page := protocol.NewKeyPage()
	page.Url = pageUrl
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

func (ci CreateIdentity) selectPageUrl(st *StateManager, body *protocol.CreateIdentity, err error, bookUrl *url.URL) (*url.URL, error) {
	pageUrl := body.KeyPageUrl
	if pageUrl == nil {
		// KeyPageUrl not specified, lookup default/first key page
		book := new(protocol.KeyBook)
		err = st.LoadUrlAs(bookUrl, book)
		if err != nil {
			// Book does not exist yet
			return buildDefaultPageUrl(bookUrl)
		}
		if len(book.Pages) == 0 {
			// Book exists, but does not have a key page somehow
			return buildDefaultPageUrl(bookUrl)
		}
		pageUrl = book.Pages[0]
	}
	return pageUrl, nil
}

func buildDefaultPageUrl(bookUrl *url.URL) (*url.URL, error) {
	parentUrl := bookUrl.String()
	parentUrl = parentUrl[:strings.LastIndex(parentUrl, "/")]
	pageUrl, err := url.Parse(parentUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid key book parent URL %q: %v", pageUrl.String(), err)
	}
	return pageUrl.JoinPath(protocol.DefaultKeyPage), nil
}
