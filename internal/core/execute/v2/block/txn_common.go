// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import "gitlab.com/accumulatenetwork/accumulate/protocol"

// TransactionContext is the context in which a transaction is executed.
type TransactionContext struct {
	*MessageContext
	transaction *protocol.Transaction
}
