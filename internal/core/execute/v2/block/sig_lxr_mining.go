// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LxrMiningSignatureValidator validates LXR mining signatures.
type LxrMiningSignatureValidator struct{}

// Type returns the signature type this validator can validate.
func (LxrMiningSignatureValidator) Type() protocol.SignatureType {
	return protocol.SignatureTypeLXRMining
}

// Validate validates an LXR mining signature.
func (LxrMiningSignatureValidator) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	sig, ok := ctx.signature.(*protocol.LxrMiningSignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("expected LxrMiningSignature, got %T", ctx.signature)
	}

	// Verify the LXR mining signature
	err := verifyLxrMiningSignature(batch, ctx.transaction, sig, sigExecMetadata{})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Get the signer
	signer, err := getSigner(batch, sig.Signer, sig.SignerVersion)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Add the signature to the transaction
	entry := new(database.SignatureSetEntry)
	entry.Signature = sig
	entry.Signer = sig.Signer
	entry.SignerVersion = sig.SignerVersion
	entry.Timestamp = sig.Timestamp
	entry.TransactionHash = ctx.transaction.GetHash()

	err = addSignature(batch, ctx, signer, entry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return nil, nil
}

// getSigner gets a signer by URL and version.
func getSigner(batch *database.Batch, signerUrl *protocol.URL, version uint64) (protocol.Signer2, error) {
	// Get the account
	account, err := batch.Account(signerUrl).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("failed to load signer: %v", err)
	}

	// Check if the account is a signer
	signer, ok := account.(protocol.Signer2)
	if !ok {
		return nil, errors.UnknownError.WithFormat("account %v is not a signer", signerUrl)
	}

	// Check if the version matches
	if signer.GetVersion() != version {
		return nil, errors.UnknownError.WithFormat("signer version mismatch: got %d, want %d", version, signer.GetVersion())
	}

	return signer, nil
}

// verifyLxrMiningSignature verifies an LxrMiningSignature
func verifyLxrMiningSignature(batch *database.Batch, transaction *protocol.Transaction, signature *protocol.LxrMiningSignature, md sigExecMetadata) error {
	// Get the signer URL
	signerUrl := signature.Signer
	if signerUrl == nil {
		return protocol.ErrInvalidSignature.With("missing signer URL")
	}

	// Get the key page
	var keyPage *protocol.KeyPage
	err := batch.Account(signerUrl).Main().GetAs(&keyPage)
	if err != nil {
		return protocol.ErrInvalidSignature.WithFormat("failed to load key page: %v", err)
	}

	// Check if mining is enabled
	if !keyPage.MiningEnabled {
		return protocol.ErrInvalidSignature.With("mining is not enabled for this key page")
	}

	// Check if the signer version matches
	if signature.SignerVersion != keyPage.Version {
		return protocol.ErrInvalidSignature.WithFormat("signer version mismatch: got %d, want %d", signature.SignerVersion, keyPage.Version)
	}

	// Create a hasher
	hasher := lxr.NewHasher()

	// Verify the signature
	if !hasher.VerifySignature(signature, keyPage.MiningDifficulty) {
		return protocol.ErrInvalidSignature.With("invalid LXR mining signature")
	}

	return nil
}
