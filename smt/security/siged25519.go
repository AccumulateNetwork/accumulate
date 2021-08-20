package security

import (
	"bytes"

	"github.com/AccumulateNetwork/SMT/smt/storage"
	"golang.org/x/crypto/ed25519"
)

type SigEd25519 struct {
	publicKey []byte // Ed25519 public key, 32 bytes
	sig       []byte // Signature, 64 bytes
}

// Type
// Returns the type for an Ed25519 signature
func (s *SigEd25519) Type() int {
	return SEd25519
}

// Signature
// Returns the bytes holding the signature
func (s *SigEd25519) Signature() []byte {
	return s.sig
}

// Sign
// Signs the given message, and populates the signature in the Sig
func (s *SigEd25519) Sign(msg []byte) {
	s.sig = ed25519.Sign(s.publicKey, msg)
}

// PublicKey
// Returns public key for the signature.  We can in many cases eliminate
// the public key from the signature, but for now we will include it.
func (s *SigEd25519) PublicKey() []byte {
	return s.publicKey
}

// Marshal
// Marshals the Signature
func (s *SigEd25519) Marshal() (data []byte) {
	data = storage.Int64Bytes(int64(SEd25519)) // Note that this function is a varint, so overhead is 1 byte generally
	data = append(data, s.publicKey...)        // Add the public Key
	data = append(data, s.sig...)              // Add the signature.  Must be 64 bytes
	return data
}

// Unmarshal
// UnMarshals the data stream into a SigEd25519
// Note that the type byte is assumed to be present, and a panic is thrown
// if the the byte isn't present, or the data stream is not sufficent to hold
// the Ed25519 signature
func (s *SigEd25519) Unmarshal(data []byte) []byte {
	emsg1 := "not enough data provided to encode a signature "                 // First data check
	emsg2 := "not an Ed25519 signature"                                        // Type check error
	emsg3 := "not enough data provided to encode the public key and signature" // Second data check
	var sigType int64                                                          // place to put our type
	if len(data) < 64 {                                                        // Sort that we have data for the type
		panic(emsg1) //                                                              and complain if we don't
	}
	sigType, data = storage.BytesInt64(data) //                                   Get the type from the data stream
	if int(sigType) != SEd25519 {            //                                   It needs to be us.
		panic(emsg2) //                                                           Panic if it isn't right
	}
	if len(data) < 32+64 { //                                                     Now that varInt is gone, check that
		panic(emsg3) //                                                             there's room for the public key
	} //                                                                            and the signature
	s.publicKey, data = append(s.publicKey[:0], data[:32]...), data[32:] //         First 32 bytes is the public key
	s.sig, data = append(s.sig[:0], data[:64]...), data[64:]             //         followed by 64 byte sig.

	return data // We are all done.
}

// Verify
// Verify that the signature is valid for the given message
func (s *SigEd25519) Verify(_ *MultiSig, _ int, msgHash [32]byte) bool {
	b := ed25519.Verify(s.publicKey, msgHash[:], s.sig) // Validate the signature
	return b                                            // Return the result
}

// Equal
// compare two Sig instances and return true if they are the same
func (s *SigEd25519) Equal(sig Sig) bool {
	switch { //                                            Sort to make sure all fields are the same
	case sig.Type() != SEd25519: //                        Type must be the same
		return false //
	case !bytes.Equal(s.publicKey, sig.PublicKey()): //    Public Key must be the same
		return false //
	case !bytes.Equal(s.sig, sig.Signature()): //          Signature must be the same
		return false //
	}
	return true //                                         All is the same, it is equal
}
