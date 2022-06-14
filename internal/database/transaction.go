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

	return getOrCreateMap(&t.signatures, t.key.Append("Signatures", signer), func() *VersionedSignatureSet {
		return newVersionedSignatureSet(t.container, t.store, t.key, signer)
	})
}

func (t *Transaction) AddSignature(signature protocol.Signature) error {
	if signature.Type().IsSystem() {
		return t.SystemSignatures().Add(signature)
	}
	return t.Signatures(signature.GetSigner()).Add(signature)
}

func (t *Transaction) addSigners(signers []*url.URL) error {
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

		status.Initiator = signers[0]
		err = t.Status().Put(status)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	err = t.Signers().Add(signers...)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}
	return nil
}

func (t *Transaction) Commit() error {
	// Ensure the signer index is up to date
	var signers []*url.URL
	if t.systemSignatures != nil && t.systemSignatures.IsDirty() {
		// ACME is the 'signer' for system signatures
		signers = append(signers, protocol.AcmeUrl())
	}

	for _, set := range t.signatures {
		if set.IsDirty() {
			signers = append(signers, set.set.Key(3).(*url.URL))
		}
	}

	if len(signers) > 0 {
		err := t.addSigners(signers)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	// Do the normal commit stuff
	err := t.baseCommit()
	return errors.Wrap(errors.StatusUnknown, err)
}
