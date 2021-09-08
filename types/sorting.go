package types

import "bytes"

//------------------------------------------------
// Byte array sorting - ascending
type ByByteArray [][]byte

func (f ByByteArray) Len() int {
	return len(f)
}
func (f ByByteArray) Less(i, j int) bool {
	return bytes.Compare(f[i], f[j]) < 0
}
func (f ByByteArray) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
