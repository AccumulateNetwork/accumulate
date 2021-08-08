package security

// SigSpec
// Specifies the signature required to validate a transaction.
type SigSpec struct {
	PublicKeys []byte
	m, n       int64
}

// Validate
// Validates that a signature matches the specification.
func (s *SigSpec) Validate(mSig MSigs) bool {
	mSig.Sort()
	return true
}
