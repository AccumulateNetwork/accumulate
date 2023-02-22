// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignatureContext is the context in which a message is executed.
type SignatureContext struct {
	*MessageContext
	signature   protocol.Signature
	transaction *protocol.Transaction
}

func (s *SignatureContext) Type() protocol.SignatureType { return s.signature.Type() }

// getSigner gets the signature's signer, resolving a LTA to a LID.
func (s *SignatureContext) getSigner() *url.URL {
	signer := s.signature.GetSigner()
	if key, _ := protocol.ParseLiteIdentity(signer); key != nil {
		return signer
	} else if key, _, _ := protocol.ParseLiteTokenAddress(signer); key != nil {
		return signer.RootIdentity()
	} else {
		return signer
	}
}

// getAuthority gets the signature's signer's authority, resolving a LTA to a
// LID and a page to a book.
func (s *SignatureContext) getAuthority() *url.URL {
	signer := s.signature.GetSigner()
	if key, _ := protocol.ParseLiteIdentity(signer); key != nil {
		return signer
	} else if key, _, _ := protocol.ParseLiteTokenAddress(signer); key != nil {
		return signer.RootIdentity()
	} else {
		return signer.Identity()
	}
}

// authorityIsSatisfied verifies that the authority has voted on the transaction.
func (s *SignatureContext) authorityIsSatisfied(batch *database.Batch, authority *url.URL) (bool, error) {
	status, err := batch.Transaction(s.transaction.GetHash()).Status().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.AuthorityValidator](s.Executor, s.transaction.Body.Type())
	if ok {
		ready, fallback, err := val.AuthorityIsSatisfied(s.Executor, batch, s.transaction, status, authority)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return ready, nil
		}
	}

	ok, err = s.Executor.AuthorityIsSatisfied(batch, s.transaction, status, authority)
	return ok, errors.UnknownError.Wrap(err)
}

// didInitiate checks if this signature or its authority initiated the
// transaction.
func (s *SignatureContext) didInitiate(batch *database.Batch) (bool, error) {
	if protocol.SignatureDidInitiate(s.signature, s.transaction.Header.Initiator[:], nil) {
		return true, nil
	}

	status, err := batch.Transaction(s.transaction.GetHash()).Status().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load status: %w", err)
	}

	record := batch.Transaction(s.transaction.GetHash())
	for _, signer := range status.FindSigners(s.getAuthority()) {
		sigs, err := database.GetSignaturesForSigner(record, signer)
		if err != nil {
			return false, errors.UnknownError.WithFormat("load signatures: %w", err)
		}

		for _, sig := range sigs {
			if protocol.SignatureDidInitiate(sig, s.transaction.Header.Initiator[:], nil) {
				return true, nil
			}
		}
	}
	return false, nil
}
