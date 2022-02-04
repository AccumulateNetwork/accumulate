package protocol

import (
	"encoding"
	"encoding/json"
)

type Signature interface {
	BinarySize() int
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	Marshal() (data []byte, err error)                  // Marshals a Signature
	Unmarshal(data []byte) (nextData []byte, err error) // Marshals a Signature

	GetNonce() uint64                                           // The nonce used by this signature
	GetPublicKey() []byte                                       // 32 byte public key
	GetSignature() []byte                                       // a set of 64 byte signatures
	Equal(s2 Signature) bool                                    //
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
