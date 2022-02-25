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

func missingHost(url string) error {
	return fmt.Errorf("%w in URL %q", ErrMissingHost, url)
}

func wrongScheme(url string) error {
	return fmt.Errorf("%w in URL %q", ErrWrongScheme, url)
}
