// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import "strings"

//go:generate go run github.com/vektra/mockery/v2
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package p2p types.yml
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w .

// HasPartition returns true if Info includes the given partition.
func (i *Info) HasPartition(id string) bool {
	for _, p := range i.Partitions {
		if strings.EqualFold(p.ID, id) {
			return true
		}
	}
	return false
}
