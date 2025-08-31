// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package interfaces contains all interface definitions for TDD development.
// This file is a template for creating new feature interfaces.
package interfaces

import (
	"context"
)

// FeatureService defines the interface for feature functionality.
// This interface should be implemented by the concrete type in internal/impl/feature/
type FeatureService interface {
	// MethodName description of what this method does
	MethodName(ctx context.Context, param1 string) (string, error)
	
	// MethodName2 description of what this method does  
	MethodName2(ctx context.Context, param1 string, param2 int) error
	
	// Close releases resources and shuts down the service
	Close() error
}

// FeatureRepository defines the interface for feature data access.
// This interface should be implemented by the concrete type in internal/impl/feature/
type FeatureRepository interface {
	// Create creates a new record
	Create(ctx context.Context, data interface{}) error
	
	// Get retrieves a record by ID
	Get(ctx context.Context, id string) (interface{}, error)
	
	// Update updates an existing record
	Update(ctx context.Context, id string, data interface{}) error
	
	// Delete removes a record
	Delete(ctx context.Context, id string) error
	
	// List returns a list of records
	List(ctx context.Context, opts ListOptions) ([]interface{}, error)
}

// ListOptions contains options for listing records
type ListOptions struct {
	Limit  int
	Offset int
	Filter map[string]interface{}
}

// FeatureConfig contains configuration for feature
type FeatureConfig struct {
	// Add configuration fields here
	MaxRetries    int
	Timeout       int64 // milliseconds
	EnableLogging bool
}

// FeatureMetrics defines metrics collection interface
type FeatureMetrics interface {
	// IncrementCounter increments a counter metric
	IncrementCounter(name string, tags map[string]string)
	
	// RecordHistogram records a histogram value
	RecordHistogram(name string, value float64, tags map[string]string)
	
	// SetGauge sets a gauge value
	SetGauge(name string, value float64, tags map[string]string)
}