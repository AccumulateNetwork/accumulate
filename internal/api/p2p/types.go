// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

//go:generate go run github.com/vektra/mockery/v2
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package p2p types.yml
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

// HasService returns true if Info includes the given Service.
func (i *Info) HasService(service *api.ServiceAddress) bool {
	for _, s := range i.Services {
		if s.Address.Equal(service) {
			return true
		}
	}
	return false
}
