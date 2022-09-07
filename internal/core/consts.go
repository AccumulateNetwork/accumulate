package core

import "regexp"

const SnapshotMajorFormat = "snapshot-major-block-%09d.bpt"
const SnapshotTmStateFormat = "snapshot-major-block-%09d.state"

var SnapshotMajorRegexp = regexp.MustCompile(`snapshot-major-block-\d+.state`)
