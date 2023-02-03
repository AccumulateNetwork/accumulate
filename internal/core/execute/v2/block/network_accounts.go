// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// processNetworkAccountUpdates processes updates to network data accounts,
// updating the in-memory globals variable and pushing updates when necessary.
func (x *Executor) processNetworkAccountUpdates(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) error {
	r := x.BlockTimers.Start(BlockTimerTypeNetworkAccountUpdates)
	defer x.BlockTimers.Stop(r)

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
	if x.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() && delivery.Transaction.Body.Type().IsSynthetic() {
		return nil
	}

	targetName := strings.ToLower(strings.Trim(principal.GetUrl().Path, "/"))
	switch body := delivery.Transaction.Body.(type) {
	case *protocol.ActivateProtocolVersion:
		// Add the version change to the pending globals
		x.globals.Pending.ExecutorVersion = body.Version

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
			expectedThreshold := x.globals.Active.Globals.OperatorAcceptThreshold.Threshold(len(page.Keys))
			if page.AcceptThreshold != expectedThreshold {
				return errors.BadRequest.WithFormat("invalid %v update: incorrect accept threshold: want %d, got %d", principal.GetUrl(), expectedThreshold, page.AcceptThreshold)
			}
		}

	case *protocol.UpdateAccountAuth:
		// Prevent authority changes (prior to v1+signatureAnchoring)
		if !x.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() {
			return errors.BadRequest.WithFormat("the authority set of a network account cannot be updated")
		}

	case *protocol.WriteData:
		var err error
		switch targetName {
		case protocol.Oracle:
			// Validate entry and update variable
			err = x.globals.Pending.ParseOracle(body.Entry)

		case protocol.Globals:
			// Validate entry and update variable
			err = x.globals.Pending.ParseGlobals(body.Entry)

		case protocol.Network:
			// Validate entry and update variable
			err = x.globals.Pending.ParseNetwork(body.Entry)

		case protocol.Routing:
			// Validate entry and update variable
			err = x.globals.Pending.ParseRouting(body.Entry)

		case protocol.Votes,
			protocol.Evidence:
			// Prevent direct writes
			return errors.BadRequest.WithFormat("%v cannot be updated directly", principal)

		default:
			return nil
		}
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Force WriteToState for variable accounts
		if !body.WriteToState {
			return errors.BadRequest.WithFormat("updates to %v must write to state", principal)
		}
	}

	// Only push updates from the directory network
	if x.Describe.NetworkType != config.Directory {
		// Do not allow direct updates of the BVN accounts
		if !delivery.WasProducedByPushedUpdate() {
			return errors.BadRequest.WithFormat("%v cannot be updated directly", principal.GetUrl())
		}

		return nil
	}

	// Write the update to the ledger
	var ledger *protocol.SystemLedger
	record := batch.Account(x.Describe.Ledger())
	err := record.GetStateAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load ledger: %w", err)
	}

	var update protocol.NetworkAccountUpdate
	update.Name = targetName
	update.Body = delivery.Transaction.Body
	ledger.PendingUpdates = append(ledger.PendingUpdates, update)

	err = record.PutState(ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("store ledger: %w", err)
	}

	return nil
}
