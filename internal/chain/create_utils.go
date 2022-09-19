package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
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

	dir, err := st.batch.Account(account.Identity()).Directory().Get()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load directory index: %w", err)
	}

	if len(dir)+1 > int(st.Globals.Globals.Limits.AdiAccounts) {
		return errors.Format(errors.StatusBadRequest, "adi would have too many accounts")
	}

	return nil
}
