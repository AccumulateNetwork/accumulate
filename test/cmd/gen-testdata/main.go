// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

//lint:file-ignore ST1001 Don't care

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	sdktest "gitlab.com/accumulatenetwork/accumulate/test/sdk"
	randPkg "golang.org/x/exp/rand"
)

// seed: m/44'/281'/0'/0'/0'
var keySeed, _ = hex.DecodeString("a2fd3e3b8c130edac176da83dcf809e22a01ab5a853560806e6cc054b3e160b0")
var key = ed25519.NewKeyFromSeed(keySeed[:])
var rand = randPkg.New(randPkg.NewSource(binary.BigEndian.Uint64(keySeed[:])))
var useSimpleHash = flag.Bool("simple", true, "Use simple hashes for signatures")

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

var cmdMain = &cobra.Command{
	Use:   "gen-testdata",
	Short: "Generate Accumulate test data",
	Run:   run,
}

var GenerateEip712Vectors bool
var DataDir string

func run(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: gen-testdata <output-file> <ledger-test-vectors-file>")
		os.Exit(1)
	}

	file := args[0]

	// If the file is in a directory, make sure the directory exists
	dir := filepath.Dir(file)
	if dir != "" && dir != "." {
		check(os.MkdirAll(dir, 0755))
	}

	ts := &sdktest.TestSuite{
		Transactions: transactionTests(txnTest),
		Accounts:     accountTests(txnTest),
	}

	check(ts.Store(file))

	schema := transactionTests(txnTypedDataTestVectors)
	check(StoreTransactionSchema("eip712"+file, schema))

	if len(args) > 1 {
		lts := &sdktest.TestSuite{
			Transactions: transactionTests(txnUnsignedEnvelopeTestVectors),
			Accounts:     accountTests(txnUnsignedEnvelopeTestVectors),
		}
		check(lts.Store(args[1]))
	}
}

func main() {
	cmdMain.Flags().StringVar(&DataDir, "corpus", "corpus", "Data output directory")
	cmdMain.Flags().BoolVar(&GenerateEip712Vectors, "typed-data", false, "Generate test vectors for typed data transactions")
	_ = cmdMain.Execute()
}

type TCG = sdktest.TestCaseGroup
type TC = sdktest.TestCase

func StoreTransactionSchema(file string, tc []*TCG) error {
	tsm := make(map[string]*json.RawMessage)
	for _, v := range tc {
		tsm[v.Name] = &v.Cases[0].JSON
	}

	b, err := json.Marshal(tsm)
	if err != nil {
		return err
	}

	return os.WriteFile(file, b, 0755)
}

