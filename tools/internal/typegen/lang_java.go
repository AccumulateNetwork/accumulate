package typegen

import "strings"

func (f FieldType) JavaType() string {
	switch f.Code {
	case TypeCodeUnknown:
		split := strings.SplitAfter(f.Name, ".")
		return split[len(split)-1]
	case TypeCodeString:
		return "String"
	case TypeCodeBytes:
		return "byte[]"
	case TypeCodeRawJson:
		return "RawJson"
	case TypeCodeUrl:
		return "Url"
	case TypeCodeTxid:
		return "TxID"
	case TypeCodeBigInt:
		return "java.math.BigInteger"
	case TypeCodeUint:
		return "long"
	case TypeCodeInt:
		return "int"
	case TypeCodeHash:
		return "byte[]"
	case TypeCodeDuration:
		return "Duration"
	case TypeCodeTime:
		return "java.time.OffsetDateTime"
	case TypeCodeAny:
		return "JsonNode"
	case TypeCodeBool:
		return "boolean"
	case TypeCodeFloat:
		return "float"
	default:
		return f.Code.String()
	}
}
