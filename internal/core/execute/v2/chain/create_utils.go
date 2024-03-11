// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func originIsParent(txn *Delivery, account *url.URL) error {
	// The origin must be the parent
	if !account.Identity().Equal(txn.Transaction.Header.Principal) {
		return errors.BadRequest.WithFormat("invalid principal: cannot create %v as a child of %v", account, txn.Transaction.Header.Principal)
	}
	return nil
}

func checkCreateAdiAccount(st *StateManager, account *url.URL) error {
	// ADI accounts can only be created within an ADI
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return errors.BadRequest.WithFormat("invalid principal: want account type %v, got %v", protocol.AccountTypeIdentity, st.Origin.Type())
	}

	dir, err := st.batch.Account(account.Identity()).Directory().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load directory index: %w", err)
	}

	if len(dir)+1 > int(st.Globals.Globals.Limits.IdentityAccounts) {
		return errors.BadRequest.WithFormat("identity would have too many accounts")
	}

	return nil
}

func setInitialAuthorities(st *StateManager, account protocol.FullAccount, authorities []*url.URL) error {
	if !st.Globals.ExecutorVersion.V2BaikonurEnabled() {
		// Old logic
		return st.SetAuth(account, authorities)
	}

	switch {
	case len(authorities) > 0:
		// If the user specified a list of authorities, use them. Each authority
		// is required to sign the transaction; since it is not possible to sign
		// with an invalid authority, there's no need to check now.
		for _, authority := range authorities {
			account.GetAuth().AddAuthority(authority)
		}
		return nil

	case len(account.GetAuth().Authorities) > 0:
		// If the account already has an authority, there's nothing to do
		return nil

	default:
		// Otherwise leave the authority set empty - but verify that there is a
		// parent with a non-empty authority set
		for a := account; !a.GetUrl().IsRootIdentity(); {
			err := st.batch.Account(a.GetUrl().Identity()).Main().GetAs(&a)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}

			if len(a.GetAuth().Authorities) > 0 {
				return nil
			}
		}
		return errors.Conflict.WithFormat("authority set of %v cannot be empty", account.GetUrl())
	}
}
