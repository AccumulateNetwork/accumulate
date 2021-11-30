package transactions

// ED25519Sig
// Implements signing and validating ED25519 signatures
type Signature interface {
	GetNonce() uint64                                           // The nonce used by this signature
	GetPublicKey() []byte                                       // 32 byte public key
	GetSignature() []byte                                       // a set of 64 byte signatures
	Equal(s2 Signature) bool                                    //
	Sign(nonce uint64, privateKey []byte, msghash []byte) error // sign the msghash with the nonce and privateKey
	CanVerify(keyHash []byte) bool                              // Verify signature meets a KeyPage public key hash
	Verify(hash []byte) bool                                    // Verify signature verifies a message hash
	Marshal() (data []byte, err error)                          // Marshals a Signature
	Unmarshal(data []byte) (nextData []byte, err error)         // Marshals a Signature
}
