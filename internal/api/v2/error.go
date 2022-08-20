package api

import (
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/getsentry/sentry-go"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

var ErrInvalidUrl = errors.StatusBadRequest.New("invalid URL")

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

//Custom errors
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

// func submissionError(err error) jsonrpc2.Error {
// 	return jsonrpc2.NewError(ErrCodeSubmission, "Submission Entry Error", err)
// }

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

func metricsQueryError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeMetricsQuery, "Metrics Query Error", err)
}

func internalError(err error) jsonrpc2.Error {
	// Capture internal errors but do not forward them to the user
	sentry.CaptureException(err)
	return ErrInternal
}
