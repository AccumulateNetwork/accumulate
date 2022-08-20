package blockscheduler

import (
	"time"

	"github.com/gorhill/cronexpr"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type majorBlockScheduler struct {
	majorBlockSchedule *cronexpr.Expression
	nextMajorBlockTime time.Time
}
type MajorBlockScheduler interface {
	GetNextMajorBlockTime(time.Time) time.Time
	UpdateNextMajorBlockTime(time.Time)
	IsInitialized() bool
}

func (s *majorBlockScheduler) GetNextMajorBlockTime(blockTime time.Time) time.Time {
	if s.nextMajorBlockTime.IsZero() {
		s.UpdateNextMajorBlockTime(blockTime)
	}
	return s.nextMajorBlockTime
}

func (s *majorBlockScheduler) UpdateNextMajorBlockTime(blockTime time.Time) {
	s.nextMajorBlockTime = s.majorBlockSchedule.Next(blockTime.UTC())
}

func Init(eventBus *events.Bus) *majorBlockScheduler {
	scheduler := &majorBlockScheduler{}
	events.SubscribeSync(eventBus, scheduler.onWillChangeGlobals)
	return scheduler
}

func (s *majorBlockScheduler) onWillChangeGlobals(event events.WillChangeGlobals) (err error) {
	s.majorBlockSchedule, err = cronexpr.Parse(event.New.Globals.MajorBlockSchedule)
	s.nextMajorBlockTime = time.Time{}
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (s *majorBlockScheduler) IsInitialized() bool {
	return s.majorBlockSchedule != nil
}
