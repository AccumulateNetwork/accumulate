// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

// ExecutorVersion is an executor version number.
type ExecutorVersion uint64

// SignatureAnchoringEnabled checks if the version is at least V1 signature anchoring.
func (v ExecutorVersion) SignatureAnchoringEnabled() bool {
	return v >= ExecutorVersionV1SignatureAnchoring
}
