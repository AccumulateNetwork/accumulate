package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/smt/storage"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
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
		record, err := unmarshalRecord(&state.Object{Entry: cc})
		if err != nil {
			return fmt.Errorf("invalid chain payload: %v", err)
		}

		u, err := record.Header().ParseUrl()
		if err != nil {
			return fmt.Errorf("invalid chain URL: %v", err)
		}

		if _, err := st.LoadUrl(u); err == nil {
			return fmt.Errorf("chain %q already exists", u.String())
		} else if !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("error fetching %q: %v", u.String(), err)
		}

		urls[i] = u
		st.Store(record)
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
		case types.ChainTypeAdi:
			// An ADI must be its own identity
			if !u.Identity().Equal(u) {
				return fmt.Errorf("ADI is not its own identity")
			}
		default:
			// Anything else must be a sub-path
			if u.Identity().Equal(u) {
				return fmt.Errorf("%v cannot be its own identity", record.Header().Type)
			}

			// Make sure the ADI actually exists
			_, err = st.LoadUrl(u.Identity())
			if errors.Is(err, storage.ErrNotFound) {
				return fmt.Errorf("missing identity for %s", u.String())
			} else if err != nil {
				return fmt.Errorf("error fetching %q: %v", u.String(), err)
			}
		}

		// Check the key book
		switch record.Header().Type {
		case types.ChainTypeSigSpecGroup:
			// A key book does not itself have a key book
			if record.Header().SigSpecId != (types.Bytes32{}) {
				return errors.New("invalid key book: SigSpecId is not empty")
			}

		default:
			// Anything else must have a key book
			if record.Header().SigSpecId == (types.Bytes32{}) {
				return fmt.Errorf("%q does not specify a key book", u)
			}

			// Make sure the key book actually exists
			ssg := new(protocol.SigSpecGroup)
			err = st.LoadAs(record.Header().SigSpecId, ssg)
			if err != nil {
				return fmt.Errorf("invalid key book for %q: %v", u, err)
			}
		}

		// Store the record (pending)
		st.Store(record)
	}

	return nil
}

func (SyntheticCreateChain) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	return SyntheticCreateChain{}.Validate(st, tx)
}

func (SyntheticCreateChain) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	return SyntheticCreateChain{}.Validate(st, tx)
}