func transactionTests(txnTest func(*url.URL, TransactionBody) *TC) []*TCG {
	var txnTests = []*TCG{
		{Name: "CreateIdentity", Cases: []*TC{
			txnTest(AccountUrl("lite-token-account", "ACME"), &CreateIdentity{Url: AccountUrl("adi"), KeyHash: key[32:]}),
			txnTest(AccountUrl("lite-token-account", "ACME"), &CreateIdentity{Url: AccountUrl("adi"), KeyHash: key[32:], KeyBookUrl: AccountUrl("adi", "book")}),
		}},
		{Name: "CreateTokenAccount", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateTokenAccount{Url: AccountUrl("adi", "ACME"), TokenUrl: AccountUrl("ACME")}),
			txnTest(AccountUrl("adi"), &CreateTokenAccount{Url: AccountUrl("adi", "ACME"), TokenUrl: AccountUrl("ACME"), Authorities: []*url.URL{AccountUrl("adi", "book")}}),
		}},
		{Name: "SendTokens", Cases: []*TC{
			txnTest(AccountUrl("adi", "ACME"), &SendTokens{To: []*TokenRecipient{{Url: AccountUrl("other", "ACME"), Amount: *new(big.Int).SetInt64(100)}}}),
			txnTest(AccountUrl("adi", "ACME"), &SendTokens{To: []*TokenRecipient{{Url: AccountUrl("other", "ACME"), Amount: *new(big.Int).SetInt64(100)}}, Meta: json.RawMessage(`{"foo":"bar"}`)}),
		}},
		{Name: "CreateDataAccount", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateDataAccount{Url: AccountUrl("adi", "data")}),
		}},
		{Name: "WriteData", Cases: []*TC{
			txnTest(AccountUrl("adi"), &WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
		}},
		{Name: "WriteDataTo", Cases: []*TC{
			txnTest(AccountUrl("adi"), &WriteDataTo{Recipient: AccountUrl("lite-data-account"), Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
		}},
		{Name: "AcmeFaucet", Cases: []*TC{
			txnTest(AccountUrl("faucet"), &AcmeFaucet{Url: AccountUrl("lite-token-account")}),
		}},
		{Name: "CreateToken", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateToken{Url: AccountUrl("adi", "foocoin"), Symbol: "FOO", Precision: 10}),
		}},
		{Name: "IssueTokens", Cases: []*TC{
			txnTest(AccountUrl("adi", "foocoin"), &IssueTokens{Recipient: AccountUrl("adi", "foo"), Amount: *new(big.Int).SetInt64(100)}),
		}},
		{Name: "BurnTokens", Cases: []*TC{
			txnTest(AccountUrl("adi", "foo"), &BurnTokens{Amount: *new(big.Int).SetInt64(100)}),
		}},
		{Name: "CreateKeyPage", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: key[32:]}}}),
		}},
		{Name: "CreateKeyBook", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateKeyBook{Url: AccountUrl("adi", "book"), PublicKeyHash: key[32:]}),
		}},
		{Name: "AddCredits", Cases: []*TC{
			txnTest(AccountUrl("lite-token-account"), &AddCredits{Recipient: AccountUrl("adi", "page"), Amount: *big.NewInt(200000), Oracle: 50000000}),
		}},
		{Name: "UpdateKeyPage", Cases: []*TC{
			txnTest(AccountUrl("adi"), &UpdateKeyPage{Operation: []KeyPageOperation{&AddKeyOperation{Entry: KeySpecParams{KeyHash: key[32:]}}}}),
		}},
		{Name: "SignPending", Cases: []*TC{
			txnTest(AccountUrl("adi"), &RemoteTransaction{}),
		}},
		{Name: "SyntheticCreateIdentity", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticCreateIdentity{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Accounts: []Account{&UnknownAccount{Url: AccountUrl("foo")}}}),
		}},
		{Name: "SyntheticWriteData", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticWriteData{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
		}},
		{Name: "SyntheticDepositTokens", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticDepositTokens{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Token: AccountUrl("ACME"), Amount: *new(big.Int).SetInt64(10000)}),
		}},
		{Name: "SyntheticDepositCredits", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticDepositCredits{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})}, Amount: 1234}),
		}},
		{Name: "SyntheticBurnTokens", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticBurnTokens{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Amount: *big.NewInt(123456789)}),
		}},
	}

	return txnTests
}

func accountTests(txnTest func(*url.URL, TransactionBody) *TC) []*TCG {
	var simpleAuth = &AccountAuth{Authorities: []AuthorityEntry{{Url: AccountUrl("adi", "book")}}}
	var managerAuth = &AccountAuth{Authorities: []AuthorityEntry{{Url: AccountUrl("adi", "book"), Disabled: true}, {Url: AccountUrl("adi", "mgr")}}}

	var acntTests = []*TCG{
		{Name: "Identity", Cases: []*TC{
			sdktest.NewAcntTest(&ADI{Url: AccountUrl("adi"), AccountAuth: *simpleAuth}),
			sdktest.NewAcntTest(&ADI{Url: AccountUrl("adi"), AccountAuth: *managerAuth}),
		}},
		{Name: "TokenIssuer", Cases: []*TC{
			sdktest.NewAcntTest(&TokenIssuer{Url: AccountUrl("adi", "foocoin"), AccountAuth: *simpleAuth, Symbol: "FOO", Precision: 10}),
		}},
		{Name: "TokenAccount", Cases: []*TC{
			sdktest.NewAcntTest(&TokenAccount{Url: AccountUrl("adi", "foo"), AccountAuth: *simpleAuth, TokenUrl: AccountUrl("adi", "foocoin"), Balance: *big.NewInt(123456789)}),
		}},
		{Name: "LiteTokenAccount", Cases: []*TC{
			sdktest.NewAcntTest(&LiteTokenAccount{Url: AccountUrl("lite-token-account"), TokenUrl: AccountUrl("ACME"), Balance: *big.NewInt(12345)}),
		}},
		{Name: "LiteIdentity", Cases: []*TC{
			sdktest.NewAcntTest(&LiteIdentity{Url: AccountUrl("lite-identity"), LastUsedOn: uint64(rand.Uint32()), CreditBalance: 9835}),
		}},
		{Name: "KeyPage", Cases: []*TC{
			sdktest.NewAcntTest(&KeyPage{Url: AccountUrl("adi", "page"), Keys: []*KeySpec{{PublicKeyHash: key[32:], LastUsedOn: uint64(rand.Uint32()), Delegate: AccountUrl("foo", "bar")}}, CreditBalance: 98532, AcceptThreshold: 3}),
		}},
		{Name: "KeyBook", Cases: []*TC{
			sdktest.NewAcntTest(&KeyBook{Url: AccountUrl("adi", "book")}),
		}},
		{Name: "DataAccount", Cases: []*TC{
			sdktest.NewAcntTest(&DataAccount{Url: AccountUrl("adi", "data"), AccountAuth: *simpleAuth}),
		}},
		{Name: "LiteDataAccount", Cases: []*TC{
			sdktest.NewAcntTest(&LiteDataAccount{Url: AccountUrl("lite-data-account")}),
		}},
	}

	return acntTests
}

