// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SetLiteAccountDelegate struct{}

func (SetLiteAccountDelegate) Type() protocol.TransactionType {
	return protocol.TransactionTypeSetLiteAccountDelegate
}

func (x SetLiteAccountDelegate) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (SetLiteAccountDelegate) check(st *StateManager, tx *Delivery) (*protocol.SetLiteAccountDelegate, error) {
	body, ok := tx.Transaction.Body.(*protocol.SetLiteAccountDelegate)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SetLiteAccountDelegate), tx.Transaction.Body)
	}

	// If no delegate is set (clearing delegation), no proof is needed
	if body.Delegate == nil {
		return body, nil
	}

	// If no proof is provided, that's OK - we'll validate locally in Execute
	if body.Proof == nil {
		return body, nil
	}

	// Check the proof for missing fields and validity
	proof := body.Proof
	if proof.KeyBook == nil {
		return nil, errors.BadRequest.WithFormat("invalid proof: missing KeyBook")
	}
	if proof.Receipt == nil {
		return nil, errors.BadRequest.WithFormat("invalid proof: missing receipt")
	}
	if !proof.Receipt.Validate(nil) {
		return nil, errors.BadRequest.WithFormat("proof is invalid")
	}

	// Check that the KeyBook URL matches the delegate URL
	if !body.Delegate.Equal(proof.KeyBook.Url) {
		return nil, errors.BadRequest.With("invalid proof: KeyBook URL does not match delegate URL")
	}

	// Check the state hash
	b, err := proof.KeyBook.MarshalBinary()
	if err != nil {
		return nil, errors.InternalError.WithFormat("marshal proof state: %v", err)
	}

	hash := sha256.Sum256(b)
	if !bytes.Equal(proof.Receipt.Start, hash[:]) {
		return nil, errors.BadRequest.WithFormat("invalid proof: state hash does not match proof start")
	}

	return body, nil
}

func (x SetLiteAccountDelegate) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Verify principal is a lite token account
	if _, ok := st.Origin.(*protocol.LiteTokenAccount); !ok {
		return nil, errors.BadRequest.WithFormat(
			"SetLiteAccountDelegate requires a lite token account, got %v",
			st.Origin.Type())
	}

	// If setting a delegate, verify target is a valid authority (KeyBook)
	if body.Delegate != nil {
		err = verifyKeyBookProof(st.DescribeShim, st.batch, tx.Transaction.Header.Principal, body)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Update the account's delegate field
	st.Origin.(*protocol.LiteTokenAccount).Delegate = body.Delegate

	err = st.Update(st.Origin)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store account state: %w", err)
	}

	return nil, nil
}

func verifyKeyBookProof(net execute.DescribeShim, batch *database.Batch, principal *url.URL, body *protocol.SetLiteAccountDelegate) error {
	// Try to load the delegate locally
	account, err := batch.Account(body.Delegate).Main().Get()
	switch {
	case err == nil:
		// Account exists locally - verify it's a KeyBook
		if account.Type() != protocol.AccountTypeKeyBook {
			return errors.BadRequest.WithFormat("invalid delegate %v: expected %v, got %v", body.Delegate, protocol.AccountTypeKeyBook, account.Type())
		}
		// Local KeyBook found, no proof needed
		return nil

	case errors.Is(err, errors.NotFound):
		// Account not found locally - require proof
		if body.Proof == nil {
			return errors.BadRequest.WithFormat("delegate %v not found locally: proof of existence required", body.Delegate)
		}

	default:
		return errors.UnknownError.WithFormat("load %v: %w", body.Delegate, err)
	}

	// Validate the proof
	// Check the anchor - TODO this will not work for the DN
	chain, err := batch.Account(net.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
	if err != nil {
		return errors.InternalError.WithFormat("load anchor pool for directory anchors: %w", err)
	}
	_, err = chain.HeightOf(body.Proof.Receipt.Anchor)
	if err != nil {
		code := errors.UnknownError
		if errors.Is(err, errors.NotFound) {
			code = errors.BadRequest
		}
		return code.WithFormat("invalid proof: lookup DN anchor %X: %w", body.Proof.Receipt.Anchor[:4], err)
	}

	return nil
}
