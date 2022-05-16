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
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}
