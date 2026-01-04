// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	// EthereumDataSignature requires V2Tanegashima
	registerConditionalExec[EthereumDataSignatureExecutor](&signatureExecutors,
		func(ctx *SignatureContext) bool { return ctx.GetActiveGlobals().ExecutorVersion.V2TanegashimaEnabled() },
		protocol.SignatureTypeEthereumData,
	)
}

// EthereumDataSignatureExecutor processes EthereumDataSignature signatures.
// This signature type is used for self-authenticating writes where the
// signature is embedded in an EthereumDataEntry within the transaction body.
type EthereumDataSignatureExecutor struct{}

// ethereumDataSigContext collects all the bits of data needed to validate and
// execute an EthereumDataSignature.
type ethereumDataSigContext struct {
	*SignatureContext

	// sig is the EthereumDataSignature
	sig *protocol.EthereumDataSignature

	// signer is the derived lite identity
	signer protocol.Signer

	// entry is the EthereumDataEntry from the transaction body
	entry *protocol.EthereumDataEntry

	// isInitiator is true if the submitted signature is the transaction initiator
	isInitiator bool

	// fee is the full fee that will be charged when the signature is executed
	fee protocol.Fee
}

// Validate validates an EthereumDataSignature.
func (x EthereumDataSignatureExecutor) Validate(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	err := x.check(batch, &ethereumDataSigContext{SignatureContext: ctx})
	return nil, errors.UnknownError.Wrap(err)
}

