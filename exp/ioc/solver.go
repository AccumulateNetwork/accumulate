// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioc

import (
	"errors"
	"fmt"
)

type Factory interface {
	Requires() []Requirement
	Provides() []Provided
}

type Requirement struct {
	Descriptor Descriptor
	Optional   bool
}

type Provided struct {
	Descriptor Descriptor
}

func Solve[T Factory, S ~[]T](inputs ...S) ([]S, error) {
	var factories S
	for _, input := range inputs {
		factories = append(factories, input...)
	}

	var solved []S
	got := solverList{}
	for len(factories) > 0 {
		var unsatisfied, maybe, satisfied S
		for _, f := range factories {
			switch {
			case !got.haveNeeded(f):
				unsatisfied = append(unsatisfied, f)
			case !got.haveWanted(f):
				maybe = append(maybe, f)
			default:
				satisfied = append(satisfied, f)
			}
		}

		switch {
		case len(satisfied) > 0:
			// Some factories are satisfied, defer maybes to the next pass
			unsatisfied = append(unsatisfied, maybe...)
		case len(maybe) > 0:
			// No factories are fully satisfied so initialize the maybes
			satisfied = maybe
		default:
			msg := "cannot satisfy dependencies"
			for _, f := range unsatisfied {
				for _, n := range f.Requires() {
					s := fmt.Sprintf("%T requires %v", f, n.Descriptor.Type())
					if ns := n.Descriptor.Namespace(); ns != "" {
						s = fmt.Sprintf("%s (%s)", s, ns)
					}
					msg += "\n\t" + s
				}
			}
			return nil, errors.New(msg)
		}

		for _, f := range satisfied {
			got.add(f)
		}

		solved = append(solved, satisfied)
		factories = unsatisfied
	}

	return solved, nil
}

type solverList map[regKey]bool

func (s solverList) add(f Factory) {
	for _, p := range f.Provides() {
		s[desc2key(p.Descriptor)] = true
	}
}

func (s solverList) haveNeeded(f Factory) bool {
	for _, r := range f.Requires() {
		if !r.Optional && !s[desc2key(r.Descriptor)] {
			return false
		}
	}
	return true
}

func (s solverList) haveWanted(f Factory) bool {
	for _, r := range f.Requires() {
		if r.Optional && !s[desc2key(r.Descriptor)] {
			return false
		}
	}
	return true
}
