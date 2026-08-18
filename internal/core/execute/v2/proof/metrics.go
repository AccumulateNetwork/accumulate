// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	proofMeter = otel.Meter("gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/proof")

	// Proof creation metrics
	proofIndividualCreated metric.Int64Counter
	proofCollectionCreated metric.Int64Counter
	proofGenerationTime    metric.Int64Histogram
	proofSize              metric.Int64Histogram

	// Proof validation metrics
	proofValidationAttempts  metric.Int64Counter
	proofValidationSuccesses metric.Int64Counter
	proofValidationErrors    metric.Int64Counter

	// Error metrics
	proofGenerationErrors   metric.Int64Counter
	proofValidationFailures metric.Int64Counter
)

func init() {
	var err error

	// Create counters
	proofIndividualCreated, err = proofMeter.Int64Counter("proof.individual_created",
		metric.WithDescription("Number of individual proofs created"),
		metric.WithUnit("{proof}"))
	if err != nil {
		panic(err)
	}

	proofCollectionCreated, err = proofMeter.Int64Counter("proof.collection_created",
		metric.WithDescription("Number of collection proofs created (batched)"),
		metric.WithUnit("{proof}"))
	if err != nil {
		panic(err)
	}

	proofValidationAttempts, err = proofMeter.Int64Counter("proof.validation_attempts",
		metric.WithDescription("Number of proof validation attempts"),
		metric.WithUnit("{attempt}"))
	if err != nil {
		panic(err)
	}

	proofValidationSuccesses, err = proofMeter.Int64Counter("proof.validation_successes",
		metric.WithDescription("Number of successful proof validations"),
		metric.WithUnit("{success}"))
	if err != nil {
		panic(err)
	}

	proofGenerationErrors, err = proofMeter.Int64Counter("proof.generation_errors",
		metric.WithDescription("Number of proof generation errors"),
		metric.WithUnit("{error}"))
	if err != nil {
		panic(err)
	}

	proofValidationErrors, err = proofMeter.Int64Counter("proof.validation_errors",
		metric.WithDescription("Number of proof validation errors"),
		metric.WithUnit("{error}"))
	if err != nil {
		panic(err)
	}

	// Create histograms for distribution data
	proofGenerationTime, err = proofMeter.Int64Histogram("proof.generation_time",
		metric.WithDescription("Time taken to generate proofs"),
		metric.WithUnit("ms"))
	if err != nil {
		panic(err)
	}

	proofSize, err = proofMeter.Int64Histogram("proof.size",
		metric.WithDescription("Size of generated proofs"),
		metric.WithUnit("By"))
	if err != nil {
		panic(err)
	}

	proofValidationFailures, err = proofMeter.Int64Counter("proof.validation_failures",
		metric.WithDescription("Number of failed proof validations"),
		metric.WithUnit("{failure}"))
	if err != nil {
		panic(err)
	}
}

// RecordProofCreated records when a proof is created
func RecordProofCreated(isCollection bool, generationTimeMs int64, proofSizeBytes int64) {
	if isCollection {
		proofCollectionCreated.Add(context.TODO(), 1)
	} else {
		proofIndividualCreated.Add(context.TODO(), 1)
	}
	proofGenerationTime.Record(context.TODO(), generationTimeMs)
	proofSize.Record(context.TODO(), proofSizeBytes)
}

// RecordProofValidation records a proof validation attempt
func RecordProofValidation(success bool) {
	proofValidationAttempts.Add(context.TODO(), 1)
	if success {
		proofValidationSuccesses.Add(context.TODO(), 1)
	} else {
		proofValidationFailures.Add(context.TODO(), 1)
	}
}

// RecordProofGenerationError records a proof generation error
func RecordProofGenerationError() {
	proofGenerationErrors.Add(context.TODO(), 1)
}

// RecordProofValidationError records a proof validation error
func RecordProofValidationError() {
	proofValidationErrors.Add(context.TODO(), 1)
}
