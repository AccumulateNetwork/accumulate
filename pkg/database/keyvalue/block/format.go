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
	Format(int) string
	Parse(string) (int, error)
}

type simpleNameFormat struct{}

func (simpleNameFormat) Format(i int) string {
	return fmt.Sprintf("%d.blocks", i)
}

func (simpleNameFormat) Parse(s string) (int, error) {
	s = strings.TrimSuffix(s, ".blocks")
	i, err := strconv.ParseInt(s, 10, 64)
	return int(i), err
}
