package events

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Event interface {
	isEvent()
}

func (DidCommitBlock) isEvent()       {}
func (DidSaveSnapshot) isEvent()      {}
func (DidDataAccountUpdate) isEvent() {}

type DidCommitBlock struct {
	Index uint64
	Time  time.Time
	Major uint64
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}

type DidDataAccountUpdate struct {
	AccountUrl *url.URL
	DataEntry  *protocol.DataEntry
}
