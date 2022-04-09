package protocol

import (
	"crypto/sha256"
)

// Hash calculates the hash of the transaction as H(H(header) + H(body)).
func (t *Transaction) GetHash() []byte {
	// Already computed?
	if t.hash != nil {
		return t.hash
	}

	if remote, ok := t.Body.(*RemoteTransaction); ok {
		// For a remote transaction, use the contained hash (if one is provided)
		if remote.Hash == [32]byte{} {
			return nil
		}
		return remote.Hash[:]
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

// Type decodes the transaction type from the body.
func (t *Transaction) Type() TransactionType {
	return t.Body.Type()
}
