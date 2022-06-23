package events

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
)

type Event interface {
	isEvent()
}

func (DidCommitBlock) isEvent()    {}
func (DidSaveSnapshot) isEvent()   {}
func (WillChangeGlobals) isEvent() {}

type DidCommitBlock struct {
	Index uint64
	Time  time.Time
	Major uint64
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}

type WillChangeGlobals struct {
	New, Old *core.GlobalValues
}
