// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package snapshot enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package snapshot types.yml

// SectionType is the type of a snapshot section.
type SectionType uint64

func (t SectionType) isOneOf(u ...SectionType) bool {
	for _, u := range u {
		if t == u {
			return true
		}
	}
	return false
}
