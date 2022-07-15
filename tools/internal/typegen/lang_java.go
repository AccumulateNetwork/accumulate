package typegen

func (f FieldType) JavaType() string {
	switch f.Code {
	case TypeCodeUnknown:
		return f.Name
	case TypeCodeBytes:
		return "byte[]"
	case TypeCodeRawJson:
		return "RawJson"
	case TypeCodeUrl:
		return "Url"
	case TypeCodeTxid:
		return "TxID"
	case TypeCodeBigInt:
		return "BigInteger"
	case TypeCodeUint:
		return "long"
	case TypeCodeInt:
		return "int"
	case TypeCodeHash:
		return "byte[]"
	case TypeCodeDuration:
		return "Duration"
	case TypeCodeTime:
		return "Time"
	case TypeCodeAny:
		return "Object"
	case TypeCodeFloat:
		return "float"
	default:
		return f.Code.String()
	}
}
