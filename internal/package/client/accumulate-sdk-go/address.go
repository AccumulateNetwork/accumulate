package sdk

import (
	"github.com/AccumulateNetwork/accumulate/internal/url"
)


type AccAddress url.URL

func (a *AccAddress) String() string {
	return a.String()
}

// ----------------------------------------------------------------------------
// Validator Operator
// ----------------------------------------------------------------------------

// ValidatorAddress defines a wrapper a

type ValidatorAddress *url.URL