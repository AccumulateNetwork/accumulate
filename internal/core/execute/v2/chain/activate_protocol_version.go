// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ActivateProtocolVersion struct{}

func (ActivateProtocolVersion) Type() protocol.TransactionType {
	return protocol.TransactionTypeActivateProtocolVersion
}

func (x ActivateProtocolVersion) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (ActivateProtocolVersion) check(st *StateManager, tx *Delivery) (*protocol.ActivateProtocolVersion, error) {
	// Verify the body
	body, ok := tx.Transaction.Body.(*protocol.ActivateProtocolVersion)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid payload: want %v, got %v", protocol.TransactionTypeActivateProtocolVersion, tx.Transaction.Body.Type())
	}

	// Verify the principal
	if st.Globals.ExecutorVersion.V2VandenbergEnabled() {
		if _, ok := protocol.ParsePartitionUrl(st.OriginUrl); !ok ||
			!st.OriginUrl.PathEqual("") {
			return nil, errors.BadRequest.WithFormat("invalid principal: want partition, got %v", st.OriginUrl)
		}
	} else {
		if !st.NodeUrl().Equal(st.OriginUrl) {
			return nil, errors.BadRequest.WithFormat("invalid principal: want %v, got %v", st.NodeUrl(), st.OriginUrl)
		}
	}

	// Don't check the version number now because that could cause issues during
	// an update

	return body, nil
}

func (x ActivateProtocolVersion) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Verify we're executing on the right node
	if !st.NodeUrl().Equal(st.OriginUrl) {
		return nil, errors.BadRequest.WithFormat("invalid principal: want %v, got %v", st.NodeUrl(), st.OriginUrl)
	}

	// Verify the version number is a recognized version (vNext is not a real
	// version)
	if v := new(protocol.ExecutorVersion); !v.SetEnumValue(body.Version.GetEnumValue()) ||
		body.Version == protocol.ExecutorVersionVNext && !IsRunningTests() {
		return nil, errors.BadRequest.WithFormat("%d is not a recognized version number", body.Version)
	}

	// Load the system ledger
	var ledger *protocol.SystemLedger
	err = st.batch.Account(st.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load ledger: %w", err)
	}

	// Verify the version number is higher than the current number
	if body.Version < ledger.ExecutorVersion {
		return nil, errors.BadRequest.WithFormat("new version (%d) < old version (%d)", body.Version, ledger.ExecutorVersion)
	}

	// Update the version number
	ledger.ExecutorVersion = body.Version
	err = st.Update(ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store ledger: %w", err)
	}

	return nil, nil
}
