package typegen

import (
	"sort"
)

type Types []*Type

func TypesFrom(m map[string]*Type) Types {
	t := Types{}
	for name, typ := range m {
		typ.Name = name
		t = append(t, typ)

		if typ.TxType == "" {
			typ.TxType = typ.Name
		}

		if typ.ChainType == "" {
			typ.ChainType = typ.Name
		}

		for _, f := range typ.Fields {
			if f.Slice != nil {
				f.Slice.Name = f.Name
			}
		}
	}

	sort.Slice(t, func(i, j int) bool {
		return t[i].Name < t[j].Name
	})

	return t
}

type Type struct {
	Name         string `yaml:"-"`
	Kind         string
	TxType       string `yaml:"tx-type"`
	ChainType    string `yaml:"chain-type"`
	NonBinary    bool   `yaml:"non-binary"`
	Incomparable bool   `yaml:"incomparable"`
	Fields       []*Field
	Embeddings   []string `yaml:"embeddings"`
}

func (typ *Type) GoTxType() string {
	return "types.TxType" + typ.TxType
}

func (typ *Type) GoChainType() string {
	return "types.ChainType" + typ.ChainType
}

type Field struct {
	Name          string
	Type          string
	MarshalAs     string `yaml:"marshal-as"`
	UnmarshalWith string `yaml:"unmarshal-with"`
	Slice         *Field
	Pointer       bool
	Optional      bool
	IsUrl         bool `yaml:"is-url"`
	KeepEmpty     bool `yaml:"keep-empty"`
	Alternative   string
}

type API map[string]Method

type Method struct {
	Kind       string
	RPC        string
	Input      string
	Output     string
	Call       string
	CallParams []string `yaml:"call-params"`
	Validate   []string `yaml:"validate"`
}
