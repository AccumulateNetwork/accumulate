package build

import "time"

func Signature() SignatureBuilder { return SignatureBuilder{} }

func Transaction(principal any, path ...string) TransactionBuilder {
	return TransactionBuilder{}.WithPrincipal(principal, path...)
}

// UnixTimeNow returns the current time as a number of milliseconds since the
// Unix epoch. This is the recommended timestamp value.
func UnixTimeNow() uint64 {
	return uint64(time.Now().UTC().UnixMilli())
}