// check validates the signature and collects all the pieces needed for execution.
func (x EthereumDataSignatureExecutor) check(batch *database.Batch, ctx *ethereumDataSigContext) error {
	var ok bool
	ctx.sig, ok = ctx.signature.(*protocol.EthereumDataSignature)
	if !ok {
		return errors.BadRequest.WithFormat("invalid signature: expected EthereumDataSignature, got %v", ctx.signature.Type())
	}

	// Verify the transaction is a WriteData transaction
	writeData, ok := ctx.transaction.Body.(*protocol.WriteData)
	if !ok {
		return errors.BadRequest.WithFormat("EthereumDataSignature can only be used with WriteData transactions, got %v", ctx.transaction.Body.Type())
	}

	// Find the EthereumDataEntry in the transaction
	ctx.entry = x.findEthereumDataEntry(writeData)
	if ctx.entry == nil {
		return errors.BadRequest.With("EthereumDataSignature requires an EthereumDataEntry in the transaction body")
	}

	// Verify the embedded signature and derive the signer
	signerUrl, err := protocol.VerifyEthereumDataSignature(ctx.entry, ctx.sig.ExpectedChainId)
	if err != nil {
		return errors.Unauthenticated.WithFormat("invalid embedded Ethereum signature: %w", err)
	}

	// The signature's signer must match the derived signer
	if ctx.sig.Signer == nil {
		// If signer is not set, populate it from the recovered address
		ctx.sig.Signer = signerUrl
	} else if !ctx.sig.Signer.Equal(signerUrl) {
		return errors.Unauthenticated.WithFormat("signer mismatch: signature claims %v but recovered %v", ctx.sig.Signer, signerUrl)
	}

	// Check routing
	partition, err := ctx.Executor.Router.RouteAccount(ctx.sig.Signer)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !strings.EqualFold(partition, ctx.Executor.Describe.PartitionId) {
		return errors.BadRequest.WithFormat("signature submitted to %v instead of %v", ctx.Executor.Describe.PartitionId, partition)
	}

	// Check if the signature initiates the transaction
	err = x.checkInit(ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Load the signer and verify
	err = x.verifySigner(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify the signer can pay
	err = x.verifyCanPay(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// findEthereumDataEntry finds the EthereumDataEntry in a WriteData transaction
func (EthereumDataSignatureExecutor) findEthereumDataEntry(writeData *protocol.WriteData) *protocol.EthereumDataEntry {
	if writeData.Entry == nil {
		return nil
	}
	entry, ok := writeData.Entry.(*protocol.EthereumDataEntry)
	if ok {
		return entry
	}
	return nil
}

func (EthereumDataSignatureExecutor) checkInit(ctx *ethereumDataSigContext) error {
	ctx.isInitiator, _ = protocol.SignatureDidInitiate(ctx.sig, ctx.transaction.Header.Initiator[:], nil)

	switch {
	case ctx.isInitiator && ctx.sig.GetTimestamp() == 0:
		// The initiator must have a timestamp
		return errors.BadTimestamp.WithFormat("initial signature does not have a timestamp")

	case ctx.isInitiator && ctx.sig.GetVote() != protocol.VoteTypeAccept:
		return errors.BadRequest.WithFormat("initial signature cannot be a %v vote", ctx.sig.GetVote())
	}

	return nil
}

// verifySigner loads the signer and checks it against the signature.
func (EthereumDataSignatureExecutor) verifySigner(batch *database.Batch, ctx *ethereumDataSigContext) error {
	// Convert lite token address to lite identity
	signerUrl := ctx.sig.Signer
	if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
		signerUrl = signerUrl.RootIdentity()
	}

	var err error
	ctx.signer, err = loadSigner(batch, signerUrl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Check the signer version
	if ctx.transaction.Body.Type().IsUser() && ctx.sig.GetSignerVersion() != ctx.signer.GetVersion() {
		return errors.BadSignerVersion.WithFormat("invalid version: have %d, got %d", ctx.signer.GetVersion(), ctx.sig.GetSignerVersion())
	}

	return nil
}

// verifyCanPay verifies the signer can be charged for recording the signature.
func (EthereumDataSignatureExecutor) verifyCanPay(batch *database.Batch, ctx *ethereumDataSigContext) error {
	// Operators don't have to pay when signing directly with the operators page
	if protocol.DnUrl().LocalTo(ctx.signer.GetUrl()) {
		return nil
	}

	// Check for errors, such as payload is too big
	var err error
	ctx.fee, err = ctx.GetActiveGlobals().Globals.FeeSchedule.ComputeSignatureFee(ctx.sig)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Only charge the transaction fee for the initial signature
	if ctx.isInitiator {
		txnFee, err := ctx.GetActiveGlobals().Globals.FeeSchedule.ComputeTransactionFee(ctx.transaction)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		ctx.fee += txnFee - protocol.FeeSignature
	}

	// Verify the signer has at least 0.01 credits
	minFee := protocol.FeeSignature.GetEnumValue()
	if !ctx.signer.CanDebitCredits(minFee) {
		return errors.InsufficientCredits.WithFormat(
			"insufficient credits: have %s, want %s",
			protocol.FormatAmount(ctx.signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(minFee, protocol.CreditPrecisionPower))
	}
	return nil
}

// Process processes an EthereumDataSignature.
func (x EthereumDataSignatureExecutor) Process(batch *database.Batch, ctx *SignatureContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Process the signature
	ctx2 := &ethereumDataSigContext{SignatureContext: ctx}
	err = x.check(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = x.process(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Send signature requests
	err = x.sendSignatureRequests(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Send the credit payment
	err = x.sendCreditPayment(batch, ctx2)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Send the authority signature if the authority is ready
	err = ctx.maybeSendAuthoritySignature(batch, &protocol.AuthoritySignature{
		Authority: ctx2.sig.Signer.RootIdentity(),
	})
	return nil, errors.UnknownError.Wrap(err)
}

// process processes the signature.
func (EthereumDataSignatureExecutor) process(batch *database.Batch, ctx *ethereumDataSigContext) error {
	// Charge the fee
	if !ctx.signer.DebitCredits(ctx.fee.AsUInt64()) {
		_ = ctx.signer.DebitCredits(protocol.FeeSignature.AsUInt64())

		return errors.InsufficientCredits.WithFormat("%v has insufficient credits: have %s, want %s", ctx.signer.GetUrl(),
			protocol.FormatAmount(ctx.signer.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(ctx.fee.AsUInt64(), protocol.CreditPrecisionPower))
	}

	// Store changes to the signer
	err := batch.Account(ctx.signer.GetUrl()).Main().Put(ctx.signer)
	if err != nil {
		return errors.UnknownError.WithFormat("store signer: %w", err)
	}

	// Add to the signature set and chain
	// For EthereumDataSignature, we use keyIndex 0 since there's no key page
	err = addSignature(batch, ctx.SignatureContext, ctx.signer, &database.SignatureSetEntry{
		KeyIndex: 0,
		Version:  ctx.sig.GetSignerVersion(),
		Hash:     ctx.message.Hash(),
		Path:     nil,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// sendSignatureRequests sends signature requests so that the transaction
// will appear on the appropriate pending lists.
func (EthereumDataSignatureExecutor) sendSignatureRequests(batch *database.Batch, ctx *ethereumDataSigContext) error {
	if !ctx.isInitiator {
		return nil
	}

	// Send a notice to the principal
	msg := new(messaging.SignatureRequest)
	msg.Authority = ctx.transaction.Header.Principal
	msg.Cause = ctx.message.ID()
	msg.TxID = ctx.transaction.ID()
	err := ctx.didProduce(batch, msg.Authority, msg)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// sendCreditPayment sends the principal a notice that the signer paid.
func (EthereumDataSignatureExecutor) sendCreditPayment(batch *database.Batch, ctx *ethereumDataSigContext) error {
	if !ctx.isInitiator {
		return nil
	}

	return ctx.didProduce(
		batch,
		ctx.transaction.Header.Principal,
		&messaging.CreditPayment{
			Paid:      ctx.fee,
			Payer:     ctx.sig.Signer.RootIdentity(),
			TxID:      ctx.transaction.ID(),
			Cause:     ctx.message.ID(),
			Initiator: ctx.isInitiator,
		},
	)
}
