package events

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
)

type Event interface {
	isEvent()
}

func (DidCommitBlock) isEvent()   {}
func (DidSaveSnapshot) isEvent()  {}
func (DidChangeGlobals) isEvent() {}

type DidCommitBlock struct {
	Index uint64
	Time  time.Time
	Major uint64
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}

type DidChangeGlobals struct {
	Values *core.GlobalValues
}
