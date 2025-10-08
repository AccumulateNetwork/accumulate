// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// License that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateMiningAuthority struct{}

var _ SignerValidator = (*CreateMiningAuthority)(nil)

func (CreateMiningAuthority) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateMiningAuthority
}

func (CreateMiningAuthority) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateMiningAuthority)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMiningAuthority), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).AuthorityIsAccepted(delegate, batch, transaction, sig)
}

func (CreateMiningAuthority) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateMiningAuthority)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMiningAuthority), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction)
}

func (x CreateMiningAuthority) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Version guard: LXR mining is only available after V2LXRMining activation
	if !st.Globals.ExecutorVersion.V2LXRMiningEnabled() {
		return nil, errors.NotAllowed.With("LXR mining has not been activated on this network")
	}

	_, err := x.check(st, tx)
	return nil, err
}

func (CreateMiningAuthority) check(st *StateManager, tx *Delivery) (*protocol.CreateMiningAuthority, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateMiningAuthority)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateMiningAuthority), tx.Transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("mining authority URL is missing")
	}

	// Validate difficulty is set
	if body.Difficulty == 0 {
		return nil, errors.BadRequest.WithFormat("difficulty must be greater than zero")
	}

	// Validate table size is reasonable (10-35 bits = 1KB to 32GB)
	if body.TableSize < 10 || body.TableSize > 35 {
		return nil, errors.BadRequest.WithFormat("table size must be between 10 and 35 bits (got %d)", body.TableSize)
	}

	// Validate passes is reasonable
	if body.Passes == 0 || body.Passes > 100 {
		return nil, errors.BadRequest.WithFormat("passes must be between 1 and 100 (got %d)", body.Passes)
	}

	// Validate all authority URLs are not nil
	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	// Validate all authorized miner URLs are not nil (if specified)
	for _, u := range body.AuthorizedMiners {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authorized miner URL is nil")
		}
	}

	err := originIsParent(tx, body.Url)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return body, nil
}

func (x CreateMiningAuthority) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Version guard: LXR mining is only available after V2LXRMining activation
	if !st.Globals.ExecutorVersion.V2LXRMiningEnabled() {
		return nil, errors.NotAllowed.With("LXR mining has not been activated on this network")
	}

	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	err = checkCreateAdiAccount(st, body.Url)
	if err != nil {
		return nil, err
	}

	// Create the mining authority account
	account := new(protocol.MiningAuthority)
	account.Url = body.Url
	account.Enabled = true // Enabled by default when created
	account.Difficulty = body.Difficulty
	account.TableSize = body.TableSize
	account.TableSeed = body.TableSeed
	account.Passes = body.Passes
	account.AuthorizedMiners = body.AuthorizedMiners

	// Initialize statistics if not provided
	if account.Statistics == nil {
		account.Statistics = new(protocol.MiningStatistics)
	}

	err = setInitialAuthorities(st, account, body.Authorities)
	if err != nil {
		return nil, err
	}

	err = st.Create(account)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", account.Url, err)
	}

	return nil, nil
}
