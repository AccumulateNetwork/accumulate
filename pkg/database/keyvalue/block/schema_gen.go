// Code generated by gitlab.com/accumulatenetwork/core/schema. DO NOT EDIT.

package block

import (
	"fmt"
	record "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/core/schema"
)

var (
	sendBlockEntry   schema.Methods[*endBlockEntry, *endBlockEntry, *schema.CompositeType]
	sentry           schema.Methods[entry, *entry, *schema.UnionType]
	sentryType       schema.EnumMethods[entryType]
	srecordEntry     schema.Methods[*recordEntry, *recordEntry, *schema.CompositeType]
	sstartBlockEntry schema.Methods[*startBlockEntry, *startBlockEntry, *schema.CompositeType]
)

func init() {
	var deferredTypes schema.ResolverSet

	sendBlockEntry = schema.WithMethods[*endBlockEntry, *endBlockEntry](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "endBlockEntry",
		},
	}).SetGoType()

	sentry = schema.WithMethods[entry, *entry](
		&schema.UnionType{
			TypeBase: schema.TypeBase{
				Name: "entry",
			},
			Discriminator: (&schema.UnionDiscriminator{
				Field: "Type",
			}).
				ResolveTypeTo(&deferredTypes, "entryType").
				ResolveEnumTo(&deferredTypes, "entryType"),
			Members: []*schema.UnionMember{
				(&schema.UnionMember{
					Discriminator: "startBlock",
				}).ResolveTo(&deferredTypes, "startBlockEntry"),
				(&schema.UnionMember{
					Discriminator: "record",
				}).ResolveTo(&deferredTypes, "recordEntry"),
				(&schema.UnionMember{
					Discriminator: "endBlock",
				}).ResolveTo(&deferredTypes, "endBlockEntry"),
			},
		}).SetGoType()

	sentryType = schema.WithEnumMethods[entryType](
		&schema.EnumType{
			TypeBase: schema.TypeBase{
				Name: "entryType",
			},
			Underlying: &schema.SimpleType{Type: schema.SimpleTypeUint},
			Values: map[string]*schema.EnumValue{
				"EndBlock": {
					Name:  "EndBlock",
					Value: 3,
				},
				"Record": {
					Name:  "Record",
					Value: 2,
				},
				"StartBlock": {
					Name:  "StartBlock",
					Value: 1,
				},
			},
		}).SetGoType()

	srecordEntry = schema.WithMethods[*recordEntry, *recordEntry](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "recordEntry",
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
				Name: "KeyHash",
				Type: &schema.SimpleType{Type: schema.SimpleTypeHash},
			},
			{
				Name: "Length",
				Type: &schema.SimpleType{Type: schema.SimpleTypeInt},
			},
		},
	}).SetGoType()

	sstartBlockEntry = schema.WithMethods[*startBlockEntry, *startBlockEntry](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "startBlockEntry",
		},
		Fields: []*schema.Field{
			{
				Name: "ID",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
			{
				Name: "Parent",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
		},
	}).SetGoType()

	s, err := schema.New(
		sendBlockEntry.Type,
		sentry.Type,
		sentryType.Type,
		srecordEntry.Type,
		sstartBlockEntry.Type,
	)
	if err != nil {
		panic(fmt.Errorf("invalid embedded schema: %w", err))
	}

	s.Generate = schema.MapValue{
		"import": schema.MapValue{
			"record": schema.StringValue("gitlab.com/accumulatenetwork/accumulate/pkg/types/record"),
		},
		"methods": schema.MapValue{
			"binary": schema.BooleanValue(true),
			"json":   schema.BooleanValue(true),
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