func txnTest(originUrl *url.URL, body TransactionBody) *TC {
	signer := new(signing.Builder)
	// In reality this would not work, but *shrug* it's a marshalling test
	signer.Type = SignatureTypeED25519
	signer.Url = originUrl
	signer.SetPrivateKey(key)
	signer.Version = 1
	//provide a deterministic timestamp
	signer.SetTimestamp(uint64(1234567890))
	env := new(messaging.Envelope)
	txn := new(Transaction)
	env.Transaction = []*Transaction{txn}
	txn.Header.Principal = originUrl
	txn.Body = body

	if *useSimpleHash {
		signer.InitMode = signing.InitWithSimpleHash
	}

	sig, err := signer.Initiate(txn)
	if err != nil {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	return sdktest.NewTxnTest(env)
}

// txnUnsignedEnvelopeTestVectors creates an unsigned envelope which is used to create envelope test
// vectors for use by external signers such as the Ledger hardware wallet
func txnUnsignedEnvelopeTestVectors(originUrl *url.URL, body TransactionBody) *TC {
	signer := new(signing.Builder)
	lts := EmptySigner{}
	lts.PubKey = key.Public().(ed25519.PublicKey)

	signer.Signer = &lts

	// this is not a real transaction, only used for test vector generation
	signer.Type = SignatureTypeED25519
	signer.Url = originUrl
	signer.Version = 1
	signer.SetTimestamp(uint64(1234567890))

	env := new(messaging.Envelope)
	txn := new(Transaction)
	env.Transaction = []*Transaction{txn}
	txn.Header.Principal = originUrl
	txn.Body = body

	if *useSimpleHash {
		signer.InitMode = signing.InitWithSimpleHash
	}

	sig, err := signer.Initiate(txn)
	if err != nil && !errors.Is(err, UseRealSigner) {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	if txn.Body.Type() == TransactionTypeSendTokens {
		data, _ := env.MarshalBinary()
		fmt.Println(hex.EncodeToString(data))
	}
	return sdktest.NewTxnTest(env)
}

// txnTypedDataTestVectors creates an unsigned envelope which is used to create envelope test
// vectors for use by external signers such as the Ledger hardware wallet

type TypesEntry struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Type string `json:"type,omitempty" form:"type" query:"type" validate:"required"`
}

type TypedData struct {
	TypeName string
	Structs  []*TypedData
	Types    []TypesEntry
}

var Eip712DomainType = []TypesEntry{
	{Name: "name", Type: "string"},
	{Name: "version", Type: "string"},
	{Name: "chainId", Type: "string"},
}

//
//type EIP712Domain struct {
//	Name    string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
//	Version string `json:"version,omitempty" form:"version" query:"version" validate:"required"`
//	ChainId string `json:"chainId,omitempty" form:"chainId" query:"chainId" validate:"required"`
//}

//var Eip712Domain = EIP712Domain{
//	Name:    "Accumulate",
//	Version: "0.0.7",
//	ChainId: 281,
//}

type TransactionHeaderType struct {
	fieldsSet []bool
	Principal *url.URL `json:"principal,omitempty" form:"principal" query:"principal" validate:"required"`
	Initiator [32]byte `json:"initiator,omitempty" form:"initiator" query:"initiator" validate:"required"`
	Memo      string   `json:"memo,omitempty" form:"memo" query:"memo"`
	Metadata  []byte   `json:"metadata,omitempty" form:"metadata" query:"metadata"`
	// Expire expires the transaction as pending once the condition(s) are met.
	Expire *ExpireOptions `json:"expire,omitempty" form:"expire" query:"expire"`
	// HoldUntil holds the transaction as pending until the condition(s) are met.
	HoldUntil *HoldUntilOptions `json:"holdUntil,omitempty" form:"holdUntil" query:"holdUntil"`
	// Authorities is a list of additional authorities that must approve the transaction.
	Authorities []*url.URL `json:"authorities,omitempty" form:"authorities" query:"authorities"`
	extraData   []byte
}

var knownReducedTypes = map[string]string{
	"URL":             "string",
	"url.URL":         "string",
	"time.Time":       "uint256",
	"Time":            "uint256",
	"big.Int":         "uint256",
	"Int":             "uint256",
	"TransactionType": "string",
}

var knownDataTypes = map[string][]TypesEntry{}

func mergeStructFields(typeEntries *TypedData, t *TypedData) error {
	if t == nil {
		return nil
	}
	//loop through the typed data and merge in the types
	for _, v := range t.Types {
		haveEntry := false
		for _, u := range typeEntries.Types {
			if u.Name == v.Name {
				if u.Type == v.Type {
					haveEntry = true
					break
				} else {
					return fmt.Errorf("duplicate types in %s %s", typeEntries.TypeName, u.Type)
				}
			} else {
			}
		}
		if !haveEntry {
			typeEntries.Types = append(typeEntries.Types, v)
		}
	}

	for _, u := range t.Structs {
		haveEntry := false
		for _, v := range typeEntries.Structs {
			if v.TypeName == u.TypeName {
				haveEntry = true
				break
			}
		}
		if !haveEntry {
			typeEntries.Structs = append(typeEntries.Structs, u)
		}
	}

	return nil
}

type Func[T any, R any] func(T) (R, error)
type Enum interface {
	SetEnumValue(id uint64) bool
}

func typeFields[T any, R any](typeEntries *TypedData, fieldTypeName string, fieldTypeNameTarget string, f Func[T, R]) *TypedData {
	if strings.Contains(fieldTypeName, fieldTypeNameTarget) {
		enumType := new(T)
		enumValue, ok := any(enumType).(Enum)
		if !ok {
			return nil
		}
		operationTypeData := TypedData{
			TypeName: fieldTypeName,
		}
		typeEntries.Structs = append(typeEntries.Structs, &operationTypeData)
		for maxType := uint64(0); ; maxType++ {
			if !enumValue.SetEnumValue(maxType) {
				break
			}
			op, err := f(*enumType)
			if err != nil {
				//fmt.Println("Error calling function with enum:", err)
				continue
			}

			t := reflect.TypeOf(op)
			if t.Kind() == reflect.Ptr {
				t = t.Elem() // Safely obtaining the element type
			}

			name := t.Name()
			name = strings.TrimLeft(name, ".")

			t2 := printStructFields(t.Name(), t, "")
			mergeStructFields(&operationTypeData, t2)
		}
	}
	return typeEntries
}

func printStructFields(typeName string, t reflect.Type, indent string) *TypedData {
	// Ensure we're dealing with the type, not a pointer to the type
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	re, err := regexp.Compile(`\[.*?\]`)
	if err != nil {
		panic(fmt.Sprintf("Error compiling regex:", err))
	}

	typeEntries := TypedData{}

	parts := strings.Split(typeName, ".")
	typeName = parts[len(parts)-1]

	typeEntries.TypeName = typeName
	// Process only if it's a struct
	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			// Extract JSON tag, falling back to the field name if the tag isn't present
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" {
				//assuming if there is no tag, we are only interested in the type if it appears in a map
				continue //jsonTag = field.Name
			}
			parts := strings.FieldsFunc(jsonTag, func(r rune) bool {
				// Define delimiters: whitespace, comma, or end of line.
				return unicode.IsSpace(r) || r == ',' || r == '\n'
			})

			//we will at least have 1 parts because jsonTag is not empty
			fieldName := parts[0]

			fieldType := field.Type

			var fieldTypeName string
			var structName string
			if fieldType.Kind() == reflect.Slice {
				if fieldType.Elem().Kind() == reflect.Ptr {
					fieldTypeName = fieldType.String() //Elem().Elem().Name()
				} else {
					fieldTypeName = fieldType.String() //Elem().String()
				}

				fieldTypeName = strings.ReplaceAll(fieldTypeName, "*", "")
				matchBytes := re.FindAllString(fieldTypeName, -1)

				// Replace all occurrences of the pattern with an empty string
				fieldTypeName = re.ReplaceAllString(fieldTypeName, "")

				v, ok := knownReducedTypes[fieldTypeName]
				if ok {
					fieldTypeName = v
				}

				structName = fieldTypeName
				fieldTypeName += strings.Join(matchBytes, "")
			} else if fieldType.Kind() == reflect.Array {
				fieldTypeName = fieldType.Elem().Name()
				v, ok := knownReducedTypes[fieldTypeName]
				if ok {
					fieldTypeName = v
				}

				structName = fieldTypeName
				fieldTypeName = fmt.Sprintf("%s[%d]", fieldTypeName, fieldType.Size())
			} else if fieldType.Kind() == reflect.Ptr {
				fieldTypeName = fieldType.Elem().Name()
				v, ok := knownReducedTypes[fieldTypeName]
				if ok {
					fieldTypeName = v
				}
			} else {
				fieldTypeName = fieldType.String()

				v, ok := knownReducedTypes[fieldTypeName]
				if ok {
					fieldTypeName = v
				}
				structName = fieldTypeName
			}

			parts = strings.Split(structName, ".")
			structName = parts[len(parts)-1]

			parts = strings.Split(fieldTypeName, ".")
			fieldTypeName = parts[len(parts)-1]

			encoding.RegisterTypeDefinitionResolver("KeyPageOperation", func() {
				_ = typeFields(&typeEntries, structName, "KeyPageOperation", NewKeyPageOperation)
			})
			encoding.RegisterTypeDefinitionResolver("DataEntry", func() {
				_ = typeFields(&typeEntries, structName, "DataEntry", NewDataEntry)
			})
			_ = typeFields(&typeEntries, structName, "KeyPageOperation", NewKeyPageOperation)

			typeEntries.Types = append(typeEntries.Types, TypesEntry{Name: fieldName, Type: fieldTypeName})

			// If the field is a struct or a pointer to a struct, inspect recursively
			if fieldType.Kind() == reflect.Struct || (fieldType.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct) {
				s := printStructFields(fieldTypeName, fieldType, indent+"  ")
				if s != nil {
					typeEntries.Structs = append(typeEntries.Structs, s)
				}
			}

			// If the field is a slice or array, check the element type
			if fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array {
				elemType := fieldType.Elem()

				if elemType.Kind() == reflect.Struct || (elemType.Kind() == reflect.Ptr && elemType.Elem().Kind() == reflect.Struct) {
					s := printStructFields(elemType.Elem().String(), elemType, indent+"    ")
					if s != nil {
						typeEntries.Structs = append(typeEntries.Structs, s)
					}
				}
			}
		}
	}
	if len(typeEntries.Types) == 0 && len(typeEntries.Structs) == 0 {
		return nil
	}
	return &typeEntries
}

