// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package private types.yml

// ServiceTypeSequencer defines the service type ID for sequencer services
const ServiceTypeSequencer uint32 = 0xF001

// Sequencer defines the interface for sequencing operations
type Sequencer interface {
	Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts interfaces.SequenceOptions) (*interfaces.MessageRecord, error)
}
