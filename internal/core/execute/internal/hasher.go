// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
)

const debugHash = false

type hashable interface {
	Hash() [32]byte
	String() string
}

type rawHash [32]byte

func (r rawHash) Hash() [32]byte {
	return r
}

type hashableMemo struct {
	hashable
	memo any
}

type hashSet []hashable

func (h *hashSet) Hash() [32]byte {
	var hasher hash.Hasher
	for _, h := range *h {
		hasher.AddHash2(h.Hash())
	}
	return *(*[32]byte)(hasher.MerkleHash())
}

func (h *hashSet) Add(g hashable, memo any) {
	if debugHash && memo != "" {
		g = hashableMemo{g, memo}
	}
	*h = append(*h, g)
}

func (h *hashSet) Child(memo any) *hashSet {
	g := new(hashSet)
	h.Add(g, memo)
	return g
}

func (h rawHash) String() string {
	return fmt.Sprintf("%x", h.Hash())
}

func (h *hashSet) String() string {
	s := fmt.Sprintf("%x", h.Hash())
	for i, g := range *h {
		_ = i
		// if i > 10 {
		// 	s += "\n  ..."
		// 	break
		// }
		t := strings.Split(g.String(), "\n")
		for _, t := range t {
			s += "\n  " + t
		}
	}
	return s
}

func (h hashableMemo) String() string {
	s := h.hashable.String()
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return fmt.Sprintf("%s\t%v", s, h.memo)
	}
	return fmt.Sprintf("%s\t%v%s", s[:i], h.memo, s[i:])
}

type stringfn func() string

func (f stringfn) String() string {
	return f()
}

type stringers []any

func (s stringers) String() string {
	return fmt.Sprint(s...)
}
