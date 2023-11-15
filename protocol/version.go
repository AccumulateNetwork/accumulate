// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

// ExecutorVersion is an executor version number.
type ExecutorVersion uint64

// ExecutorVersionLatest is the latest version of the executor.
// ExecutorVersionLatest is intended primarily for testing.
const ExecutorVersionLatest = ExecutorVersionV2

// SignatureAnchoringEnabled checks if the version is at least V1 signature anchoring.
func (v ExecutorVersion) SignatureAnchoringEnabled() bool {
	return v >= ExecutorVersionV1SignatureAnchoring
}

// DoubleHashEntriesEnabled checks if the version is at least V1 double-hash entries.
func (v ExecutorVersion) DoubleHashEntriesEnabled() bool {
	return v >= ExecutorVersionV1DoubleHashEntries
}

// HaltV1 checks if the version is at least V1 halt.
func (v ExecutorVersion) HaltV1() bool {
	return v >= ExecutorVersionV1Halt
}

// V2Enabled checks if the version is at least V2Enabled.
func (v ExecutorVersion) V2Enabled() bool {
	return v >= ExecutorVersionV2
}
