package protocol

import (
	"fmt"
	"sort"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var enUsLower = cases.Lower(language.AmericanEnglish)

func (o *Object) ChainType(name string) ChainType {
	// Find the matching entry
	lcName := enUsLower.String(name)
	i := sort.Search(len(o.Chains), func(i int) bool {
		lcEntry := enUsLower.String(o.Chains[i].Name)
		return lcEntry >= lcName
	})

	if i >= len(o.Chains) {
		return ChainTypeUnknown
	}

	e := o.Chains[i]
	if lcName != enUsLower.String(e.Name) {
		return ChainTypeUnknown
	}

	return e.Type
}

// AddChain adds a chain to the object's list of chains using a binary search to
// ensure ordering. AddChain returns an error if there is an existing entry with
// the same name and a different type.
func (o *Object) AddChain(name string, typ ChainType) error {
	// Find the matching entry
	lcName := enUsLower.String(name)
	i := sort.Search(len(o.Chains), func(i int) bool {
		lcEntry := enUsLower.String(o.Chains[i].Name)
		return lcEntry >= lcName
	})

	// Append to the list
	if i >= len(o.Chains) {
		o.Chains = append(o.Chains, ChainMetadata{Name: name, Type: typ})
		return nil
	}

	// A matching entry exists
	lcEntry := enUsLower.String(o.Chains[i].Name)
	if lcEntry == lcName {
		if o.Chains[i].Type != typ {
			return fmt.Errorf("chain %s: attempted to change type from %v to %v", name, o.Chains[i].Type, typ)
		}
		return nil
	}

	// Insert within the list
	o.Chains = append(o.Chains, ChainMetadata{})
	copy(o.Chains[i+1:], o.Chains[i:])
	o.Chains[i] = ChainMetadata{Name: name, Type: typ}
	return nil
}
