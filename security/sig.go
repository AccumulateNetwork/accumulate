package security

import "github.com/AccumulateNetwork/SMT/storage"

const (
	SEd25519 = 1
	Secdsa   = 2
)

type Sig interface {
	Type() int                                           // Returns the type implementing the interface
	Signature() []byte                                   // Returns the signature
	Marshal() []byte                                     // Marshals the object implementing Sig into a slice
	PublicKey() []byte                                   // Returns the public key
	Unmarshal(data []byte) []byte                        // Sets object implementing Sig to state defined by byte slice
	Verify(ms *MultiSig, idx int, msgHash [32]byte) bool // Validates message. True if message is validated by signature
	Equal(sig Sig) bool                                  // Returns true if two signatures are equal
}

// Unmarshal
// detects the type of sig in the data stream, then allocates a sig
// and Unmarshal() its state into the new sig.  Returns the sig and the updated
// data slice
func Unmarshal(data []byte) (sig Sig, newData []byte) {
	sigType, _ := storage.BytesInt64(data)
	switch sigType {
	case SEd25519:
		sig = new(SigEd25519)
	default:
		panic("unknown sig type")
	}
	data = sig.Unmarshal(data)
	return sig, data
}
