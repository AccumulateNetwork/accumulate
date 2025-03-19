// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
		return nil, fmt.Errorf("expected LxrMiningSignature, got %T", ctx.signature)
	}

	// Verify the LXR mining signature
	err := verifyLxrMiningSignature(batch, ctx.transaction, sig, sigExecMetadata{})
	if err != nil {
		return nil, fmt.Errorf("failed to verify LXR mining signature: %w", err)
	}

	// Get the signer
	signer, err := getSigner(batch, sig.Signer, sig.SignerVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get signer: %w", err)
	}

	// Convert Signer2 to a full Signer with credits functionality
	signerWithCredits := &signerAdapter{signer: signer}

	// Add the signature to the transaction
	entry := new(database.SignatureSetEntry)
	entry.Signature = sig
	entry.Signer = sig.Signer
	entry.SignerVersion = sig.SignerVersion
	entry.Timestamp = sig.Timestamp
	entry.TransactionHash = ctx.transaction.GetHash()

	err = addSignature(batch, ctx, signerWithCredits, entry)
	if err != nil {
		return nil, fmt.Errorf("failed to add signature: %w", err)
	}

	return nil, nil
}

// signerAdapter adapts a Signer2 to implement the full Signer interface
// by providing the missing credit-related methods
type signerAdapter struct {
	signer protocol.Signer2
}

func (s *signerAdapter) GetUrl() *url.URL {
	return s.signer.GetUrl()
}

func (s *signerAdapter) GetVersion() uint64 {
	return s.signer.GetVersion()
}

func (s *signerAdapter) GetSignatureThreshold() uint64 {
	return s.signer.GetSignatureThreshold()
}

func (s *signerAdapter) EntryByKey(key []byte) (int, protocol.KeyEntry, bool) {
	return s.signer.EntryByKey(key)
}

func (s *signerAdapter) EntryByKeyHash(keyHash []byte) (int, protocol.KeyEntry, bool) {
	return s.signer.EntryByKeyHash(keyHash)
}

func (s *signerAdapter) EntryByDelegate(owner *url.URL) (int, protocol.KeyEntry, bool) {
	return s.signer.EntryByDelegate(owner)
}

func (s *signerAdapter) GetCreditBalance() uint64 {
	// Mining signatures don't consume credits, so return a large value
	return 1_000_000_000
}

func (s *signerAdapter) CreditCredits(amount uint64) {
	// No-op for mining signatures
}

func (s *signerAdapter) DebitCredits(amount uint64) bool {
	// Always succeed for mining signatures
	return true
}

func (s *signerAdapter) CanDebitCredits(amount uint64) bool {
	// Always allow debiting for mining signatures
	return true
}

func (s *signerAdapter) GetAuthority() *url.URL {
	// For mining signatures, the signer itself is the authority
	return s.signer.GetUrl()
}

// CopyAsInterface implements protocol.Account
func (s *signerAdapter) CopyAsInterface() interface{} {
	return &signerAdapter{signer: s.signer}
}

// MarshalBinary implements encoding.BinaryMarshaler
func (s *signerAdapter) MarshalBinary() ([]byte, error) {
	// Since this is only an adapter used during signature verification,
	// we don't need to implement actual marshaling
	return nil, fmt.Errorf("signerAdapter does not support binary marshaling")
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (s *signerAdapter) UnmarshalBinary(data []byte) error {
	// Since this is only an adapter used during signature verification,
	// we don't need to implement actual unmarshaling
	return fmt.Errorf("signerAdapter does not support binary unmarshaling")
}

// UnmarshalBinaryFrom unmarshals from an io.Reader
func (s *signerAdapter) UnmarshalBinaryFrom(r io.Reader) error {
	// Since this is only an adapter used during signature verification,
	// we don't need to implement actual unmarshaling
	return fmt.Errorf("signerAdapter does not support binary unmarshaling")
}

// UnmarshalFieldsFrom unmarshals specific fields from an encoding.Reader
func (s *signerAdapter) UnmarshalFieldsFrom(r *encoding.Reader) error {
	// Since this is only an adapter used during signature verification,
	// we don't need to implement actual unmarshaling
	return fmt.Errorf("signerAdapter does not support field unmarshaling")
}

// StripUrl implements protocol.Account
func (s *signerAdapter) StripUrl() {}

// Type implements protocol.Account
func (s *signerAdapter) Type() protocol.AccountType {
	return protocol.AccountTypeKeyPage // Most signers are KeyPages, so use that as a default
}

// getSigner gets a signer by URL and version.
func getSigner(batch *database.Batch, signerUrl *url.URL, version uint64) (protocol.Signer2, error) {
	// Get the account
	account, err := batch.Account(signerUrl).Main().Get()
	if err != nil {
		return nil, fmt.Errorf("failed to load signer: %v", err)
	}

	// Check if the account is a signer
	signer, ok := account.(protocol.Signer2)
	if !ok {
		return nil, fmt.Errorf("account %v is not a signer", signerUrl)
	}

	// Check if the version matches
	if signer.GetVersion() != version {
		return nil, fmt.Errorf("signer version mismatch: got %d, want %d", version, signer.GetVersion())
	}

	return signer, nil
}

// verifyLxrMiningSignature verifies an LxrMiningSignature
func verifyLxrMiningSignature(batch *database.Batch, transaction *protocol.Transaction, signature *protocol.LxrMiningSignature, md sigExecMetadata) error {
	// Get the signer URL
	signerUrl := signature.Signer
	if signerUrl == nil {
		return fmt.Errorf("%w: missing signer URL", protocol.ErrInvalidSignature)
	}

	// Get the key page
	var keyPage *protocol.KeyPage
	err := batch.Account(signerUrl).Main().GetAs(&keyPage)
	if err != nil {
		return fmt.Errorf("%w: failed to load key page: %v", protocol.ErrInvalidSignature, err)
	}

	// Check if mining is enabled
	if !keyPage.MiningEnabled {
		return fmt.Errorf("%w: mining is not enabled for this key page", protocol.ErrInvalidSignature)
	}

	// Check if the signer version matches
	if signature.SignerVersion != keyPage.Version {
		return fmt.Errorf("%w: signer version mismatch: got %d, want %d", protocol.ErrInvalidSignature, signature.SignerVersion, keyPage.Version)
	}

	// Create a hasher
	hasher := lxr.NewHasher()

	// Verify the signature
	if !hasher.VerifySignature(signature, keyPage.MiningDifficulty) {
		return fmt.Errorf("%w: invalid LXR mining signature", protocol.ErrInvalidSignature)
	}

	return nil
}
