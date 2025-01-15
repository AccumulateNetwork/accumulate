// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package url

import (
	"errors"
	"fmt"
)

// ErrMissingHost means that a URL did not include a hostname.
var ErrMissingHost = errors.New("missing host")

// ErrWrongScheme means that a URL included a scheme other than the Accumulate
// scheme.
var ErrWrongScheme = errors.New("wrong scheme")

// ErrMissingHash means that a transaction ID did not include a hash.
var ErrMissingHash = errors.New("missing hash")

// ErrInvalidHash means that a transaction ID did not include a valid hash.
var ErrInvalidHash = errors.New("invalid hash")

func missingHost(url string) error {
	return fmt.Errorf("%w in URL %q", ErrMissingHost, url)
}

func wrongScheme(url string) error {
	return fmt.Errorf("%w in URL %q", ErrWrongScheme, url)
}

func missingHash(url *URL) error {
	return fmt.Errorf("%q is not a transaction ID: %w", url, ErrMissingHash)
}

func invalidHash(url *URL, err interface{}) error {
	return fmt.Errorf("%q is not a transaction ID: %w: %v", url, ErrInvalidHash, err)
}
