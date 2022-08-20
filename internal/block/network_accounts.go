package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// processNetworkAccountUpdates processes updates to network data accounts,
// updating the in-memory globals variable and pushing updates when necessary.
func (x *Executor) processNetworkAccountUpdates(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) error {
	r := x.BlockTimers.Start(BlockTimerTypeNetworkAccountUpdates)
	defer x.BlockTimers.Stop(r)
	// Only process updates to network accounts
	if principal == nil || !x.Describe.NodeUrl().PrefixOf(principal.GetUrl()) {
		return nil
	}

	// Allow system transactions to do their thing
	if delivery.Transaction.Body.Type().IsSystem() {
		return nil
	}

	targetName := strings.ToLower(strings.Trim(principal.GetUrl().Path, "/"))
	switch body := delivery.Transaction.Body.(type) {
	case *protocol.UpdateKeyPage:
		switch targetName {
		case protocol.Operators + "/1":
			// Synchronize updates to the operator book
			targetName = protocol.Operators

			page, ok := principal.(*protocol.KeyPage)
			if !ok {
				return errors.Internal.Format("%v is not a key page", principal.GetUrl())
			}

			// Reject the transaction if the threshold is not set correctly according to the ratio
			expectedThreshold := x.globals.Active.Globals.OperatorAcceptThreshold.Threshold(len(page.Keys))
			if page.AcceptThreshold != expectedThreshold {
				return errors.BadRequest.Format("invalid %v update: incorrect accept threshold: want %d, got %d", principal.GetUrl(), expectedThreshold, page.AcceptThreshold)
			}
		}

	case *protocol.UpdateAccountAuth:
		// Prevent authority changes
		return errors.BadRequest.Format("the authority set of a network account cannot be updated")

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
			return errors.BadRequest.Format("%v cannot be updated directly", principal)

		default:
			return nil
		}
		if err != nil {
			return errors.Unknown.Wrap(err)
		}

		// Force WriteToState for variable accounts
		if !body.WriteToState {
			return errors.BadRequest.Format("updates to %v must write to state", principal)
		}
	}

	// Only push updates from the directory network
	if x.Describe.NetworkType != config.Directory {
		// Do not allow direct updates of the BVN accounts
		if !delivery.WasProducedByPushedUpdate() {
			return errors.BadRequest.Format("%v cannot be updated directly", principal.GetUrl())
		}

		return nil
	}

	// Write the update to the ledger
	var ledger *protocol.SystemLedger
	record := batch.Account(x.Describe.Ledger())
	err := record.GetStateAs(&ledger)
	if err != nil {
		return errors.Unknown.Format("load ledger: %w", err)
	}

	var update protocol.NetworkAccountUpdate
	update.Name = targetName
	update.Body = delivery.Transaction.Body
	ledger.PendingUpdates = append(ledger.PendingUpdates, update)

	err = record.PutState(ledger)
	if err != nil {
		return errors.Unknown.Format("store ledger: %w", err)
	}

	return nil
}
