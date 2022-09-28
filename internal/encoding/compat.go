package encoding

import (
	"io"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type (
	Error           = encoding.Error
	Reader          = encoding.Reader
	Writer          = encoding.Writer
	EnumValueGetter = encoding.EnumValueGetter
	EnumValueSetter = encoding.EnumValueSetter
	BinaryValue     = encoding.BinaryValue
)

func NewReader(r io.Reader) *Reader { return encoding.NewReader(r) }
func NewWriter(r io.Writer) *Writer { return encoding.NewWriter(r) }

func SplitDuration(d time.Duration) (sec, ns uint64) {
	return encoding.SplitDuration(d)
}
