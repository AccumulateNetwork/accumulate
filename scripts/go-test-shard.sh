#!/bin/bash
# go-test-shard.sh PACKAGE [go test flags...]
#
# Runs this CI job's share of one package's tests. GitLab numbers the jobs of
# a `parallel:` job CI_NODE_INDEX of CI_NODE_TOTAL; each runs every
# CI_NODE_TOTAL-th test, example and fuzz target that `go test -list` names,
# so every one of them runs in exactly one job. Without the variables it runs
# them all.
set -euo pipefail

pkg=$1
shift
index=${CI_NODE_INDEX:-1}
total=${CI_NODE_TOTAL:-1}

names=$(go test "$@" -list '.*' "$pkg" | grep -E '^(Test|Example|Fuzz)' |
    awk -v i="$index" -v n="$total" '(NR - 1) % n == i - 1')
if [ -z "$names" ]; then
    echo "$pkg: nothing for job $index of $total"
    exit 0
fi
echo "$pkg: job $index of $total runs $(echo "$names" | wc -l) tests:" $names

exec go run gotest.tools/gotestsum --junitfile report.xml --format testname -- \
    "$@" -run "^($(echo $names | tr ' ' '|'))\$" "$pkg"
