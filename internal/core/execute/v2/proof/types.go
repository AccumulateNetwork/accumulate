// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ProofType represents the type of cryptographic proof
type ProofType uint8

const (
	// ProofTypeTransaction represents a transaction proof
	ProofTypeTransaction ProofType = iota
	// ProofTypeCollection represents a collection proof (batched)
	ProofTypeCollection
)

// ProofRequest represents a request for a cryptographic proof
type ProofRequest struct {
	Account   *url.URL
	Anchor    *url.URL
	Sequence  uint64
	Root      [32]byte
	Timestamp time.Time
}

// ProofResponse contains the cryptographic proof and metadata
type ProofResponse struct {
	Proof     []byte
	Type      ProofType
	Root      [32]byte
	Anchor    *url.URL
	Sequence  uint64
	Timestamp time.Time
}

// ProofMetrics contains metrics about proof generation
type ProofMetrics struct {
	GenerationTime int64 // milliseconds
	ProofSize      int64 // bytes
	ErrorCount     int64
	SuccessCount   int64
}

// ProofBatch groups multiple proof requests for optimization
type ProofBatch struct {
	Requests   []*ProofRequest
	Destination *url.URL
	CreatedAt   time.Time
}

// ValidationResult contains the result of proof validation
type ValidationResult struct {
	Valid        bool
	ErrorMessage string
	ProofType    ProofType
	Timestamp    time.Time
}
