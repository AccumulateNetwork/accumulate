package protocol

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
)

// Hash calculates the hash of the transaction as H(H(header) + H(body)).
func (t *Transaction) GetHash() []byte {
	// Already computed?
	if t.hash != nil {
		return t.hash
	}

	if r, ok := t.Body.(*RemoteTransaction); ok {
		t.hash = r.Hash[:]
		return r.Hash[:]
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
		// TransactionPayload.MarshalBinary should never return an error, but
		// better a panic then a silently ignored error.
		panic(err)
	}

	hash := sha256.Sum256(data)
	return hash[:]
}

func (w *WriteData) GetHash() []byte {
	hasher := new(hash.Hasher)
	if w.Entry != nil {
		hasher.AddHash((*[32]byte)(w.Entry.Hash()))
	}
	if w.Scratch {
		hasher.AddBool(w.Scratch)
	}
	return hasher.MerkleHash()
}

func (w *WriteDataTo) GetHash() []byte {
	hasher := new(hash.Hasher)
	if w.Entry != nil {
		hasher.AddHash((*[32]byte)(w.Entry.Hash()))
	}
	hasher.AddUrl(w.Recipient)
	return hasher.MerkleHash()
}
