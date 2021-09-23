package transactions

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/SMT/common"
)

// ED25519Sig
// Implements signing and validating ED25519 signatures
type ED25519Sig struct {
	Nonce     uint64 // Nonce of Signature
	PublicKey []byte // 32 byte public key
	Signature []byte // a set of 64 byte signatures
}

// Equal
// Return true if the given Signature has the same Nonce, PublicKey,
// and Signature
func (e *ED25519Sig) Equal(e2 *ED25519Sig) bool {
	return bytes.Equal(e.PublicKey, e2.PublicKey) && //  the publickey is the same and
		bytes.Equal(e.Signature, e2.Signature) //        the signature is the same
}

// Sign
// Returns the signature for the given message.  What happens is the message
// is hashed with sha256, then the hash is signed.  The signature of the hash
// is returned.
func (e *ED25519Sig) Sign(nonce uint64, privateKey []byte, hash []byte) error {
	e.Nonce = nonce
	nHash := append(common.Uint64Bytes(nonce), hash...) //       Add nonce to hash
	s := ed25519.Sign(privateKey, nHash)                //       Sign the nonce+hash
	e.PublicKey = append([]byte{}, privateKey[32:]...)  //       Note that the last 32 bytes of a private key is the
	if !bytes.Equal(e.PublicKey, privateKey[32:]) {     //       Check that we have the proper keys to sign
		return errors.New("privateKey cannot sign this struct") // Return error that the doesn't match
	}
	e.Signature = s // public key.  Save the signature. // Save away the signature.
	return nil
}

// CanVerify
// will check to see if the keyHash of the public key, matches the key
// hash of the key that is part of this transaction. This is useful for
// situations where we need to see if the sender has permission to sign
// the transaction.
func (e *ED25519Sig) CanVerify(keyHash []byte) bool {
	if keyHash == nil { //if no key hash is provided assume the caller
		return true
	}
	pubKeyHash := sha256.Sum256(e.PublicKey)
	return bytes.Compare(keyHash, pubKeyHash[:]) == 0
}

// Verify
// Returns true if the signature matches the message.  Involves a hash of the
// message.
func (e *ED25519Sig) Verify(hash []byte) bool {
	return ed25519.Verify( //                               ed25519 verify signature
		e.PublicKey, //                                     with the public key of the signature
		append(common.Uint64Bytes(e.Nonce), hash...), //    Of the nonce+transaction hash
		e.Signature) //                                     with the signature
}

// Marshal
// Marshal a signature.  The data can be unmarshaled
func (e *ED25519Sig) Marshal() (data []byte, err error) { //

	defer func() { //                                                   On any error, just report the error
		if err := recover(); err != nil { //                            Check for error on exist
			err = fmt.Errorf("error marshaling ED25519Sig %v", err) //  Generate the error message
		} //
	}() //                                                              If no error occurs, err will be nil

	if len(e.PublicKey) != 32 || len(e.Signature) != 64 { //            Double check data sizes
		return nil, fmt.Errorf("poorly formed signature") //            Report error if sizes are wrong
	} //
	data = append(data, common.Uint64Bytes(e.Nonce)...) //              Add Nonce
	data = append(data, e.PublicKey...)                 //              Add Public Key
	data = append(data, e.Signature...)                 //              Add Signature
	return data, nil                                    //              Return the bytes
}

// Unmarshal
// UnMarshal a signature
// further unmarshalling can be done with the returned data
func (e *ED25519Sig) Unmarshal(data []byte) (nextData []byte, err error) {
	defer func() {
		if err := recover(); err != nil {
			err = fmt.Errorf("error unmarshaling ED25519Sig %v", err)
		}
	}()
	e.Nonce, data = common.BytesUint64(data)
	e.PublicKey = append([]byte{}, data[:32]...)
	data = data[32:]
	e.Signature = append([]byte{}, data[:64]...)
	data = data[64:]
	return data, nil
}
