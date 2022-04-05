package block

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) ProcessRemoteSignatures(block *Block, transaction *protocol.Transaction, signatures []protocol.Signature) (*protocol.SyntheticForwardTransaction, error) {
	body := new(protocol.SyntheticForwardTransaction)
	body.Signatures = make([]protocol.ForwardedSignature, len(signatures))

	if transaction.Body.Type() == protocol.TransactionTypeRemote {
		body.TransactionHash = transaction.GetHash()
	} else {
		body.Transaction = transaction
	}

	for i, signature := range signatures {
		var signer protocol.Signer
		err := block.Batch.Account(signature.GetSigner()).GetStateAs(&signer)
		if err != nil {
			return nil, err
		}

		switch acct := signer.(type) {
		case *protocol.KeyPage:
			// Make a copy of the key page with no keys
			acct = acct.Copy()
			acct.Keys = nil
			signer = acct
		}

		body.Signatures[i] = protocol.ForwardedSignature{
			Signature: signature,
			Signer:    protocol.MakeLiteSigner(signer),
		}
	}

	return body, nil
}
