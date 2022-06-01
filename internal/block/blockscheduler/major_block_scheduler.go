package blockscheduler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
	events.SubscribeAsync(eventBus, scheduler.onDidDataAccountUpdate)
	return scheduler
}

func (s *majorBlockScheduler) onDidDataAccountUpdate(event events.DidDataAccountUpdate) {
	path := event.AccountUrl.Path
	if strings.HasPrefix(path, "/") { // TODO URL.JoinPath does not include a / while when URL is parsed by the CLI it does
		path = path[1:]
	}
	switch {
	case path == protocol.Globals:
		globals := new(protocol.NetworkGlobals)
		entry := *event.DataEntry
		err := json.Unmarshal(entry.GetData()[0], globals)
		if err != nil {
			panic(fmt.Errorf("network globals data entry is corrupt: %v", err))
		}
		s.update(globals)
	}

}

func (s *majorBlockScheduler) IsInitialized() bool {
	if s.majorBlockSchedule == nil {
		return false
	}

	if s.nextMajorBlockTime.IsZero() {
		s.UpdateNextMajorBlockTime()
	}
	return true
}

func (s *majorBlockScheduler) update(globals *protocol.NetworkGlobals) {
	s.majorBlockSchedule = cronexpr.MustParse(globals.MajorBlockSchedule)
	s.nextMajorBlockTime = time.Time{}
}
