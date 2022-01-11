package api

import (
	"errors"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// General Errors
const (
	ErrCodeInternal = -32800 - iota
	ErrCodeDuplicateTxn
	ErrCodeValidation
	ErrCodeSubmission
	ErrCodeAccumulate
	ErrCodeNotLiteAccount
	ErrCodeNotAcmeAccount
	ErrCodeNotFound
	ErrCodeDirectory
	ErrCodeBadURL
	ErrCodeInvalidTxnType
	ErrCodeTxn
	ErrCodeTxnHistory
	ErrCodeInvalidURL
	ErrCodeToken
	ErrCodeTokenAccount
	ErrCodeADI
	ErrCodeChainState
)

// Metrics errors
const (
	ErrCodeMetricsQuery = -32900 - iota
	ErrCodeMetricsNotAVector
	ErrCodeMetricsVectorEmpty
)

var (
	ErrInternal           = jsonrpc2.NewError(ErrCodeInternal, "Internal Error", "An internal error occured")
	ErrMetricsNotAVector  = jsonrpc2.NewError(ErrCodeMetricsNotAVector, "Metrics Query Error", "response is not a vector")
	ErrMetricsVectorEmpty = jsonrpc2.NewError(ErrCodeMetricsVectorEmpty, "Metrics Query Error", "response vector is empty")
)

func validatorError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeValidation, "Validation Error", err)
}

func invalidTxnTypeError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeInvalidTxnType, "Invalid Txn Type Error", err)
}

func invalidURLError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeInvalidURL, "Invalid URL Error", err)
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

func directoryError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Directory Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeDirectory, "Directory Error", err)
}

func metricsQueryError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(ErrCodeMetricsQuery, "Metrics Query Error", err)
}

func internalError(err error) jsonrpc2.Error {
	// Capture internal errors but do not forward them to the user
	sentry.CaptureException(err)
	return ErrInternal
}

func transactionError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Transaction Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeTxn, "Transaction Error", err)
}

func transactionHistoryError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Transaction History Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeTxnHistory, "Transaction History Error", err)
}

func tokenAccountError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Token Account Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeTokenAccount, "Token Account Error", err)
}

func tokenError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Token Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeToken, "Token Error", err)
}

func adiError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "ADI Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeADI, "ADI Error", err)
}

func chainStateError(err error) jsonrpc2.Error {
	if errors.Is(err, storage.ErrNotFound) {
		return jsonrpc2.NewError(ErrCodeNotFound, "Chain State Error", "Not Found")
	}
	return jsonrpc2.NewError(ErrCodeChainState, "Chain State Error", err)
}
