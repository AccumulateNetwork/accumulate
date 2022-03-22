package protocol

import (
	"bytes"
	"crypto/sha256"
)

// GetTxHash returns the hash of the transaction.
//
// GetTxHash will panic if Transaction is nil and TxHash is nil or not a valid
// hash.
func (e *Envelope) GetTxHash() []byte {
	if len(e.TxHash) == sha256.Size {
		return e.TxHash
	}

	if e.Transaction != nil {
		return e.Transaction.GetHash()
	}

	if len(e.TxHash) == 0 {
		panic("both Transaction and TxHash are unspecified")
	}
	panic("invalid TxHash")
}

// EnvHash calculates the hash of the envelope as H(H(sig₀) + H(sig₁) + ... +
// H(txn)).
//
// EnvHash will panic if any of the signatures are not well formed or if
// Transaction is nil and TxHash is nil or not a valid hash.
func (e *Envelope) EnvHash() []byte {
	// Already computed?
	if e.hash != nil {
		return e.hash
	}

	// Marshal and hash the signatures
	hashes := make([]byte, 0, (len(e.Signatures)+1)*sha256.Size)
	for _, sig := range e.Signatures {
		data, err := sig.MarshalBinary()
		if err != nil {
			// Warn the user
			panic(err)
		}
		h := sha256.Sum256(data)
		hashes = append(hashes, h[:]...)
	}

	// Append the transaction hash
	hashes = append(hashes, e.GetTxHash()...)

	// Hash!
	h := sha256.Sum256(hashes)
	e.hash = h[:]
	return h[:]
}

// VerifyTxHash verifies that TxHash matches the hash of the transaction.
func (e *Envelope) VerifyTxHash() bool {
	switch {
	case e.TxHash == nil || e.Transaction == nil:
		// If neither are specified, it cannot be invalid
		return true
	case e.TxHash != nil && e.Transaction != nil:
		return bytes.Equal(e.TxHash, e.Transaction.GetHash())
	default:
		// If only one is specified, it cannot be invalid
		return true
	}
}

// Hash calculates the hash of the transaction as H(H(header) + H(body)).
func (t *Transaction) GetHash() []byte {
	// Already computed?
	if t.hash != nil {
		return t.hash
	}

	if t.Body.Type() == TransactionTypeSignPending {
		// Do not use the hash for a signature transaction
		return nil
	}

	// Marshal the header
	header, err := t.Header.MarshalBinary()
	if err != nil {
		// TransactionHeader.MarshalBinary will never return an error, but better safe than sorry.
		panic(err)
	}

	body, err := t.Body.MarshalBinary()
	if err != nil {
		// TransactionPayload.MarshalBinary should never return an error, but better safe than sorry.
		panic(err)
	}

	// Calculate the hash
	h1 := sha256.Sum256(header)
	h2 := sha256.Sum256(body)
	header = make([]byte, sha256.Size*2)
	copy(header, h1[:])
	copy(header[sha256.Size:], h2[:])
	h := sha256.Sum256(header)
	t.hash = h[:]
	return h[:]
}

// TxType returns the type of the transaction body, or SignPending if the
// transaction body is nil.
func (e *Envelope) TxType() TransactionType {
	if e.Transaction != nil {
		return e.Transaction.Body.Type()
	}
	return TransactionTypeSignPending
}

// Verify verifies that the signatures are valid.
func (e *Envelope) Verify() bool {
	// Compute the transaction hash
	txid := e.GetTxHash()

	// Check each signature
	for _, v := range e.Signatures {
		if !v.Verify(txid) {
			return false
		}
	}

	return true
}
