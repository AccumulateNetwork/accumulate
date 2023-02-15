package vdk

import "errors"

var ErrOnePassNotSupported = errors.New("1Password integration is not supported on macOS")

func canExecOnePass() error {
	return ErrOnePassNotSupported
}

func isOnePassWarn(err error) bool {
	return errors.Is(err, ErrOnePassNotSupported)
}
