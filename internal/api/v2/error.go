package api

import (
	"errors"

	"github.com/AccumulateNetwork/accumulated/smt/storage"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/getsentry/sentry-go"
)

var ErrInvalidUrl = errors.New("invalid URL")

// General Errors
const (
	ErrCodeInternal = -32800 - iota
	_               // maintain previous numbering
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

var (
	ErrInternal           = jsonrpc2.NewError(ErrCodeInternal, "Internal Error", "An internal error occured")
	ErrCanceled           = jsonrpc2.NewError(ErrCodeCanceled, "Canceled", "The request was canceled")
	ErrMetricsNotAVector  = jsonrpc2.NewError(ErrCodeMetricsNotAVector, "Metrics Query Error", "response is not a vector")
	ErrMetricsVectorEmpty = jsonrpc2.NewError(ErrCodeMetricsVectorEmpty, "Metrics Query Error", "response vector is empty")
)

func validatorError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeValidation, "Validation Error", err)
}

func submissionError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeSubmission, "Submission Entry Error", err)
}

func accumulateError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Accumulate Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeAccumulate, "Accumulate Error", err)
}

func metricsQueryError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeMetricsQuery, "Metrics Query Error", err)
}

func internalError(err error) jsonrpc2.Error {
	// Capture internal errors but do not forward them to the user
	sentry.CaptureException(err)
	return ErrInternal
}
