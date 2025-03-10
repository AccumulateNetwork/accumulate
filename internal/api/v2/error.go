// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var ErrInvalidUrl = errors.BadRequest.With("invalid URL")

// General Errors
const (
	ErrCodeInternal = -32800 - iota
	ErrCodeDispatch
	ErrCodeValidation
	ErrCodeSubmission
	ErrCodeAccumulate
	ErrCodeNotLiteAccount
	ErrCodeNotAcmeAccount
	ErrCodeNotFound
	ErrCodeCanceled
)

// Metrics errors
const (
	ErrCodeMetricsQuery = -32900 - iota
	ErrCodeMetricsNotAVector
	ErrCodeMetricsVectorEmpty
)

// Custom errors
const (
	ErrCodeProtocolBase = -33000 - iota
)

var (
	ErrInternal           = jsonrpc2.NewError(ErrCodeInternal, "Internal Error", "An internal error occurred")
	ErrCanceled           = jsonrpc2.NewError(ErrCodeCanceled, "Canceled", "The request was canceled")
	ErrMetricsNotAVector  = jsonrpc2.NewError(ErrCodeMetricsNotAVector, "Metrics Query Error", "response is not a vector")
	ErrMetricsVectorEmpty = jsonrpc2.NewError(ErrCodeMetricsVectorEmpty, "Metrics Query Error", "response vector is empty")
)

func validatorError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeValidation, "Validation Error", err)
}

func accumulateError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Accumulate Error", "Not Found")
	}

	var perr *errors.Error
	if errors.As(err, &perr) {
		return jsonrpc2.NewError(ErrCodeProtocolBase-jsonrpc2.ErrorCode(perr.Code), "Accumulate Error", perr.Message)
	}

	var jerr jsonrpc2.Error
	if errors.As(err, &jerr) {
		return jerr
	}

	return jsonrpc2.NewError(ErrCodeAccumulate, "Accumulate Error", err)
}
