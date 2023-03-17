// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
)

type store struct {
	mu     sync.RWMutex
	values map[storage.Key][]byte
}

func (s *store) get(p string, k storage.Key) ([]byte, bool) {
	if p != "" {
		k = k.Append(p)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[k]
	return v, ok
}

// func (s *store) put(p string, k storage.Key, v []byte) {
// 	if p != "" {
// 		k = k.Append(p)
// 	}
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()
// 	if s.values == nil {
// 		s.values = map[storage.Key][]byte{}
// 	}
// 	s.values[k] = v
// }

func (s *store) putAll(p string, v map[storage.Key][]byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.values == nil {
		s.values = make(map[storage.Key][]byte, len(v))
	}
	for k, v := range v {
		if p != "" {
			k = k.Append(p)
		}
		s.values[k] = v
	}
}

func (s *store) copy() *store {
	values := make(map[storage.Key][]byte, len(s.values))
	for k, v := range s.values {
		values[k] = v
	}
	return &store{values: values}
}
