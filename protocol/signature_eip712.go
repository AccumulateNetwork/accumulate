package protocol

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"golang.org/x/crypto/sha3"
)

//go:embed signature_eip712.json
var Eip712TransactionTypesFile []byte

// Global variable to hold the JSON data as a map
var eip712TransactionTypes map[string]encoding.TypeDefinition

func mergeStructFields(d *[]encoding.TypeField, s *[]encoding.TypeField) error {
	//loop through the typed data and merge in the types
	for _, v := range *s {
		haveEntry := false
		for _, u := range *d {
			if u.Name == v.Name {
				if u.Type == v.Type {
					haveEntry = true
					break
				} else {
					return fmt.Errorf("duplicate types in %s %s", u.Name, u.Type)
				}
			} else {
			}
		}
		if !haveEntry {
			*d = append(*d, v)
		}
	}

	return nil
}

type Func[T any, R any] func(T) (R, error)
type Enum interface {
	SetEnumValue(id uint64) bool
}

func typeFieldUnion[T any, R any](fieldTypeNameTarget string, f Func[T, R]) error {
	operationTypeData, ok := encoding.SchemaDictionary[fieldTypeNameTarget]
	if ok {
		return fmt.Errorf("%s already set", fieldTypeNameTarget)
	}

	operationTypeData = &[]encoding.TypeField{}
	encoding.SchemaDictionary[fieldTypeNameTarget] = operationTypeData

	enumType := new(T)
	enumValue, ok := any(enumType).(Enum)
	if !ok {
		return nil
	}

	for maxType := uint64(0); ; maxType++ {
		if !enumValue.SetEnumValue(maxType) {
			break
		}
		op, err := f(*enumType)
		if err != nil {
			continue
		}

		t := reflect.TypeOf(op)
		if t.Kind() == reflect.Ptr {
			t = t.Elem() // Safely obtaining the element type
		}

		name := t.Name()
		name = strings.TrimLeft(name, ".")

		t2, ok := encoding.SchemaDictionary[name]
		if !ok {
			continue
		}

		mergeStructFields(operationTypeData, t2)
	}
	return nil
}

var eip712Dictionary map[string]string

func getTypeEncoding(typeName string, fields []encoding.TypeField) string {
	// Sort fields by name to ensure deterministic order
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})

	fieldStrings := []string{}
	for _, field := range fields {
		fieldStrings = append(fieldStrings, fmt.Sprintf("%s %s", field.Type, field.Name))
	}

	return fmt.Sprintf("%s(%s)", typeName, strings.Join(fieldStrings, ","))
}

func init() {
	//handle edge cases
	encoding.RegisterTypeDefinitionResolver("DataEntry", func() {
		name := "DataEntry"
		err := typeFieldUnion(name, NewDataEntry)
		if err != nil {
			log.Printf("deferring %s registration: %v", name, err)
			return
		}

		//remove the resolver
		encoding.UnregisterTypeDefinitionResolver(name)
	})

	encoding.RegisterTypeDefinitionResolver("KeyPageOperation", func() {
		name := "KeyPageOperation"
		err := typeFieldUnion(name, NewKeyPageOperation)
		if err != nil {
			log.Printf("deferring %s registration: %v", name, err)
			return
		}

		//remove the resolver
		encoding.UnregisterTypeDefinitionResolver(name)

		//since this will run as the last resolver for what we care about,
		// create a pre-encoded cache
		for k, v := range encoding.SchemaDictionary {
			eip712Dictionary[k] = getTypeEncoding(k, *v)
		}
	})

	encoding.ResolveTypeDefinitions()
}

type TransactionHeaderMap struct {
	Principal string `json:"principal"`
	Initiator string `json:"initiator"`
}

type TransactionBodyMap map[string]interface{}

//
//type TransactionMessage struct {
//	Header TransactionHeader  `json:"header"`
//	Body   TransactionBodyMap `json:"body"`
//}

func encodeType(typeName string, types encoding.TypeDefinition) string {
	fields := types[typeName]
	sort.Slice(fields, func(i, j int) bool {
		return (*fields)[i].Name < (*fields)[j].Name
	})

	var buffer bytes.Buffer
	buffer.WriteString(typeName + "(")
	for i, field := range *fields {
		buffer.WriteString(field.Type + " " + field.Name)
		if i < len(*fields)-1 {
			buffer.WriteString(",")
		}
	}
	buffer.WriteString(")")
	return buffer.String()
}

func encodeData(data map[string]interface{}, typeName string, fields *[]encoding.TypeField) ([]byte, error) {
	//eip712 requires sorting of the fields prior to encoding, probably should presort these
	sort.Slice(fields, func(i, j int) bool {
		return (*fields)[i].Name < (*fields)[j].Name
	})

	//now loop through the fields and either encode the value or recursively dive into more types
	var buffer bytes.Buffer
	for _, field := range *fields {
		value := data[field.Name]
		encodedValue, err := encodeValue(value, field.Type)
		if err != nil {
			return nil, err
		}
		buffer.Write(encodedValue)
	}
	return buffer.Bytes(), nil
}

func encodeValue(value interface{}, typeName string) ([]byte, error) {
	// Implementation depends on the type
	// first look up the type to see if it is in the dictionary
	newType, ok := encoding.SchemaDictionary[typeName]
	if ok {
		//test if the type is an interface type, if not we're in trouble
		data, ok := value.(map[string]interface{})
		if !ok {
			///oops this is really bad...
			return nil, fmt.Errorf("expected map[string]interface{} for type %s", typeName)
		}
		//recurse back to encode data
		encodeData(data, typeName, newType)
	}
	// Now just process the primitive type
	//1 if string or variable length byte array, to the keccac of the value
	//if known binary type, just use the raw binary representation, anything less than a bytes32
	//needs to be right padded to 32 bytes.
	d, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	hasher := sha3.NewLegacyKeccak256()
	switch typeName {
	case "string", "bytes":
		hasher.Write(d)
	}
	return d, nil
}

func hashStruct(data map[string]interface{}, typeName string, eip712Types string) ([]byte, error) {
	fields, ok := encoding.SchemaDictionary[typeName]
	if !ok {
		return nil, fmt.Errorf("unable to locate type %s in dictionary", typeName)
	}
	//lookup typename
	encoded, err := encodeData(data, typeName, fields)
	if err != nil {
		return nil, err
	}
	hash := sha3.NewLegacyKeccak256()
	//hash.Write(eip712Types)
	hash.Write(encoded)
	return hash.Sum(nil), nil
}

func eip712hasher(sig Signature, txn Transaction) []byte {
	td, ok := eip712Dictionary[txn.Body.Type().String()]
	if !ok {
		log.Printf("unable to locate type %s", txn.Body.Type().String())
		return nil
	}

	j, err := txn.MarshalJSON()
	if err != nil {
		return nil
	}

	var jmap map[string]interface{}
	err = json.Unmarshal(j, &jmap)
	if err != nil {
		return nil
	}

	h, err := hashStruct(jmap, txn.Body.Type().String(), td)
	if err != nil {
		return nil
	}
	return h
}
