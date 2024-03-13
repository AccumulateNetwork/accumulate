// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
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

// SignerCanSignValidator validates that a signer is authorized to sign a
// transaction (e.g a key page's black list).
type SignerCanSignValidator interface {
	SignerCanSign(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) (fallback bool, err error)
}

// SignerValidator validates signatures for a specific type of transaction.
type SignerValidator interface {
	TransactionExecutor

	// AuthorityIsAccepted checks if the authority signature is accepted for the
	// transaction.
	AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error)

	// TransactionIsReady checks if the transaction is ready to be executed.
	TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error)
}

type AuthorityValidator interface {
	// AuthorityWillVote checks if the authority is ready to vote. MUST NOT
	// MODIFY STATE.
	AuthorityWillVote(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, authority *url.URL) (fallback bool, vote *AuthVote, err error)
}

type AuthVote struct {
	Source *url.URL
	Vote   protocol.VoteType
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
	// GetActiveGlobals returns the active network global values.
	GetActiveGlobals() *core.GlobalValues

	// GetAccountAuthoritySet returns the authority set of an account.
	GetAccountAuthoritySet(batch *database.Batch, account protocol.Account) (*protocol.AccountAuth, error)

	// GetMessageAs retrieves a signature by hash from the bundle or database.
	GetSignatureAs(batch *database.Batch, hash [32]byte) (protocol.Signature, error)

	// TransactionIsInitiated verifies the transaction has been paid for and the
	// initiator signature has been processed.
	TransactionIsInitiated(batch *database.Batch, transaction *protocol.Transaction) (bool, *messaging.CreditPayment, error)

	// SignerCanSign returns an error if the signer is not authorized to sign
	// the transaction (e.g. a key page's transaction blacklist).
	SignerCanSign(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) error

	// AuthorityDidVote verifies the authority is ready to send an authority
	// signature. For most transactions, this succeeds if at least one of the
	// authority's signers is satisfied.
	AuthorityDidVote(batch *database.Batch, transaction *protocol.Transaction, authUrl *url.URL) (bool, protocol.VoteType, error)

	// AuthorityWillVote verifies that an authority signature (aka vote)
	// approving the transaction has been received from the authority.
	AuthorityWillVote(batch *database.Batch, block uint64, transaction *protocol.Transaction, authUrl *url.URL) (*AuthVote, error)
}
