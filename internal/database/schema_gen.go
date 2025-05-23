// Code generated by gitlab.com/accumulatenetwork/core/schema. DO NOT EDIT.

package database

import (
	"fmt"
	"time"

	protocol "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/core/schema"
)

var (
	sBlockLedger schema.Methods[*BlockLedger, *BlockLedger, *schema.CompositeType]
)

func init() {
	var deferredTypes schema.ResolverSet

	sBlockLedger = schema.WithMethods[*BlockLedger, *BlockLedger](&schema.CompositeType{
		TypeBase: schema.TypeBase{
			Name: "BlockLedger",
		},
		Fields: []*schema.Field{
			{
				Name: "Index",
				Type: &schema.SimpleType{Type: schema.SimpleTypeUint},
			},
			{
				Name: "Time",
				Type: schema.TypeReferenceFor[time.Time](),
			},
			{
				Name: "Entries",
				Type: &schema.ArrayType{
					TypeBase: schema.TypeBase{},
					Elem: &schema.PointerType{
						TypeBase: schema.TypeBase{},
						Elem:     schema.TypeReferenceFor[protocol.BlockEntry](),
					},
				},
			},
		},
	}).SetGoType()

	s, err := schema.New(
		sBlockLedger.Type,
	)
	if err != nil {
		panic(fmt.Errorf("invalid embedded schema: %w", err))
	}

	s.Generate = schema.MapValue{
		"import": schema.MapValue{
			"encoding": schema.StringValue("gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"),
			"protocol": schema.StringValue("gitlab.com/accumulatenetwork/accumulate/protocol"),
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
