// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

import "gitlab.com/accumulatenetwork/accumulate/pkg/errors"

type (
	Error  = errors.Error
	Status = errors.Status
)

const (
	StatusOK                  = errors.OK
	StatusDelivered           = errors.Delivered
	StatusPending             = errors.Pending
	StatusRemote              = errors.Remote
	StatusWrongPartition      = errors.WrongPartition
	StatusBadRequest          = errors.BadRequest
	StatusUnauthenticated     = errors.Unauthenticated
	StatusInsufficientCredits = errors.InsufficientCredits
	StatusUnauthorized        = errors.Unauthorized
	StatusNotFound            = errors.NotFound
	StatusNotAllowed          = errors.NotAllowed
	StatusConflict            = errors.Conflict
	StatusBadSignerVersion    = errors.BadSignerVersion
	StatusBadTimestamp        = errors.BadTimestamp
	StatusBadUrlLength        = errors.BadUrlLength
	StatusIncompleteChain     = errors.IncompleteChain
	StatusInsufficientBalance = errors.InsufficientBalance
	StatusInternalError       = errors.InternalError
	StatusUnknownError        = errors.UnknownError
	StatusEncodingError       = errors.EncodingError
	StatusFatalError          = errors.FatalError
)

func As(err error, target interface{}) bool { return errors.As(err, target) }
func Is(err, target error) bool             { return errors.Is(err, target) }
func Unwrap(err error) error                { return errors.Unwrap(err) }

func New(code Status, v interface{}) *Error {
	return errors.New(code, v)
}

func Wrap(code Status, err error) error {
	return errors.Wrap(code, err)
}

func FormatWithCause(code Status, cause error, format string, args ...interface{}) *Error {
	return errors.FormatWithCause(code, cause, format, args...)
}

func Format(code Status, format string, args ...interface{}) *Error {
	return errors.Format(code, format, args...)
}
