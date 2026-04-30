// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck
)

type ReleaseLockedOperation struct{}

var _ PrincipalValidator = (*ReleaseLockedOperation)(nil)
var _ SignerValidator = (*ReleaseLockedOperation)(nil)

func (ReleaseLockedOperation) Type() protocol.TransactionType {
	return protocol.TransactionTypeReleaseLockedOperation
}

func (ReleaseLockedOperation) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// Can create lite token accounts like SyntheticLockedDeposit
	key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	return key != nil
}

func (ReleaseLockedOperation) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	// For lite accounts, authorization comes from the lite identity
	key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	if key == nil {
		// Not a lite account, fall back to normal authorization
		return true, nil
	}

	// For lite accounts, any valid signature from the lite identity is sufficient
	return false, nil
}

func (ReleaseLockedOperation) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	// For lite accounts where the principal doesn't exist yet, check if we have a valid signature
	key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	if key == nil {
		// Not a lite account, fall back to normal authorization
		return false, true, nil
	}

	// Check if the lite identity exists and has signed
	liteIdUrl := transaction.Header.Principal.RootIdentity()
	var liteId *protocol.LiteIdentity
	err = batch.Account(liteIdUrl).Main().GetAs(&liteId)
	if err != nil {
		return false, false, errors.NotFound.WithFormat("lite identity not found: %w", err)
	}

	// The transaction is ready if it's been signed by the lite identity
	// (The signature validation happens elsewhere, we just check here that we can proceed)
	return true, false, nil
}

func (x ReleaseLockedOperation) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Validate only performs basic checks on the transaction body.
	// Database-dependent checks are done in Execute.
	_, err := x.validateBody(tx)
	return nil, err
}

func (ReleaseLockedOperation) validateBody(tx *Delivery) (*protocol.ReleaseLockedOperation, error) {
	body, ok := tx.Transaction.Body.(*protocol.ReleaseLockedOperation)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.ReleaseLockedOperation), tx.Transaction.Body)
	}

	if body.LockedTxID == nil {
		return nil, errors.BadRequest.With("locked transaction ID is required")
	}

	if len(body.Preimage) == 0 {
		return nil, errors.BadRequest.With("preimage is required")
	}

	if len(body.Preimage) > 256 {
		return nil, errors.BadRequest.With("preimage too large (max 256 bytes)")
	}

	return body, nil
}

func (ReleaseLockedOperation) check(st *StateManager, tx *Delivery) (*protocol.ReleaseLockedOperation, *protocol.SyntheticLockedDeposit, *protocol.TransactionStatus, error) {
	body, ok := tx.Transaction.Body.(*protocol.ReleaseLockedOperation)
	if !ok {
		return nil, nil, nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.ReleaseLockedOperation), tx.Transaction.Body)
	}

	if body.LockedTxID == nil {
		return nil, nil, nil, errors.BadRequest.With("locked transaction ID is required")
	}

	if len(body.Preimage) == 0 {
		return nil, nil, nil, errors.BadRequest.With("preimage is required")
	}

	if len(body.Preimage) > 256 {
		return nil, nil, nil, errors.BadRequest.With("preimage too large (max 256 bytes)")
	}

	// Load the locked deposit transaction status
	txStatus, err := st.batch.Transaction(body.LockedTxID.HashSlice()).Status().Get()
	if err != nil {
		return nil, nil, nil, errors.NotFound.WithFormat("locked transaction not found: %w", err)
	}

	// Check if already released by examining the result
	if txStatus.Result != nil {
		if result, ok := txStatus.Result.(*protocol.SyntheticLockedDepositResult); ok {
			if result.ReleaseTxID != nil {
				return nil, nil, nil, errors.BadRequest.With("locked deposit has already been released")
			}
		}
	}

	// Load the locked deposit transaction body
	var lockedMsg messaging.MessageWithTransaction
	err = st.batch.Message(body.LockedTxID.Hash()).Main().GetAs(&lockedMsg)
	if err != nil {
		return nil, nil, nil, errors.NotFound.WithFormat("load locked transaction: %w", err)
	}

	lockedTx := lockedMsg.GetTransaction()
	if lockedTx == nil {
		return nil, nil, nil, errors.NotFound.With("locked transaction body is nil")
	}

	lockedBody, ok := lockedTx.Body.(*protocol.SyntheticLockedDeposit)
	if !ok {
		return nil, nil, nil, errors.BadRequest.WithFormat("referenced transaction is not a locked deposit: got %T", lockedTx.Body)
	}

	// Verify principal matches locked deposit destination
	if !tx.Transaction.Header.Principal.Equal(lockedTx.Header.Principal) {
		return nil, nil, nil, errors.Unauthorized.With("release must be signed by locked deposit recipient")
	}

	// Verify preimage using the correct hash algorithm
	var computedHash []byte
	switch lockedBody.HashAlgorithm {
	case protocol.HashAlgorithmSHA256:
		h := sha256.Sum256(body.Preimage)
		computedHash = h[:]
	case protocol.HashAlgorithmSHA256D:
		h1 := sha256.Sum256(body.Preimage)
		h2 := sha256.Sum256(h1[:])
		computedHash = h2[:]
	case protocol.HashAlgorithmHASH160:
		h1 := sha256.Sum256(body.Preimage)
		h2 := ripemd160.New()
		h2.Write(h1[:])
		computedHash = h2.Sum(nil)
	default:
		return nil, nil, nil, errors.BadRequest.WithFormat("unsupported hash algorithm: %v", lockedBody.HashAlgorithm)
	}

	if !bytes.Equal(computedHash, lockedBody.Hash) {
		return nil, nil, nil, errors.Unauthenticated.With("preimage does not match hash")
	}

	// Check expiration using the system ledger timestamp
	var ledger *protocol.SystemLedger
	err = st.LoadUrlAs(st.NodeUrl(protocol.Ledger), &ledger)
	if err != nil {
		return nil, nil, nil, errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	if lockedBody.Expiration != nil && ledger.Timestamp.After(*lockedBody.Expiration) {
		return nil, nil, nil, errors.Expired.With("locked deposit has expired")
	}

	return body, lockedBody, txStatus, nil
}

