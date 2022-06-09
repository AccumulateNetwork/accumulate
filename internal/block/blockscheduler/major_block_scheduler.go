package blockscheduler

import (
	"time"

	"github.com/gorhill/cronexpr"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
)

const debugMajorBlocks = false

type majorBlockScheduler struct {
	majorBlockSchedule *cronexpr.Expression
	nextMajorBlockTime time.Time
}
type MajorBlockScheduler interface {
	GetNextMajorBlockTime() time.Time
	UpdateNextMajorBlockTime()
	IsInitialized() bool
}

func (s *majorBlockScheduler) GetNextMajorBlockTime() time.Time {
	return s.nextMajorBlockTime
}

func (s *majorBlockScheduler) UpdateNextMajorBlockTime() {
	if debugMajorBlocks {
		s.nextMajorBlockTime = time.Now().UTC().Truncate(time.Second).Add(20 * time.Second)
	} else {
		s.nextMajorBlockTime = s.majorBlockSchedule.Next(time.Now().UTC())
	}
}

func Init(eventBus *events.Bus) *majorBlockScheduler {
	scheduler := &majorBlockScheduler{}
	events.SubscribeAsync(eventBus, scheduler.onDidChangeGlobals)
	return scheduler
}

func (s *majorBlockScheduler) onDidChangeGlobals(event events.DidChangeGlobals) {
	s.majorBlockSchedule = cronexpr.MustParse(event.Values.Globals.MajorBlockSchedule)
	s.nextMajorBlockTime = time.Time{}
}

func (s *majorBlockScheduler) IsInitialized() bool {
	if debugMajorBlocks {
		return true
	}
	if s.majorBlockSchedule == nil {
		return false
	}

	if s.nextMajorBlockTime.IsZero() {
		s.UpdateNextMajorBlockTime()
	}
	return true
}
