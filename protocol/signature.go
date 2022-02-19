package protocol

import (
	"encoding"
	"encoding/json"
)

type Signature interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler

	// Equal(s2 Signature) bool
	GetNonce() uint64                                           // The nonce used by this signature
	GetPublicKey() []byte                                       // 32 byte public key
	GetSignature() []byte                                       // a set of 64 byte signatures
	Sign(nonce uint64, privateKey []byte, msghash []byte) error // sign the msghash with the nonce and privateKey
	CanVerify(keyHash []byte) bool                              // Verify signature meets a KeyPage public key hash
	Verify(hash []byte) bool                                    // Verify signature verifies a message hash
}

func UnmarshalSignature(data []byte) (Signature, error) {
	ed := new(ED25519Sig)
	err := ed.UnmarshalBinary(data)
	return ed, err
}

func UnmarshalSignatureJSON(data []byte) (Signature, error) {
	ed := new(ED25519Sig)
	err := json.Unmarshal(data, ed)
	return ed, err
}
