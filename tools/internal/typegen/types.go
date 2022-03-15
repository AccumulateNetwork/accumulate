package typegen

import (
	"sort"
	"strings"
)

type DataTypes []*DataType

func DataTypesFrom(m map[string]*DataType) DataTypes {
	t := DataTypes{}
	for name, typ := range m {
		typ.Name = name
		t = append(t, typ)

		if typ.TxType == "" {
			if typ.Kind == "tx-result" && strings.HasSuffix(name, "Result") {
				typ.TxType = name[0 : len(name)-6]
			} else {
				typ.TxType = name
			}
		}

		if typ.ChainType == "" {
			typ.ChainType = name
		}

		if typ.SignatureType == "" {
			if strings.HasSuffix(typ.Name, "Signature") {
				typ.SignatureType = name[0 : len(name)-9]
			} else {
				typ.SignatureType = name
			}
		}
	}

	sort.Slice(t, func(i, j int) bool {
		return t[i].Name < t[j].Name
	})

	return t
}

type DataType struct {
	Name          string `yaml:"-"`
	Description   string
	Kind          string
	TxType        string `yaml:"tx-type"`
	ChainType     string `yaml:"chain-type"`
	SignatureType string `yaml:"signature-type"`
	NonBinary     bool   `yaml:"non-binary"`
	Incomparable  bool   `yaml:"incomparable"`
	OmitNewFunc   bool   `yaml:"omit-new-func"`
	Fields        []*Field
	Embeddings    []string `yaml:"embeddings"`
}

func (typ *DataType) GoTxType() string {
	return "protocol.TransactionType" + typ.TxType
}

func (typ *DataType) GoChainType() string {
	return "types.ChainType" + typ.ChainType
}

type Field struct {
	Name          string
	Description   string
	Type          string
	MarshalAs     string `yaml:"marshal-as"`
	UnmarshalWith string `yaml:"unmarshal-with"`
	Repeatable    bool
	Pointer       bool
	Optional      bool
	KeepEmpty     bool `yaml:"keep-empty"`
	Alternative   string
	ZeroValue     interface{} `yaml:"zero-value"`
}

type API map[string]Method

type Method struct {
	Kind        string
	Description string
	Deprecated  string
	RPC         string
	Input       string
	Output      string
	Call        string
	CallParams  []string `yaml:"call-params"`
	Validate    []string `yaml:"validate"`
}

type Type map[string]*TypeValue

type TypeValue struct {
	Value       interface{}
	Description string
	Aliases     []string
}
