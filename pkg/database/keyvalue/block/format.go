// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"strconv"
	"strings"
)

var DefaultNameFormat = simpleNameFormat{}

type NameFormat interface {
	Format(uint64) string
	// Parse(string) (uint64, error)
}

type simpleNameFormat struct{}

func (simpleNameFormat) Format(i uint64) string {
	return fmt.Sprintf("%d.blocks", i)
}

func (simpleNameFormat) Parse(s string) (uint64, error) {
	s = strings.TrimSuffix(s, ".blocks")
	i, err := strconv.ParseUint(s, 10, 64)
	return i, err
}
