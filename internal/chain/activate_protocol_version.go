package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ActivateProtocolVersion struct{}

func (ActivateProtocolVersion) Type() protocol.TransactionType {
	return protocol.TransactionTypeActivateProtocolVersion
}

func (ActivateProtocolVersion) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (ActivateProtocolVersion{}).Validate(st, tx)
}

func (ActivateProtocolVersion) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.ActivateProtocolVersion)
	if !ok {
		return nil, errors.Format(errors.BadRequest, "invalid payload: want %v, got %v", protocol.TransactionTypeActivateProtocolVersion, tx.Transaction.Body.Type())
	}

	if !st.NodeUrl().Equal(st.OriginUrl) {
		return nil, errors.Format(errors.BadRequest, "invalid principal: want %v, got %v", st.NodeUrl(), st.OriginUrl)
	}

	var ledger *protocol.SystemLedger
	err := st.batch.Account(st.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.Format(errors.UnknownError, "load ledger: %w", err)
	}

	if body.Version <= ledger.ExecutorVersion {
		return nil, errors.Format(errors.BadRequest, "new version (%d) <= old version (%d)", body.Version, ledger.ExecutorVersion)
	}

	ledger.ExecutorVersion = body.Version
	err = st.Update(ledger)
	if err != nil {
		return nil, errors.Format(errors.UnknownError, "store ledger: %w", err)
	}

	return nil, nil
}
