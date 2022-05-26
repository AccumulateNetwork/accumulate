package events

import "time"

type Event interface {
	isEvent()
}

func (DidCommitBlock) isEvent()  {}
func (DidSaveSnapshot) isEvent() {}

type DidCommitBlock struct {
	Index uint64
	Time  time.Time
	Major uint64
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}
