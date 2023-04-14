// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SignatureValidationMetadata struct {
	Location    *url.URL
	IsInitiator bool
	Delegated   bool
	Forwarded   bool
}

func (d SignatureValidationMetadata) SetDelegated() SignatureValidationMetadata {
	e := d
	e.Delegated = true
	return e
}

func (d SignatureValidationMetadata) SetForwarded() SignatureValidationMetadata {
	e := d
	e.Forwarded = true
	return e
}

func (d SignatureValidationMetadata) Nested() bool {
	return d.Delegated || d.Forwarded
}

// TransactionExecutor executes a specific type of transaction.
type TransactionExecutor interface {
	// Type is the transaction type the executor can execute.
	Type() protocol.TransactionType

	// Validate validates the transaction for acceptance.
	Validate(*StateManager, *Delivery) (protocol.TransactionResult, error)

	// Execute fully validates and executes the transaction.
	Execute(*StateManager, *Delivery) (protocol.TransactionResult, error)
}

// SignerValidator validates signatures for a specific type of transaction.
type SignerValidator interface {
	TransactionExecutor

	// SignerIsAuthorized checks if the signature is authorized for the
	// transaction.
	SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error)

	// TransactionIsReady checks if the transaction is ready to be executed.
	TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error)
}

type AuthorityValidator interface {
	AuthorityIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, authority *url.URL) (satisfied, fallback bool, err error)
}

// PrincipalValidator validates the principal for a specific type of transaction.
type PrincipalValidator interface {
	TransactionExecutor

	AllowMissingPrincipal(*protocol.Transaction) bool
}

// TransactionExecutorCleanup cleans up after a failed transaction.
type TransactionExecutorCleanup interface {
	// DidFail is called if the transaction failed.
	DidFail(*ProcessTransactionState, *protocol.Transaction) error
}

type AuthDelegate interface {
	GetAccountAuthoritySet(*database.Batch, protocol.Account) (*protocol.AccountAuth, error)

	// TransactionIsInitiated verifies the transaction has been paid for and the
	// initiator signature has been processed.
	TransactionIsInitiated(batch *database.Batch, transaction *protocol.Transaction) (bool, *messaging.CreditPayment, error)

	// SignerIsAuthorized verifies the signer is authorized to sign a given
	// transaction.
	SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) error

	// AuthorityIsSatisfied verifies the authority is ready to send an authority
	// signature. For most transactions, this succeeds if at least one of the
	// authority's signers is satisfied.
	AuthorityIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, authUrl *url.URL) (bool, error)

	// SignerIsSatisfied verifies the signer is satisfied. For most
	// transactions, this succeeds if the signer's conditions (thresholds) have
	// been met.
	SignerIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, signerUrl *url.URL) (bool, error)

	// AuthorityIsReady verifies that an authority signature (aka vote)
	// approving the transaction has been received from the authority.
	AuthorityIsReady(batch *database.Batch, transaction *protocol.Transaction, authUrl *url.URL) (bool, error)
}