func parseTypes(txh any) []TypesEntry {
	t := reflect.TypeOf(txh)

	types := []TypesEntry{}
	//pattern := regexp.MustCompile(`(\[\d*\])?(\*?\w+)`)
	pattern := regexp.MustCompile(`(\[\d*\])?(\*?.+)`)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Extract JSON tag, falling back to the field name if the tag isn't present
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			//assuming if there is no tag, we are only interested in the type if it appears in a map
			continue //jsonTag = field.Name
		}

		parts := strings.FieldsFunc(jsonTag, func(r rune) bool {
			// Define delimiters: whitespace, comma, or end of line.
			return unicode.IsSpace(r) || r == ',' || r == '\n'
		})

		//we will at least have 1 parts because jsonTag is not empty
		name := parts[0]
		//
		//parts = strings.FieldsFunc(field.Type.String(), func(r rune) bool {
		//	// Define delimiters: whitespace, comma, or end of line.
		//	return unicode.IsSpace(r) || r == '*'
		//})
		matches := pattern.FindStringSubmatch(field.Type.String())

		array := ""
		typeName := ""

		// Check if the array part was matched
		if len(matches) > 1 && matches[1] != "" {
			array = matches[1] // The array part, e.g., "[32]"
		}
		// The type name, possibly including an asterisk for pointers
		if len(matches) > 2 {
			typeName = matches[2] // The type name, e.g., "*int"
		}

		types = append(types, TypesEntry{Name: name, Type: typeName + array})
		//fmt.Printf("Name: %s, Type: %s%s\n", name, typeName, array)
	}

	return types
}

