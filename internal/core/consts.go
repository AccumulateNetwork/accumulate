package core

import (
	"regexp"

	"github.com/robfig/cron/v3"
)

const SnapshotMajorFormat = "snapshot-major-block-%09d.bpt"

var SnapshotMajorRegexp = regexp.MustCompile(`snapshot-major-block-\d+.bpt`)

var Cron = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
