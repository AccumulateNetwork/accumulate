package security

// SigGroup
// Defines a set of SigSpecs.  The priorities of the SigSpecs is
// indicated by their position in the SigSpecs slice, where a lower
// index indicates a higher priority.
//
// SigGroups are persisted as a slice of hashes. On load, the SigSpecs are
// loaded for not MAST SigSpecs.  For MAST SigSpecs, transactions must provide
// a SigSpec that validates against the hash in the SigGroup
type SigGroup struct {
	SigSpecHashes [][]byte  // The hashes of the SigSpec stores them in the database (if known)
	SigSpecs      []SigSpec // The SigSpec of a hash if not a MAST SigSpec
}

// AddSigSpec
// Adds or replaces a SigSpec authorized by the SigSpec at index auth, and
// placed at the index dest.  Note that the destination cannot leave a nil
// entry in the SigSpecs slice.
func (s *SigGroup) AddSigSpec(auth, dest int, sigSpec SigSpec) error {
	_ = auth
	_ = dest
	_ = sigSpec
	return nil
}

// DeleteSigSpec
// Removes a SigSpec from the the SigSpec Slice
func (s *SigGroup) DeleteSigSpec(auth, dest int, sigSpec SigSpec) error {
	_, _, _ = auth, dest, sigSpec
	return nil
}
