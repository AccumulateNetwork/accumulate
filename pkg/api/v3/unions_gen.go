// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// NewRecord creates a new Record for the specified RecordType.
func NewRecord(typ RecordType) (Record, error) {
	switch typ {
	case RecordTypeAccount:
		return new(AccountRecord), nil
	case RecordTypeChainEntry:
		return new(ChainEntryRecord[Record]), nil
	case RecordTypeChain:
		return new(ChainRecord), nil
	case RecordTypeIndexEntry:
		return new(IndexEntryRecord), nil
	case RecordTypeKey:
		return new(KeyRecord), nil
	case RecordTypeMajorBlock:
		return new(MajorBlockRecord), nil
	case RecordTypeMessage:
		return new(MessageRecord[messaging.Message]), nil
	case RecordTypeMinorBlock:
		return new(MinorBlockRecord), nil
	case RecordTypeRange:
		return new(RecordRange[Record]), nil
	case RecordTypeSignatureSet:
		return new(SignatureSetRecord), nil
	case RecordTypeTxID:
		return new(TxIDRecord), nil
	case RecordTypeUrl:
		return new(UrlRecord), nil
	default:
		return nil, fmt.Errorf("unknown record %v", typ)
	}
}

// EqualRecord is used to compare the values of the union
func EqualRecord(a, b Record) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *AccountRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*AccountRecord)
		return ok && a.Equal(b)
	case *ChainEntryRecord[Record]:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ChainEntryRecord[Record])
		return ok && a.Equal(b)
	case *ChainRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ChainRecord)
		return ok && a.Equal(b)
	case *IndexEntryRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*IndexEntryRecord)
		return ok && a.Equal(b)
	case *KeyRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*KeyRecord)
		return ok && a.Equal(b)
	case *MajorBlockRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MajorBlockRecord)
		return ok && a.Equal(b)
	case *MessageRecord[messaging.Message]:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MessageRecord[messaging.Message])
		return ok && a.Equal(b)
	case *MinorBlockRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MinorBlockRecord)
		return ok && a.Equal(b)
	case *RecordRange[Record]:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*RecordRange[Record])
		return ok && a.Equal(b)
	case *SignatureSetRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*SignatureSetRecord)
		return ok && a.Equal(b)
	case *TxIDRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*TxIDRecord)
		return ok && a.Equal(b)
	case *UrlRecord:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*UrlRecord)
		return ok && a.Equal(b)
	default:
		return false
	}
}

// CopyRecord copies a Record.
func CopyRecord(v Record) Record {
	switch v := v.(type) {
	case *AccountRecord:
		return v.Copy()
	case *ChainRecord:
		return v.Copy()
	case *IndexEntryRecord:
		return v.Copy()
	case *KeyRecord:
		return v.Copy()
	case *MajorBlockRecord:
		return v.Copy()
	case *MinorBlockRecord:
		return v.Copy()
	case *SignatureSetRecord:
		return v.Copy()
	case *TxIDRecord:
		return v.Copy()
	case *UrlRecord:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Record)
	}
}

// UnmarshalRecord unmarshals a Record.
func UnmarshalRecord(data []byte) (Record, error) {
	return UnmarshalRecordFrom(bytes.NewReader(data))
}

