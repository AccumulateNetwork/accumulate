// Code generated by gitlab.com/accumulatenetwork/core/schema. DO NOT EDIT.

package bpt

import (
	"fmt"

	record "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/core/schema"
)

var (
	sbranch     schema.Methods[*branch, *branch, *schema.CompositeType]
	semptyNode  schema.Methods[*emptyNode, *emptyNode, *schema.CompositeType]
	sleaf       schema.Methods[*leaf, *leaf, *schema.CompositeType]
	snode       schema.Methods[node, *node, *schema.UnionType]
	snodeType   schema.EnumMethods[nodeType]
	sparameters schema.Methods[*parameters, *parameters, *schema.CompositeType]
	sstateData  schema.Methods[*stateData, *stateData, *schema.CompositeType]
)

func init() {
	var deferredTypes schema.ResolverSet

	sbranch = schema.WithMethods[*branch, *branch](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "branch",
		},
		Fields: []*schema.Field{
			{
				Name: "Height",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
			{
				Name: "Key",
				Type: &schema.SimpleType{Type: schema.SimpleTypeHash},
			},
			{
				Name: "Hash",
				Type: &schema.SimpleType{Type: schema.SimpleTypeHash},
			},
		},
		Transients: []*schema.Field{
			{
				Name: "bpt",
				Type: &schema.PointerType{
					TypeBase: schema.TypeBase{},
					Elem:     schema.ExternalTypeReference[BPT](),
				},
			},
			{
				Name: "parent",
				Type: (&schema.PointerType{
					TypeBase: schema.TypeBase{},
				}).
					ResolveElemTo(&deferredTypes, "branch"),
			},
			{
				Name: "status",
				Type: schema.ExternalTypeReference[branchStatus](),
			},
			(&schema.Field{
				Name: "Left",
			}).ResolveTo(&deferredTypes, "node"),
			(&schema.Field{
				Name: "Right",
			}).ResolveTo(&deferredTypes, "node"),
		},
	}).SetGoType()

	semptyNode = schema.WithMethods[*emptyNode, *emptyNode](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "emptyNode",
		},
		Transients: []*schema.Field{
			{
				Name: "parent",
				Type: (&schema.PointerType{
					TypeBase: schema.TypeBase{},
				}).
					ResolveElemTo(&deferredTypes, "branch"),
			},
		},
	}).SetGoType()

	sleaf = schema.WithMethods[*leaf, *leaf](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "leaf",
			Generate: schema.MapValue{
				"methods": schema.MapValue{
					"discriminator": schema.BooleanValue(false),
				},
			},
		},
		Fields: []*schema.Field{
			{
				Name: "Key",
				Type: &schema.PointerType{
					TypeBase: schema.TypeBase{},
					Elem:     schema.ExternalTypeReference[record.Key](),
				},
			},
			{
				Name: "Value",
				Type: &schema.SimpleType{Type: schema.SimpleTypeBytes},
			},
		},
		Transients: []*schema.Field{
			{
				Name: "parent",
				Type: (&schema.PointerType{
					TypeBase: schema.TypeBase{},
				}).
					ResolveElemTo(&deferredTypes, "branch"),
			},
		},
	}).SetGoType()

	snode = schema.WithMethods[node, *node](
		&schema.UnionType{
			TypeBase: schema.TypeBase{
				Name: "node",
			},
			Discriminator: (&schema.UnionDiscriminator{
				Field: "Type",
			}).
				ResolveTypeTo(&deferredTypes, "nodeType").
				ResolveEnumTo(&deferredTypes, "nodeType"),
			Members: []*schema.UnionMember{
				(&schema.UnionMember{
					Discriminator: "empty",
				}).ResolveTo(&deferredTypes, "emptyNode"),
				(&schema.UnionMember{
					Discriminator: "branch",
				}).ResolveTo(&deferredTypes, "branch"),
				(&schema.UnionMember{
					Discriminator: "leaf",
				}).ResolveTo(&deferredTypes, "leaf"),
			},
		}).SetGoType()

	snodeType = schema.WithEnumMethods[nodeType](
		&schema.EnumType{
			TypeBase: schema.TypeBase{
				Name: "nodeType",
			},
			Underlying: &schema.SimpleType{Type: schema.SimpleTypeInt},
			Values: map[string]*schema.EnumValue{
				"Boundary": {
					Name:        "Boundary",
					Value:       4,
					Description: "is the boundary between blocks",
				},
				"Branch": {
					Name:        "Branch",
					Value:       2,
					Description: "is a branch node",
				},
				"Empty": {
					Name:        "Empty",
					Value:       1,
					Description: "is an empty node",
				},
				"Leaf": {
					Name:        "Leaf",
					Value:       3,
					Description: "is a leaf node",
				},
				"LeafWithExpandedKey": {
					Name:        "LeafWithExpandedKey",
					Value:       5,
					Description: "is a leaf node with an expanded key",
					Label:       "leaf+key",
				},
			},
		}).SetGoType()

	sparameters = schema.WithMethods[*parameters, *parameters](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "parameters",
		},
		Fields: []*schema.Field{
			{
				Name: "Power",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
			{
				Name: "Mask",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
		},
	}).SetGoType()

	sstateData = schema.WithMethods[*stateData, *stateData](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "stateData",
		},
		Fields: []*schema.Field{
			{
				Name: "RootHash",
				Type: &schema.SimpleType{Type: schema.SimpleTypeHash},
			},
			{
				Name: "MaxHeight",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
			(&schema.Field{}).ResolveTo(&deferredTypes, "parameters"),
		},
	}).SetGoType()

	s, err := schema.New(
		sbranch.Type,
		semptyNode.Type,
		sleaf.Type,
		snode.Type,
		snodeType.Type,
		sparameters.Type,
		sstateData.Type,
	)
	if err != nil {
		panic(fmt.Errorf("invalid embedded schema: %w", err))
	}

	s.Generate = schema.MapValue{
		"import": schema.MapValue{
			"record": schema.StringValue("gitlab.com/accumulatenetwork/accumulate/pkg/types/record"),
		},
		"varPrefix": schema.MapValue{
			"schema": schema.StringValue("s"),
			"widget": schema.StringValue("w"),
		},
		"widgets": schema.BooleanValue(true),
	}

	deferredTypes.Resolve(s)
	err = s.Validate()
	if err != nil {
		panic(fmt.Errorf("invalid embedded schema: %w", err))
	}
}
