package simulator

import (
	"time"
)

type fakeMajorBlockScheduler struct {
	nextMajorBlockTime time.Time
}

func (f *fakeMajorBlockScheduler) GetNextMajorBlockTime(blockTime time.Time) time.Time {
	return f.nextMajorBlockTime
}

func (f *fakeMajorBlockScheduler) UpdateNextMajorBlockTime(blockTime time.Time) {
	f.nextMajorBlockTime = blockTime.Add(72 * time.Hour)
}

func (f *fakeMajorBlockScheduler) IsInitialized() bool {
	return true
}

func InitFakeMajorBlockScheduler(genesisTime time.Time) *fakeMajorBlockScheduler {
	scheduler := &fakeMajorBlockScheduler{
		nextMajorBlockTime: genesisTime.Add(time.Hour * 12),
	}
	return scheduler
}
