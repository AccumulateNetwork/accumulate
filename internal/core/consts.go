package core

import "regexp"

const SnapshotVersion1 = 1
const SnapshotMajorFormat = "snapshot-major-block-%09d.bpt"

var SnapshotMajorRegexp = regexp.MustCompile(`snapshot-major-block-\d+.bpt`)
