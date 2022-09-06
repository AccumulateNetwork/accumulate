package core

import "regexp"

const SnapshotMajorFormat = "snapshot-major-block-%09d.bpt"
const SnapshotHeaderFormat = "snapshot-major-block-%09d-header.json"
const SnapshotBlockCommitFormat = "snapshot-major-block-%09d-commit.json"
const SnapshotBlockFormat = "snapshot-major-block-%09d-block.json"

var SnapshotMajorRegexp = regexp.MustCompile(`snapshot-major-block-\d+.bpt`)
