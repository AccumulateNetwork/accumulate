// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ExecutorProofContext provides proof generation capabilities integrated with the executor
type ExecutorProofContext struct {
	service      *Service
	partitionID  string
	logger       logging.Logger
	db           database.Beginner
}

// NewExecutorProofContext creates a new proof context for executor integration
func NewExecutorProofContext(db database.Beginner, partitionID string, logger logging.Logger) *ExecutorProofContext {
	return &ExecutorProofContext{
		service:     NewService(db, logger),
		partitionID: partitionID,
		logger:      logger,
		db:          db,
	}
}

// CreateCrossPartitionProof creates a proof for cross-partition validation
func (epc *ExecutorProofContext) CreateCrossPartitionProof(ctx context.Context, sourceAccount *url.URL,
	destPartition string, sequence uint64, root [32]byte) (*ProofResponse, error) {

	if sourceAccount == nil {
		return nil, fmt.Errorf("source account is required")
	}

	if destPartition == "" {
		return nil, fmt.Errorf("destination partition is required")
	}

	// Create anchor URL for the destination partition
	anchorUrl := protocol.PartitionUrl(destPartition)

	req := &ProofRequest{
		Account:   sourceAccount,
		Anchor:    anchorUrl,
		Sequence:  sequence,
		Root:      root,
	}

	proof, err := epc.service.CreateProof(ctx, req)
	if err != nil {
		RecordProofGenerationError()
		return nil, err
	}

	// Record the proof with OTel attributes for partition awareness
	RecordProofCreated(false, 0, int64(len(proof.Proof)))

	return proof, nil
}

// ValidatePartitionProof validates a proof from another partition
func (epc *ExecutorProofContext) ValidatePartitionProof(ctx context.Context, proof []byte,
	root [32]byte, sourcePartition string) (bool, error) {

	if len(proof) == 0 {
		return false, fmt.Errorf("proof is required")
	}

	if sourcePartition == "" {
		return false, fmt.Errorf("source partition is required")
	}

	anchorUrl := protocol.PartitionUrl(sourcePartition)

	result, err := epc.service.ValidateProof(ctx, proof, root, anchorUrl)
	if err != nil {
		RecordProofValidationError()
		return false, err
	}

	RecordProofValidation(result.Valid)
	return result.Valid, nil
}

// GetPartitionID returns the partition ID this context operates in
func (epc *ExecutorProofContext) GetPartitionID() string {
	return epc.partitionID
}

// Close closes the proof context and releases resources
func (epc *ExecutorProofContext) Close() error {
	return epc.service.Close()
}

// CrossPartitionProofValidator provides validation for proofs from other partitions
type CrossPartitionProofValidator struct {
	db       database.Beginner
	logger   logging.Logger
	contexts map[string]*ExecutorProofContext
}

// NewCrossPartitionProofValidator creates a new cross-partition proof validator
func NewCrossPartitionProofValidator(db database.Beginner, logger logging.Logger) *CrossPartitionProofValidator {
	return &CrossPartitionProofValidator{
		db:       db,
		logger:   logger,
		contexts: make(map[string]*ExecutorProofContext),
	}
}

// RegisterPartition registers a proof context for a partition
func (cpv *CrossPartitionProofValidator) RegisterPartition(partitionID string,
	proofContext *ExecutorProofContext) {
	if cpv.contexts == nil {
		cpv.contexts = make(map[string]*ExecutorProofContext)
	}
	cpv.contexts[partitionID] = proofContext
}

// ValidateProofFromPartition validates a proof that originated from another partition
func (cpv *CrossPartitionProofValidator) ValidateProofFromPartition(ctx context.Context,
	proof []byte, root [32]byte, sourcePartitionID string) (bool, error) {

	proofCtx, exists := cpv.contexts[sourcePartitionID]
	if !exists {
		return false, fmt.Errorf("partition context not registered: %s", sourcePartitionID)
	}

	return proofCtx.ValidatePartitionProof(ctx, proof, root, sourcePartitionID)
}
