package protocol

import (
	"crypto/sha256"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
)

// SetHash sets the hash returned by GetHash. This will return an error if the
// body type is not Remote.
func (t *Transaction) SetHash(hash []byte) error {
	if t.Body.Type() != TransactionTypeRemote {
		return errors.New("cannot set the hash: not a remote transaction")
	}
	t.GetHash()
	t.hash = hash
	return nil
}

// Hash calculates the hash of the transaction as H(H(header) + H(body)).
func (t *Transaction) GetHash() []byte {
	// Already computed?
	if t.hash != nil {
		return t.hash
	}

	// Marshal the header
	header, err := t.Header.MarshalBinary()
	if err != nil {
		// TransactionHeader.MarshalBinary will never return an error, but better safe than sorry.
		panic(err)
	}
	headerHash := sha256.Sum256(header)

	// Calculate the hash
	sha := sha256.New()
	sha.Write(headerHash[:])
	sha.Write(t.getBodyHash())
	return sha.Sum(nil)
}

func (t *Transaction) getBodyHash() []byte {
	hasher, ok := t.Body.(interface{ GetHash() []byte })
	if ok {
		return hasher.GetHash()
	}

	data, err := t.Body.MarshalBinary()
	if err != nil {
		// TransactionPayload.MarshalBinary should never return an error, but better safe than sorry.
		panic(err)
	}

	hash := sha256.Sum256(data)
	return hash[:]
}

func (r *RemoteTransaction) GetHash() []byte {
	return r.Hash[:]
}

func (w *WriteData) GetHash() []byte {
	return w.Entry.Hash()
}

func (w *WriteDataTo) GetHash() []byte {
	hasher := new(hash.Hasher)
	hasher.AddHash((*[32]byte)(w.Entry.Hash()))
	hasher.AddUrl(w.Recipient)
	return hasher.MerkleHash()
}
