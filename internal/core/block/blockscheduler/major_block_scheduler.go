// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package blockscheduler

import (
	"time"

	"github.com/robfig/cron/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type majorBlockScheduler struct {
	majorBlockSchedule cron.Schedule
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
	s.majorBlockSchedule, err = core.Cron.Parse(event.New.Globals.MajorBlockSchedule)
	s.nextMajorBlockTime = time.Time{}
	return errors.UnknownError.Wrap(err)
}

func (s *majorBlockScheduler) IsInitialized() bool {
	return s.majorBlockSchedule != nil
}
