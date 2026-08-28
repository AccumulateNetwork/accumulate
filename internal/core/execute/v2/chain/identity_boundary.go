// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AssertIdentityBoundary enables the identity-boundary assertion (#4143):
// every account a transaction writes must share the principal's root
// identity. Accounts are assigned to partitions by identity, so a write
// outside the principal's identity could land on an account this partition
// does not even hold — cross-account effects must be produced messages
// instead. The property already holds (a violation would break multi-BVN
// deployments today); the assertion is the regression guard that keeps it
// holding as executors change, and it is the premise sharded execution
// (#4145) is built on.
//
// This is an assert mode, not a consensus rule: enable it in tests and CI
// with ACC_ASSERT_IDENTITY_BOUNDARY=1 (or by setting the variable directly).
// When it fires, the transaction fails and its writes are discarded with it,
// so the violating write never commits.
var AssertIdentityBoundary = os.Getenv("ACC_ASSERT_IDENTITY_BOUNDARY") != ""

// checkIdentityBoundary reports whether writing account is a violation of
// the identity boundary for the current transaction.
//
// System transactions are exempt, explicitly: they are produced by the
// partition itself (anchors, ledger updates, network account propagation)
// and their writes are the partition's own bookkeeping — the exemption in
// the plan's SystemAccountWriteFromASystemTransactionAllowed. Everything
// else, user and synthetic alike, is held to the boundary, with ONE known
// legacy crossing: before Vandenberg, markAcmeBurnt wrote the partition's
// own system ledger from user transactions — the very write Vandenberg
// moved into block state. That path is kept executable for replaying old
// blocks, so it is exempted, narrowly, by version and by account.
func (st *stateCache) checkIdentityBoundary(account protocol.Account) error {
	if !AssertIdentityBoundary || st.txType.IsSystem() || st.principal == nil {
		return nil
	}
	if account.GetUrl().LocalTo(st.principal) {
		return nil
	}
	if st.Globals != nil && !st.Globals.ExecutorVersion.V2VandenbergEnabled() &&
		account.GetUrl().Equal(st.NodeUrl(protocol.Ledger)) {
		return nil // pre-Vandenberg markAcmeBurnt, discovered by the CI run
	}
	return errors.InternalError.WithFormat(
		"identity boundary violation: %v transaction for %v writes %v, which belongs to %v — cross-account effects must be produced messages (#4143)",
		st.txType, st.principal, account.GetUrl(), account.GetUrl().RootIdentity())
}
