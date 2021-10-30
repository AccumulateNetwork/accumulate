package types

type QueryType uint64

//QueryType enumeration order matters, do not change order when adding new enums.
const (
	QueryTypeUnknown      = QueryType(iota)
	QueryTypeUrl          // Query by Url
	QueryTypeChainId      // Query by chain id
	QueryTypeTxId         // Query tx and pending chains By TxId
	QueryTypeTxHistory    // Query transaction history
	QueryTypeDirectoryUrl // Query directory by URL

)

// Enum value maps for QueryType.
var (
	QueryTypeName = map[QueryType]string{
		QueryTypeUnknown:      "QueryTypeUnknown",
		QueryTypeUrl:          "QueryTypeUrl",
		QueryTypeChainId:      "QueryTypeChainId",
		QueryTypeTxId:         "QueryTypeTxId",
		QueryTypeTxHistory:    "QueryTypeTxHistory",
		QueryTypeDirectoryUrl: "QueryTypeDirectoryUrl",
	}
	QueryTypeValue = map[string]QueryType{
		"QueryTypeUnknown":      QueryTypeUnknown,
		"QueryTypeUrl":          QueryTypeUrl,
		"QueryTypeChainId":      QueryTypeChainId,
		"QueryTypeTxId":         QueryTypeTxId,
		"QueryTypeTxHistory":    QueryTypeTxHistory,
		"QueryTypeDirectoryUrl": QueryTypeDirectoryUrl,
	}
)

//Name will return the name of the type
func (t QueryType) Name() string {
	if name := QueryTypeName[t]; name != "" {
		return name
	}
	return QueryTypeUnknown.Name()
}

//SetType will set the type based on the string name submitted
func (t *QueryType) SetType(s string) {
	*t = QueryTypeValue[s]
}

//AsUint64 casts as a uint64
func (t QueryType) AsUint64() uint64 {
	return uint64(t)
}

func (t QueryType) String() string { return t.Name() }
