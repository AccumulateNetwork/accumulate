// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.
package block

import (
	"fmt"
	"testing"
)

func TestCommit(t *testing.T) {
	t.Run("Scenario 1", func(t *testing.T) {
		fmt.Println(t.Name())
		m := new(vmap[int, int])
		a := m.View()
		a.Put(1, 1)
		b := m.View()
		b.Put(2, 2)
		_ = a.Commit()
		_ = b.Commit()
	})

	t.Run("Scenario 2", func(t *testing.T) {
		fmt.Println(t.Name())
		m := new(vmap[int, int])
		a := m.View()
		b := m.View()
		b.Put(2, 2)
		_ = a.Commit()
		_ = b.Commit()
	})

	t.Run("Scenario 3", func(t *testing.T) {
		fmt.Println(t.Name())
		m := new(vmap[int, int])
		a := m.View()
		a.Put(1, 1)
		b := m.View()
		_ = a.Commit()
		_ = b.Commit()
	})

	t.Run("Scenario 4", func(t *testing.T) {
		fmt.Println(t.Name())
		m := new(vmap[int, int])
		a := m.View()
		a.Put(1, 1)
		b := m.View()
		_ = a.Commit()
		b.Discard()
	})
}
