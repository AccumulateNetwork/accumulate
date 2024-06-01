// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionContext is the context in which a transaction is executed.
type TransactionContext struct {
	*MessageContext
	transaction *protocol.Transaction
}

func (x *TransactionContext) effectivePrincipal() *url.URL {
	// If we're on Vandenberg and the transaction is a DN anchor...
	if !x.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() ||
		x.transaction.Body.Type() != protocol.TransactionTypeDirectoryAnchor {
		return x.transaction.Header.Principal
	}

	// Use the local anchor ledger as the effective principal
	return x.Executor.Describe.AnchorPool()
}
