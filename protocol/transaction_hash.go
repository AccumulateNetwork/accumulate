package protocol

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func (t *Transaction) ID() *url.TxID {
	if t.Header.Principal == nil {
		return (&url.URL{Authority: Unknown}).WithTxID(*(*[32]byte)(t.GetHash()))
	}
	return t.Header.Principal.WithTxID(*(*[32]byte)(t.GetHash()))
}

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

func hashWriteData(withoutEntry TransactionBody, entry DataEntry) []byte {
	data, err := withoutEntry.MarshalBinary()
	if err != nil {
		panic(err) // This should be impossible
	}

	hasher := new(hash.Hasher)
	hasher.AddBytes(data)

	if entry == nil {
		var zero [32]byte
		hasher.AddHash(&zero)
	} else {
		hasher.AddHash((*[32]byte)(entry.Hash()))
	}

	return hasher.MerkleHash()
}

func (w *WriteData) GetHash() []byte {
	x := w.Copy()
	x.Entry = nil
	return hashWriteData(x, w.Entry)
}

func (w *WriteDataTo) GetHash() []byte {
	x := w.Copy()
	x.Entry = nil
	return hashWriteData(x, w.Entry)
}

func (w *SyntheticWriteData) GetHash() []byte {
	x := w.Copy()
	x.Entry = nil
	return hashWriteData(x, w.Entry)
}

func (w *SystemWriteData) GetHash() []byte {
	x := w.Copy()
	x.Entry = nil
	return hashWriteData(x, w.Entry)
}
