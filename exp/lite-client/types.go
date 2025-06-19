package liteclient

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

type VerifiedAccount struct {
	Url     string
	Receipt *merkle.Receipt
	Height  int64
}
