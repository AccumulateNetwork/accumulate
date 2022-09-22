package core

import "regexp"

const SnapshotMajorFormat = "snapshot-major-block-%09d.bpt"

var SnapshotMajorRegexp = regexp.MustCompile(`snapshot-major-block-\d+.bpt`)
