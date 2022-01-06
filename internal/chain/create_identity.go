package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateIdentity struct{}

func (CreateIdentity) Type() types.TxType { return types.TxTypeCreateIdentity }

func (CreateIdentity) Validate(st *StateManager, tx *transactions.Envelope) error {
	// *protocol.IdentityCreate, *url.URL, state.Chain
	body := new(protocol.CreateIdentity)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	identityUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	err = protocol.IsValidAdiUrl(identityUrl)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	switch st.Origin.(type) {
	case *protocol.LiteTokenAccount, *protocol.ADI:
		// OK
	default:
		return fmt.Errorf("chain type %d cannot be the origininator of ADIs", st.Origin.Header().Type)
	}

	var pageUrl, bookUrl *url.URL
	if body.KeyBookName == "" {
		return fmt.Errorf("missing key book name")
	} else {
		bookUrl = identityUrl.JoinPath(body.KeyBookName)
	}
	if body.KeyPageName == "" {
		return fmt.Errorf("missing key page name")
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
	return nil
}
