package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) ProcessRemoteSignatures(block *Block, delivery *chain.Delivery) error {
	var transactions []*protocol.SyntheticForwardTransaction
	index := map[[32]byte]int{}

	for _, signature := range delivery.Remote {
		var transaction *protocol.SyntheticForwardTransaction
		if i, ok := index[signature.Destination.AccountID32()]; ok {
			transaction = transactions[i]
		} else {
			transaction = new(protocol.SyntheticForwardTransaction)
			transaction.Transaction = delivery.Transaction
			index[signature.Destination.AccountID32()] = len(transactions)
			transactions = append(transactions, transaction)
			delivery.State.DidProduceTxn(signature.Destination, transaction)
		}

		transaction.Signatures = append(transaction.Signatures, *signature)
	}

	return nil
}

func (x *Executor) prepareToForward(delivery *chain.Delivery, state *ProcessSignatureState, signature protocol.Signature) (*protocol.ForwardedSignature, error) {
	// Do not process remote signatures for synthetic transactions
	if !delivery.Transaction.Body.Type().IsUser() {
		return nil, nil
	}

	fwd := new(protocol.ForwardedSignature)
	for i, signer := range state.Signers {
		if signer, ok := signer.(*protocol.UnknownSigner); ok {
			if signer.GetUrl().LocalTo(signature.RoutingLocation()) {
				// This should not happen
				return nil, fmt.Errorf("signer is local but unknown")
			}
			fwd.Destination = signer.GetUrl()
			fwd.Signers = state.Signers[:i]
			break
		}
	}
	if fwd.Destination != nil {
		// Forward to next signer
	} else if !delivery.Transaction.Header.Principal.LocalTo(signature.RoutingLocation()) {
		// Forward to principal
		fwd.Destination = delivery.Transaction.Header.Principal
		fwd.Signers = state.Signers
	} else {
		// No forwarding needeed
		return nil, nil
	}

	switch signature := signature.(type) {
	case *protocol.ForwardedSignature:
		fwd.Signature = signature.Signature
	case protocol.KeySignature:
		fwd.Signature = signature
	default:
		// This should not happen
		return nil, fmt.Errorf("unexpected remote signature that is not a key signature: %T", signature)
	}

	for i, signer := range fwd.Signers {
		fwd.Signers[i] = protocol.MakeLiteSigner(signer)
	}

	return fwd, nil
}
