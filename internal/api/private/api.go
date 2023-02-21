// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

const ServiceTypeSequencer api.ServiceType = 0xF001

type Sequencer interface {
	Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.TransactionRecord, error)
}
