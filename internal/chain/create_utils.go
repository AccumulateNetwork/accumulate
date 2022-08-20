package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func checkCreateAdiAccount(st *StateManager, account *url.URL) error {
	// ADI accounts can only be created within an ADI
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return errors.Format(errors.StatusBadRequest, "invalid principal: want account type %v, got %v", protocol.AccountTypeIdentity, st.Origin.Type())
	}

	// The origin must be the parent
	if !account.Identity().Equal(st.OriginUrl) {
		return errors.Format(errors.StatusBadRequest, "invalid principal: cannot create %v as a child of %v", account, st.OriginUrl)
	}

	return nil
}