// UnmarshalRecordFrom unmarshals a Record.
func UnmarshalRecordFrom(rd io.Reader) (Record, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ RecordType
	if !reader.ReadEnum(1, &typ) {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new record
	v, err := NewRecord(RecordType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the record
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalRecordJson unmarshals a Record.
func UnmarshalRecordJSON(data []byte) (Record, error) {
	var typ *struct{ RecordType RecordType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewRecord(typ.RecordType)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewQuery creates a new Query for the specified QueryType.
func NewQuery(typ QueryType) (Query, error) {
	switch typ {
	case QueryTypeAnchorSearch:
		return new(AnchorSearchQuery), nil
	case QueryTypeBlock:
		return new(BlockQuery), nil
	case QueryTypeChain:
		return new(ChainQuery), nil
	case QueryTypeData:
		return new(DataQuery), nil
	case QueryTypeDefault:
		return new(DefaultQuery), nil
	case QueryTypeDelegateSearch:
		return new(DelegateSearchQuery), nil
	case QueryTypeDirectory:
		return new(DirectoryQuery), nil
	case QueryTypeMessageHashSearch:
		return new(MessageHashSearchQuery), nil
	case QueryTypePending:
		return new(PendingQuery), nil
	case QueryTypePublicKeyHashSearch:
		return new(PublicKeyHashSearchQuery), nil
	case QueryTypePublicKeySearch:
		return new(PublicKeySearchQuery), nil
	default:
		return nil, fmt.Errorf("unknown query %v", typ)
	}
}

// EqualQuery is used to compare the values of the union
func EqualQuery(a, b Query) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *AnchorSearchQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*AnchorSearchQuery)
		return ok && a.Equal(b)
	case *BlockQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*BlockQuery)
		return ok && a.Equal(b)
	case *ChainQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ChainQuery)
		return ok && a.Equal(b)
	case *DataQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*DataQuery)
		return ok && a.Equal(b)
	case *DefaultQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*DefaultQuery)
		return ok && a.Equal(b)
	case *DelegateSearchQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*DelegateSearchQuery)
		return ok && a.Equal(b)
	case *DirectoryQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*DirectoryQuery)
		return ok && a.Equal(b)
	case *MessageHashSearchQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*MessageHashSearchQuery)
		return ok && a.Equal(b)
	case *PendingQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*PendingQuery)
		return ok && a.Equal(b)
	case *PublicKeyHashSearchQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*PublicKeyHashSearchQuery)
		return ok && a.Equal(b)
	case *PublicKeySearchQuery:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*PublicKeySearchQuery)
		return ok && a.Equal(b)
	default:
		return false
	}
}

// CopyQuery copies a Query.
func CopyQuery(v Query) Query {
	switch v := v.(type) {
	case *AnchorSearchQuery:
		return v.Copy()
	case *BlockQuery:
		return v.Copy()
	case *ChainQuery:
		return v.Copy()
	case *DataQuery:
		return v.Copy()
	case *DefaultQuery:
		return v.Copy()
	case *DelegateSearchQuery:
		return v.Copy()
	case *DirectoryQuery:
		return v.Copy()
	case *MessageHashSearchQuery:
		return v.Copy()
	case *PendingQuery:
		return v.Copy()
	case *PublicKeyHashSearchQuery:
		return v.Copy()
	case *PublicKeySearchQuery:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Query)
	}
}

// UnmarshalQuery unmarshals a Query.
func UnmarshalQuery(data []byte) (Query, error) {
	return UnmarshalQueryFrom(bytes.NewReader(data))
}

// UnmarshalQueryFrom unmarshals a Query.
func UnmarshalQueryFrom(rd io.Reader) (Query, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ QueryType
	if !reader.ReadEnum(1, &typ) {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new query
	v, err := NewQuery(QueryType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the query
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalQueryJson unmarshals a Query.
func UnmarshalQueryJSON(data []byte) (Query, error) {
	var typ *struct{ QueryType QueryType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewQuery(typ.QueryType)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewEvent creates a new Event for the specified EventType.
func NewEvent(typ EventType) (Event, error) {
	switch typ {
	case EventTypeBlock:
		return new(BlockEvent), nil
	case EventTypeError:
		return new(ErrorEvent), nil
	case EventTypeGlobals:
		return new(GlobalsEvent), nil
	default:
		return nil, fmt.Errorf("unknown event %v", typ)
	}
}

// EqualEvent is used to compare the values of the union
func EqualEvent(a, b Event) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *BlockEvent:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*BlockEvent)
		return ok && a.Equal(b)
	case *ErrorEvent:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*ErrorEvent)
		return ok && a.Equal(b)
	case *GlobalsEvent:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*GlobalsEvent)
		return ok && a.Equal(b)
	default:
		return false
	}
}

// CopyEvent copies a Event.
func CopyEvent(v Event) Event {
	switch v := v.(type) {
	case *BlockEvent:
		return v.Copy()
	case *ErrorEvent:
		return v.Copy()
	case *GlobalsEvent:
		return v.Copy()
	default:
		return v.CopyAsInterface().(Event)
	}
}

// UnmarshalEvent unmarshals a Event.
func UnmarshalEvent(data []byte) (Event, error) {
	return UnmarshalEventFrom(bytes.NewReader(data))
}

// UnmarshalEventFrom unmarshals a Event.
func UnmarshalEventFrom(rd io.Reader) (Event, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ EventType
	if !reader.ReadEnum(1, &typ) {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new event
	v, err := NewEvent(EventType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the rest of the event
	err = v.UnmarshalFieldsFrom(reader)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalEventJson unmarshals a Event.
func UnmarshalEventJSON(data []byte) (Event, error) {
	var typ *struct{ EventType EventType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewEvent(typ.EventType)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
