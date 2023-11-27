// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"encoding/json"
	"time"

	"github.com/robfig/cron/v3"
)

type CronSchedule struct {
	src   string
	inner cron.Schedule
}

func ParseCron(spec string) (*CronSchedule, error) {
	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		return nil, err
	}
	return &CronSchedule{spec, schedule}, nil
}

func MustParseCron(spec string) *CronSchedule {
	s, err := ParseCron(spec)
	if err != nil {
		panic(err)
	}
	return s
}

func (s *CronSchedule) Next(t time.Time) time.Time {
	return s.inner.Next(t)
}

func (s *CronSchedule) Copy() *CronSchedule {
	return s // CronSchedule is immutable so there's no need to copy
}

func (s *CronSchedule) Equal(r *CronSchedule) bool {
	return s.src == r.src
}

func (s *CronSchedule) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.src)
}

func (s *CronSchedule) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &s.src)
	if err != nil {
		return err
	}

	s.inner, err = cron.ParseStandard(s.src)
	return err
}
