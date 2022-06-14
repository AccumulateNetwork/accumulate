package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateNetworkGlobals struct{}

var _ SignerValidator = (*UpdateNetworkGlobals)(nil)

func (UpdateNetworkGlobals) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateNetworkGlobals
}

// SignerIsAuthorized returns nil if the transaction is writing to a lite data
// account.
func (UpdateNetworkGlobals) SignerIsAuthorized(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, _ protocol.Signer, _ bool) (fallback bool, err error) {
	lite, err := isWriteToLiteDataAccount(batch, transaction)
	if err != nil {
		return false, errors.Wrap(errors.StatusUnknown, err)
	}

	return !lite, nil
}

// TransactionIsReady returns true if the transaction is writing to a lite data
// account.
func (UpdateNetworkGlobals) TransactionIsReady(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	lite, err := isWriteToLiteDataAccount(batch, transaction)
	if err != nil {
		return false, false, errors.Wrap(errors.StatusUnknown, err)
	}

	// Writing to a lite data account only requires one signature
	if lite {
		return len(status.Signers) > 0, false, nil
	}

	// Fallback to general authorization
	return false, true, nil
}

func (UpdateNetworkGlobals) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (UpdateNetworkGlobals{}).Validate(st, tx)
}

func (UpdateNetworkGlobals) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateNetworkGlobals)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.UpdateNetworkGlobals), tx.Transaction.Body)
	}

	globals := st.Globals.Copy()
	modified := false
	if body.OperatorAcceptThreshold != nil && !globals.Globals.OperatorAcceptThreshold.Equal(body.OperatorAcceptThreshold) {
		globals.Globals.OperatorAcceptThreshold = *body.OperatorAcceptThreshold
		modified = true
	}
	if len(body.MajorBlockSchedule) > 0 && globals.Globals.MajorBlockSchedule != body.MajorBlockSchedule {
		globals.Globals.MajorBlockSchedule = body.MajorBlockSchedule
		modified = true
	}

	if modified {
		err := globals.StoreGlobals(st.Network, func(accountUrl *url.URL, target interface{}) error {
			da := new(protocol.DataAccount)
			da.Url = accountUrl
			da.AddAuthority(st.Network.OperatorBook())
			return encoding.SetPtr(da, target)
		}, func(account protocol.Account) error {
			err := st.Update(account)
			if err != nil {
				return errors.Format(errors.StatusUnknown, "store account: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
	}

	return nil, nil
}
