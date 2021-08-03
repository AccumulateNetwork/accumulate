package security

type SigSpec interface {
	MatchSignature(signature []byte) Signature // Ensures that the provided signature matches this SigSpec
	Validate(signature, data []byte) bool      // Validates that the signature did sign the given data
	Marshal() []byte                           // Marshal a SigSpec into data
	UnMarshal([]byte)                          // UnMarshal a SigSpec into a SigSpec instance
	Hash() [32]byte                            // Return a hash of the SigSpec
}
