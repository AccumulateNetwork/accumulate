package protocol

// ExecutorVersion is an executor version number.
type ExecutorVersion uint64

// ExecutorVersionLatest is the latest version of the executor.
// ExecutorVersionLatest is intended primarily for testing.
const ExecutorVersionLatest = ExecutorVersionV1SignatureAnchoring

// SignatureAnchoringEnabled checks if the version is at least V1 signature anchoring.
func (v ExecutorVersion) SignatureAnchoringEnabled() bool {
	return v >= ExecutorVersionV1SignatureAnchoring
}

// DoubleHashEntriesEnabled checks if the version is at least V1 double-hash entries.
func (v ExecutorVersion) DoubleHashEntriesEnabled() bool {
	return v >= ExecutorVersionV1DoubleHashEntries
}