func (x ReleaseLockedOperation) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, lockedBody, lockedTxStatus, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Credit tokens to recipient (same logic as SyntheticDepositTokens)
	var account protocol.AccountWithTokens
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteTokenAccount:
			account = origin
		case *protocol.TokenAccount:
			account = origin
		default:
			return nil, fmt.Errorf("invalid principal: want token account, got %v", st.Origin.Type())
		}
		if !account.GetTokenUrl().Equal(lockedBody.Token) {
			return nil, fmt.Errorf("token type mismatch: want %s, got %s", account.GetTokenUrl(), lockedBody.Token)
		}
	} else {
		// Create lite token account if needed
		keyHash, tok, err := protocol.ParseLiteTokenAddress(tx.Transaction.Header.Principal)
		if err != nil {
			return nil, fmt.Errorf("invalid lite token account URL: %v", err)
		}
		if keyHash == nil {
			return nil, errors.NotFound.WithFormat("could not find token account")
		}
		if !lockedBody.Token.Equal(tok) {
			return nil, fmt.Errorf("token URL does not match lite token account URL")
		}

		// Check account limit
		liteIdUrl := tx.Transaction.Header.Principal.RootIdentity()
		dir, err := st.batch.Account(liteIdUrl).Directory().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load directory index: %w", err)
		}
		if len(dir)+1 > int(st.Globals.Globals.Limits.IdentityAccounts) {
			return nil, errors.BadRequest.WithFormat("identity would have too many accounts")
		}

		// Create lite token account
		lite := new(protocol.LiteTokenAccount)
		lite.Url = tx.Transaction.Header.Principal
		lite.TokenUrl = lockedBody.Token
		account = lite

		var liteIdentity *protocol.LiteIdentity
		err = st.LoadUrlAs(liteIdUrl, &liteIdentity)
		switch {
		case err == nil:
			// OK
		case errors.Is(err, storage.ErrNotFound):
			liteIdentity = new(protocol.LiteIdentity)
			liteIdentity.Url = liteIdUrl
			err := st.Update(liteIdentity)
			if err != nil {
				return nil, fmt.Errorf("failed to update %v: %v", liteIdentity.GetUrl(), err)
			}
		default:
			return nil, err
		}

		err = st.AddDirectoryEntry(liteIdUrl, tx.Transaction.Header.Principal)
		if err != nil {
			return nil, fmt.Errorf("failed to add directory entries: %v", err)
		}
	}

	// Credit the tokens
	if !account.CreditTokens(&lockedBody.Amount) {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	// Create or update the account depending on whether it existed
	if st.Origin != nil {
		err = st.Update(account)
		if err != nil {
			return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
		}
	} else {
		err = st.Create(account)
		if err != nil {
			return nil, fmt.Errorf("failed to create %v: %v", account.GetUrl(), err)
		}
	}

	// Mark the locked deposit as released by updating its result
	lockedTxStatus.Result = &protocol.SyntheticLockedDepositResult{
		ReleaseTxID: tx.Transaction.ID(),
	}
	err = st.batch.Transaction(body.LockedTxID.HashSlice()).Status().Put(lockedTxStatus)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("update locked tx status: %w", err)
	}

	// Return result with preimage for cross-chain extraction
	result := &protocol.ReleaseLockedOperationResult{
		Preimage:      body.Preimage,
		HashAlgorithm: lockedBody.HashAlgorithm,
		Hash:          lockedBody.Hash,
		Amount:        lockedBody.Amount,
		Token:         lockedBody.Token,
	}
	return result, nil
}
