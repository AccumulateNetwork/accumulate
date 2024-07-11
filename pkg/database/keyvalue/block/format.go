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
	Format(*blockID) string
	Parse(string) (*blockID, error)
}

type simpleNameFormat struct{}

const dotBlocks = ".blocks"

func (simpleNameFormat) Format(b *blockID) string {
	if b.Part == 0 {
		return fmt.Sprintf("%d"+dotBlocks, b.ID)
	}
	return fmt.Sprintf("%d-%d"+dotBlocks, b.ID, b.Part)
}

func (simpleNameFormat) Parse(s string) (*blockID, error) {
	if !strings.HasSuffix(s, dotBlocks) {
		return nil, fmt.Errorf("%q is not a block file", s)
	}

	b := strings.Split(s[:len(s)-len(dotBlocks)], "-")
	if len(b) > 2 {
		return nil, fmt.Errorf("%q is not a block file", s)
	}

	i, err := strconv.ParseUint(b[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%q is not a block file: %w", s, err)
	}
	if len(b) == 1 {
		return &blockID{ID: i}, nil
	}

	j, err := strconv.ParseUint(b[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%q is not a block file: %w", s, err)
	}
	return &blockID{ID: i, Part: j}, nil
}
