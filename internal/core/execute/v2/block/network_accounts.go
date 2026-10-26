// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// processNetworkAccountUpdates validates updates to network data accounts,
// parses the globals they change, and pushes updates when necessary.
//
// From V2Canaveral it changes nothing in memory: it returns the pending
// globals the transaction makes, or nil if it makes no change, and the caller
// makes them the executor's pending globals once the transaction has
// succeeded. Before, it wrote the pending globals as it parsed, so a
// transaction rejected after the parse, or whose commit failed, still changed
// them (#4615).
func (x *Executor) processNetworkAccountUpdates(st *chain.StateManager, delivery *chain.Delivery, principal protocol.Account) (*core.GlobalValues, error) {
	r := x.BlockTimers.Start(BlockTimerTypeNetworkAccountUpdates)
	defer x.BlockTimers.Stop(r)

	var next *core.GlobalValues
	staged := x.globals().Active.ExecutorVersion.V2CanaveralEnabled()
	pending := func() *core.GlobalValues {
		switch {
		case !staged:
			return &x.globals().Pending
		case next == nil:
			next = x.globals().Pending.Copy()
		}
		return next
	}
	err := x.updateNetworkAccount(st, delivery, principal, pending, staged)
	if err != nil {
		return nil, err
	}
	return next, nil
}

// updateNetworkAccount does the work of processNetworkAccountUpdates, parsing
// changes into the globals pending returns.
func (x *Executor) updateNetworkAccount(st *chain.StateManager, delivery *chain.Delivery, principal protocol.Account, pending func() *core.GlobalValues, staged bool) error {

	// Only process updates to network accounts. To prevent potential issues
	// replaying history, this condition must reject everything that was
	// rejected by v0 except ActivateProtocolVersion.
	switch {
	case principal == nil:
		return nil
	case x.Describe.NodeUrl().PrefixOf(principal.GetUrl()):
		// Ok
	case delivery.Transaction.Body.Type() == protocol.TransactionTypeActivateProtocolVersion:
		// Ok - no other condition is necessary because ActivateProtocolVersion
		// will fail if the principal is not the partition ADI
	default:
		return nil
	}

	// Allow system transactions to do their thing
	if delivery.Transaction.Body.Type().IsSystem() {
		return nil
	}

	// Do not forward synthetic transactions
	if x.globals().Active.ExecutorVersion.SignatureAnchoringEnabled() && delivery.Transaction.Body.Type().IsSynthetic() {
		return nil
	}

	targetName := strings.ToLower(strings.Trim(principal.GetUrl().Path, "/"))
	switch body := delivery.Transaction.Body.(type) {
	case *protocol.ActivateProtocolVersion:
		// Add the version change to the pending globals
		pending().ExecutorVersion = body.Version

	case *protocol.UpdateKeyPage:
		switch targetName {
		case protocol.Operators + "/1":
			// Synchronize updates to the operator book
			targetName = protocol.Operators

			page, ok := principal.(*protocol.KeyPage)
			if !ok {
				return errors.InternalError.WithFormat("%v is not a key page", principal.GetUrl())
			}

			// Reject the transaction if the threshold is not set correctly according to the ratio
			expectedThreshold := x.globals().Active.Globals.OperatorAcceptThreshold.Threshold(len(page.Keys))
			if page.AcceptThreshold != expectedThreshold {
				return errors.BadRequest.WithFormat("invalid %v update: incorrect accept threshold: want %d, got %d", principal.GetUrl(), expectedThreshold, page.AcceptThreshold)
			}
		}

	case *protocol.UpdateAccountAuth:
		// Prevent authority changes (prior to v1+signatureAnchoring)
		if !x.globals().Active.ExecutorVersion.SignatureAnchoringEnabled() {
			return errors.BadRequest.WithFormat("the authority set of a network account cannot be updated")
		}

	case *protocol.WriteData:
		var err error
		switch targetName {
		case protocol.Oracle:
			// Validate entry and update variable
			err = pending().ParseOracle(body.Entry)

		case protocol.Globals:
			// Validate entry and update variable
			err = pending().ParseGlobals(body.Entry)

		case protocol.Network:
			// Validate entry and update variable
			if staged {
				err = pending().ParseNetwork(body.Entry)
			} else {
				err = pending().ParseNetworkLegacy(body.Entry)
			}

		case protocol.Routing:
			// Validate entry and update variable
			err = pending().ParseRouting(body.Entry)

		case protocol.Votes,
			protocol.Evidence:
			// Prevent direct writes
			return errors.BadRequest.WithFormat("%v cannot be updated directly", principal.GetUrl())

		default:
			return nil
		}
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Force WriteToState for variable accounts
		if !body.WriteToState {
			return errors.BadRequest.WithFormat("updates to %v must write to state", principal.GetUrl())
		}
	}

	// Only push updates from the directory network
	if x.Describe.NetworkType != protocol.PartitionTypeDirectory {
		// Do not allow direct updates of the BVN accounts
		if !delivery.WasProducedInternally() {
			return errors.BadRequest.WithFormat("%v cannot be updated directly", principal.GetUrl())
		}

		return nil
	}

	// Queue the update for distribution
	var update protocol.NetworkAccountUpdate
	update.Name = targetName
	update.Body = delivery.Transaction.Body
	if x.globals().Active.ExecutorVersion.V2VandenbergEnabled() {
		st.State.NetworkUpdate = append(st.State.NetworkUpdate, &update)

	} else {
		var ledger *protocol.SystemLedger
		record := st.GetBatch().Account(x.Describe.Ledger())
		err := record.Main().GetAs(&ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("load ledger: %w", err)
		}

		ledger.PendingUpdates = append(ledger.PendingUpdates, update)

		err = record.Main().Put(ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("store ledger: %w", err)
		}
	}

	return nil
}
