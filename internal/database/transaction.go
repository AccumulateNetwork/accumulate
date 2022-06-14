package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (c *ChangeSet) Transaction(hash []byte) *Transaction {
	if len(hash) != 32 {
		panic("invalid hash: wrong length")
	}
	return getOrCreateMap(&c.transaction, record.Key{}.Append("Transaction", hash), func() *Transaction {
		v := new(Transaction)
		v.logger = c.logger
		v.store = c.store
		v.key = record.Key{}.Append("Transaction", hash)
		v.container = c
		return v
	})
}

func (t *Transaction) Signatures(signer *url.URL) SignatureSet {
	// ACME is the 'signer' for system signatures
	if protocol.AcmeUrl().Equal(signer) {
		return t.SystemSignatures()
	}

	key := t.key.Append("Signatures", signer)
	return getOrCreateMap(&t.signatures, key, func() *VersionedSignatureSet {
		return newVersionedSignatureSet(t, t.store, key, "transaction %[2]x signatures %[4]v")
	})
}

func (t *Transaction) AddSignature(signature protocol.Signature) error {
	if signature.Type().IsSystem() {
		return t.SystemSignatures().Add(signature)
	}
	return t.Signatures(signature.GetSigner()).Add(signature)
}

func (t *Transaction) addSigner(signer *url.URL) error {
	s, err := t.Signers().Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Update the initiator
	if len(s) == 0 {
		status, err := t.Status().Get()
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}

		status.Initiator = signer
		err = t.Status().Put(status)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	err = t.Signers().Add(signer)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// TODO Deprecated in favor of t.Signers()
	status, err := t.Status().Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	status.AddSigner(&protocol.UnknownSigner{Url: signer})
	err = t.Status().Put(status)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	return nil
}
