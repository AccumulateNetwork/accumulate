package chain

import (
	"errors"
	"fmt"
	"strings"

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
	body := new(protocol.SyntheticCreateChain)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	if body.Cause == [32]byte{} {
		return nil, fmt.Errorf("cause is missing")
	}

	// Do basic validation and add everything to the state manager
	urls := make([]*url.URL, len(body.Chains))
	for i, cc := range body.Chains {
		record, err := protocol.UnmarshalChain(cc.Data)
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
		switch record.Header().Type {
		case types.AccountTypeIdentity:
			// An ADI must be its own identity
			if !u.Identity().Equal(u) {
				return nil, fmt.Errorf("ADI is not its own identity")
			}
		default:
			// Anything else must be a sub-path
			if u.Identity().Equal(u) {
				return nil, fmt.Errorf("account type %v cannot be its own identity", record.Header().Type)
			}

			if u.Path != "" && strings.Contains(u.Path[1:], "/") {
				return nil, fmt.Errorf("account type %v cannot contain more than one slash in its URL", record.Header().Type)
			}

			// Make sure the ADI actually exists
			_, err = st.LoadUrl(u.Identity())
			if errors.Is(err, storage.ErrNotFound) {
				return nil, fmt.Errorf("missing identity for %s", u.String())
			} else if err != nil {
				return nil, fmt.Errorf("error fetching %q: %v", u.String(), err)
			}

		}

		// Check the key book
		switch record.Header().Type {
		case types.AccountTypeKeyBook:
			// A key book does not itself have a key book
			if record.Header().KeyBook != "" {
				return nil, errors.New("invalid key book: KeyBook is not empty")
			}

		case types.AccountTypeKeyPage:
			// A key page can be unbound

		default:
			// Anything else must have a key book
			if record.Header().KeyBook == "" {
				return nil, fmt.Errorf("%q does not specify a key book", u)
			}
		}

		// Make sure the key book actually exists
		if record.Header().KeyBook != "" {
			book := new(protocol.KeyBook)
			url, err := url.Parse(record.Header().KeyBook)
			if err != nil {
				return nil, fmt.Errorf("invalid keybook url %s", url.String())
			}
			err = st.LoadUrlAs(url, book)
			if err != nil {
				return nil, fmt.Errorf("invalid key book for %q: %v", u, err)
			}
		}
	}

	return nil, nil
}