func printTypedData(data *TypedData, typedDataList []*TypedData, typesList *map[string]json.RawMessage) {
	if data == nil {
		return
	}

	for _, s := range typedDataList {
		printTypedData(s, nil, typesList)
	}

	d, err := json.Marshal(data.Types)
	if err != nil {
		return
	}

	(*typesList)[data.TypeName] = json.RawMessage(d)
	return
}
func mapTypedData(typedDataMap *map[string]*TypedData, typedDataList *[]*TypedData, data *TypedData) {
	if data == nil {
		return
	}

	if typedDataMap == nil {
		typedDataMap = new(map[string]*TypedData)
		*typedDataMap = make(map[string]*TypedData)
	}

	ok := false
	for i, s := range data.Structs {
		if s == nil {
			continue
		}
		mapTypedData(typedDataMap, typedDataList, s)
		_, ok = (*typedDataMap)[s.TypeName]
		if !ok {
			(*typedDataMap)[s.TypeName] = data.Structs[i]
			*typedDataList = append(*typedDataList, data.Structs[i])
		}
	}
}

// TODO: deal with DataEntry and Account, also mising URL
func txnTypedDataTestVectors(originUrl *url.URL, body TransactionBody) *TC {
	txh := TransactionHeader{}
	_ = txh

	//txnHeaderEntries := parseTypes(txh)
	exampleType := reflect.TypeOf(body)
	//	fmt.Printf("Inspecting ExampleNestedStruct: %s\n", exampleType.Elem().Name())

	typedData := printStructFields(exampleType.Elem().Name(), exampleType, "")
	_ = typedData
	//for v := range typedData.Structs {
	//	printStructs()
	//}
	//Reflect the transaction
	//	txnBodyEntries := parseTypes(body)

	tx := Transaction{}
	tx.Body = body
	tx.Header = txh
	td := encoding.TypeDefinition{}
	body.MarshalSchema(&td)
	err := tx.MarshalSchema(&td)
	fmt.Printf("%v", td)
	var typedDataList []*TypedData

	mapTypedData(nil, &typedDataList, typedData)

	//	eip712 := EIP712Domain{"Accumulate", "0.0.1", "281"}
	//	d, err := json.Marshal(eip712)
	//	if err != nil {
	//		return nil
	//	}
	tc := TC{}
	typesList := map[string]json.RawMessage{}

	printTypedData(typedData, typedDataList, &typesList)

	d, err := json.Marshal(typesList)

	if err != nil {
		return nil
	}

	//primaryType := map[string]json.RawMessage{
	//	exampleType.Elem().Name(): json.RawMessage(d),
	//}
	//
	//d, err = json.Marshal(primaryType)
	//
	//if err != nil {
	//	return nil
	//}

	tc.JSON = d
	fmt.Printf("%s\n", d)

	//	_ = txnBodyEntries
	//_ = txnHeaderEntries
	/*
		signer := new(signing.Builder)
		// In reality this would not work, but *shrug* it's a marshalling test
		signer.Type = SignatureTypeEip712TypedData
		signer.Url = originUrl
		signer.SetPrivateKey(key)
		signer.Version = 1
		//provide a deterministic timestamp
		signer.SetTimestamp(uint64(1234567890))
		env := new(messaging.Envelope)
		txn := new(Transaction)

		env.Transaction = []*Transaction{txn}
		txn.Header.Principal = originUrl
		txn.Body = body

		if *useSimpleHash {
			signer.InitMode = signing.InitWithSimpleHash
		}

		sig, err := signer.Initiate(txn)
		if err != nil {
			panic(err)
		}

		env.Signatures = append(env.Signatures, sig)
		return sdktest.NewTxnTest(env)

	*/
	return &tc
}
