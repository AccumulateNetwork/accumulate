// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"

// The spine records (#4058) are defined in [api] so a public proof service can
// return them. These are aliases, not new types: code written against
// private.MajorHeaderRecord and code written against api.MajorHeaderRecord
// refer to the same type, so moving the definition breaks no caller.
type (
	MajorHeaderRecord  = api.MajorHeaderRecord
	MinorRootRecord    = api.MinorRootRecord
	NetworkUpdateProof = api.NetworkUpdateProof
)
