package chain

import (
	"errors"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SyntheticCreateChain struct{}

func (SyntheticCreateChain) Type() types.TxType {
	return types.TxTypeSyntheticCreateChain
}

func (SyntheticCreateChain) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticCreateChain)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticCreateChain), tx.Transaction.Body)
	}

	if body.Cause == [32]byte{} {
		return nil, fmt.Errorf("cause is missing")
	}

	// Do basic validation and add everything to the state manager
	urls := make([]*url.URL, len(body.Chains))
	for i, cc := range body.Chains {
		record, err := protocol.UnmarshalAccount(cc.Data)
		if err != nil {
			return nil, fmt.Errorf("invalid chain payload: %v", err)
		}

		u, err := record.Header().ParseUrl()
		if err != nil {
			return nil, fmt.Errorf("invalid chain URL: %v", err)
		}

		_, err = st.LoadUrl(u)
		switch {
		case err != nil && !errors.Is(err, storage.ErrNotFound):
			return nil, fmt.Errorf("error fetching %q: %v", u, err)
		case cc.IsUpdate && errors.Is(err, storage.ErrNotFound):
			return nil, fmt.Errorf("cannot update %q: does not exist", u)
		case !cc.IsUpdate && err == nil:
			return nil, fmt.Errorf("cannot create %q: already exists", u)
		case !cc.IsUpdate:

			_, err := st.LoadUrl(u)
			if err == nil {
				return nil, fmt.Errorf("cannot create %q: already exists", u)
			}
			err = st.AddDirectoryEntry(u)
		}

		urls[i] = u
		st.Update(record)
	}

	// Verify everything is sane
	for _, u := range urls {
		record, err := st.LoadUrl(u)
		if err != nil {
			// This really shouldn't happen, but don't panic
			return nil, fmt.Errorf("internal error: failed to fetch pending record")
		}

		// Check the identity
		switch record.GetType() {
		case protocol.AccountTypeIdentity:
		default:
			// Make sure the ADI actually exists
			_, err = st.LoadUrl(u.Identity()) // TODO debug if this works
			//_, err = st.LoadUrl(u.RootIdentity())
			if errors.Is(err, storage.ErrNotFound) {
				return nil, fmt.Errorf("missing identity for %s", u.String())
			} else if err != nil {
				return nil, fmt.Errorf("error fetching %q: %v", u.String(), err)
			}

		}

		// Check the key book
		switch record.GetType() {
		case protocol.AccountTypeKeyBook:
			// A key book does not itself have a key book
			if record.Header().KeyBook != nil {
				return nil, errors.New("invalid key book: KeyBook is not empty")
			}

		case protocol.AccountTypeKeyPage:
			// A key page can be unbound

		default:
			// Anything else must have a key book
			if record.Header().KeyBook == nil {
				return nil, fmt.Errorf("%q does not specify a key book", u)
			}
		}

		// Make sure the key book actually exists
		if record.Header().KeyBook != nil {
			book := new(protocol.KeyBook)
			err = st.LoadUrlAs(record.Header().KeyBook, book)
			if err != nil {
				return nil, fmt.Errorf("invalid key book %q for %q: %v", record.Header().KeyBook, u, err)
			}
		}
	}

	return nil, nil
}
