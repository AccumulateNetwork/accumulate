package chain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type SyntheticCreateChain struct{}

func (SyntheticCreateChain) Type() types.TxType {
	return types.TxTypeSyntheticCreateChain
}

func (SyntheticCreateChain) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticCreateChain)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if body.Cause == [32]byte{} {
		return fmt.Errorf("cause is missing")
	}

	// Do basic validation and add everything to the state manager
	urls := make([]*url.URL, len(body.Chains))
	for i, cc := range body.Chains {
		record, err := unmarshalRecord(&state.Object{Entry: cc.Data})
		if err != nil {
			return fmt.Errorf("invalid chain payload: %v", err)
		}

		u, err := record.Header().ParseUrl()
		if err != nil {
			return fmt.Errorf("invalid chain URL: %v", err)
		}

		_, err = st.LoadUrl(u)
		switch {
		case err != nil && !errors.Is(err, storage.ErrNotFound):
			return fmt.Errorf("error fetching %q: %v", u, err)
		case cc.IsUpdate && errors.Is(err, storage.ErrNotFound):
			return fmt.Errorf("cannot update %q: does not exist", u)
		case !cc.IsUpdate && err == nil:
			return fmt.Errorf("cannot create %q: already exists", u)
		}

		urls[i] = u
		st.Update(record)
	}

	// Verify everything is sane
	for _, u := range urls {
		record, err := st.LoadUrl(u)
		if err != nil {
			// This really shouldn't happen, but don't panic
			return fmt.Errorf("internal error: failed to fetch pending record")
		}

		// Check the identity
		switch record.Header().Type {
		case types.ChainTypeIdentity:
			// An ADI must be its own identity
			if !u.Identity().Equal(u) {
				return fmt.Errorf("ADI is not its own identity")
			}
		default:
			// Anything else must be a sub-path
			if u.Identity().Equal(u) {
				return fmt.Errorf("chain type %v cannot be its own identity", record.Header().Type)
			}

			if u.Path != "" && strings.Contains(u.Path[1:], "/") {
				return fmt.Errorf("chain type %v cannot contain more than one slash in its URL", record.Header().Type)
			}

			// Make sure the ADI actually exists
			_, err = st.LoadUrl(u.Identity())
			if errors.Is(err, storage.ErrNotFound) {
				return fmt.Errorf("missing identity for %s", u.String())
			} else if err != nil {
				return fmt.Errorf("error fetching %q: %v", u.String(), err)
			}

			// Update the ADI's directory index
			err = st.AddDirectoryEntry(u)
			if err != nil {
				return fmt.Errorf("failed to add ADI directory entry: %v", err)
			}
		}

		// Check the key book
		switch record.Header().Type {
		case types.ChainTypeKeyBook:
			// A key book does not itself have a key book
			if record.Header().KeyBook != (types.Bytes32{}) {
				return errors.New("invalid key book: KeyBook is not empty")
			}

		case types.ChainTypeKeyPage:
			// A key page can be unbound

		default:
			// Anything else must have a key book
			if record.Header().KeyBook == (types.Bytes32{}) {
				return fmt.Errorf("%q does not specify a key book", u)
			}
		}

		// Make sure the key book actually exists
		if record.Header().KeyBook != (types.Bytes32{}) {
			ssg := new(protocol.KeyBook)
			err = st.LoadAs(record.Header().KeyBook, ssg)
			if err != nil {
				return fmt.Errorf("invalid key book for %q: %v", u, err)
			}
		}
	}

	return nil
}
