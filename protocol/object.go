package protocol

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var enUsLower = cases.Lower(language.AmericanEnglish)

func (o *Object) ChainType(name string) ChainType {
	// Find the matching entry
	lcName := enUsLower.String(name)
	i, found := sortutil.Search(o.Chains, func(entry ChainMetadata) int {
		lcEntry := enUsLower.String(entry.Name)
		return strings.Compare(lcEntry, lcName)
	})
	if !found {
		return ChainTypeUnknown
	}
	return o.Chains[i].Type
}

// AddChain adds a chain to the object's list of chains using a binary search to
// ensure ordering. AddChain returns an error if there is an existing entry with
// the same name and a different type.
func (o *Object) AddChain(name string, typ ChainType) error {
	// Find the matching entry
	lcName := enUsLower.String(name)
	ptr, new := sortutil.BinaryInsert(&o.Chains, func(entry ChainMetadata) int {
		lcEntry := enUsLower.String(entry.Name)
		return strings.Compare(lcEntry, lcName)
	})

	// A matching entry exists
	if !new {
		if ptr.Type != typ {
			return fmt.Errorf("chain %s: attempted to change type from %v to %v", name, ptr.Type, typ)
		}
		return nil
	}

	// Update the new entry
	*ptr = ChainMetadata{Name: name, Type: typ}
	return nil
}

func (c *ChainMetadata) Compare(d *ChainMetadata) int {
	cn := enUsLower.String(c.Name)
	dn := enUsLower.String(d.Name)
	return strings.Compare(cn, dn)
}
