// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"fmt"
	"math"
	"strings"

	"github.com/robfig/cron/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var CronFormat = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

type globalValueMemos struct {
	bvns               []string
	threshold          map[string]uint64
	active             map[string]int
	majorBlockSchedule cron.Schedule
}

func (g *GlobalValues) memoizeValidators() {
	if g.memoize.active != nil {
		return
	}

	var bvns []string
	for _, p := range g.Network.Partitions {
		if p.Type == protocol.PartitionTypeBlockValidator {
			bvns = append(bvns, p.ID)
		}
	}

	active := make(map[string]int, len(g.Network.Partitions))
	for _, v := range g.Network.Validators {
		for _, p := range v.Partitions {
			if p.Active {
				active[strings.ToLower(p.ID)]++
			}
		}
	}

	threshold := make(map[string]uint64, len(g.Network.Partitions))
	for partition, active := range active {
		threshold[partition] = g.Globals.ValidatorAcceptThreshold.Threshold(active)
	}

	g.memoize.bvns = bvns
	g.memoize.active = active
	g.memoize.threshold = threshold
}

func (g *GlobalValues) BvnExecutorVersion() protocol.ExecutorVersion {
	if len(g.BvnExecutorVersions) == 0 {
		return 0
	}

	v := g.ExecutorVersion
	for _, b := range g.BvnExecutorVersions {
		if b.Version < v {
			v = b.Version
		}
	}
	return v
}

func (g *GlobalValues) BvnNames() []string {
	g.memoizeValidators()
	return g.memoize.bvns
}

func (g *GlobalValues) ValidatorThreshold(partition string) uint64 {
	g.memoizeValidators()
	v, ok := g.memoize.threshold[strings.ToLower(partition)]
	if !ok {
		return math.MaxUint64
	}
	return v
}

func (g *GlobalValues) MajorBlockSchedule() cron.Schedule {
	if g.memoize.majorBlockSchedule != nil {
		return g.memoize.majorBlockSchedule
	}
	s, err := CronFormat.Parse(g.Globals.MajorBlockSchedule)
	if err != nil {
		panic(fmt.Errorf("cannot parse major block schedule: %w", err))
	}
	g.memoize.majorBlockSchedule = s
	return s
}
