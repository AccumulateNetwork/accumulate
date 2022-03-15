package protocol

import (
	"encoding"
)

//maybe we should have Chain header then entry, rather than entry containing all the Headers

// Deprecated: don't use this anymore
func (o *Object) As(entry encoding.BinaryUnmarshaler) error {
	return entry.UnmarshalBinary(o.Entry)
}
