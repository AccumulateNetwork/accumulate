// Copyright 2023 The Accumulate Authors
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

func checkCreateAdiAccount(st *StateManager, account *url.URL) error {
	// ADI accounts can only be created within an ADI
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return errors.BadRequest.WithFormat("invalid principal: want account type %v, got %v", protocol.AccountTypeIdentity, st.Origin.Type())
	}

	// The origin must be the parent
	if !account.Identity().Equal(st.OriginUrl) {
		return errors.BadRequest.WithFormat("invalid principal: cannot create %v as a child of %v", account, st.OriginUrl)
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
