#!/usr/bin/env bash
# Launch the 12-node DAG-BFT docker network (3 BVNs x 4 validators) referenced
# by test/STABILITY_TEST_PLAN.md.
#
# The network is defined in test/docker/docker-compose.yml: a bootstrap node
# (peer discovery, REQUIRED), an init job that generates configs from
# test/docker/docker-network.yml, and 12 validators with their APIs mapped to
# host ports 26660-26671 (bvn1-val1 .. bvn3-val4).
#
# Usage:
#   ./test/run-dagbft-network.sh up        # build, start, wait until READY
#   ./test/run-dagbft-network.sh status    # container + API + block height
#   ./test/run-dagbft-network.sh logs [svc]
#   ./test/run-dagbft-network.sh down      # stop and remove volumes
#
# Once READY, run a load test against any node, e.g. 50 TPS for 30 minutes:
#   go run ./test/cmd/load -s http://127.0.0.1:26660/v2 -t 50 -d 1800 -dashboard

set -uo pipefail
cd "$(dirname "$0")/.." # repo root

COMPOSE="docker compose -f test/docker/docker-compose.yml"
LOG=/tmp/dagbft-network
mkdir -p "$LOG"
PORTS=(26660 26661 26662 26663 26664 26665 26666 26667 26668 26669 26670 26671)

log() { echo "[$(date +%T)] $*"; }

# The node API is JSON-RPC over POST (GET routes are not served)
api_ok() {
	curl -sf -m 5 -X POST -H 'Content-Type: application/json' \
		-d '{"jsonrpc":"2.0","id":1,"method":"describe","params":{}}' \
		"http://127.0.0.1:$1/v2" 2>/dev/null | grep -q '"result"'
}

# bvn_height PORT BVN -> the BVN's block height (system ledger index), or NA
bvn_height() {
	curl -sf -m 5 -X POST -H 'Content-Type: application/json' \
		-d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"query\",\"params\":{\"scope\":\"acc://bvn-$2.acme/ledger\"}}" \
		"http://127.0.0.1:$1/v3" 2>/dev/null |
		python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["account"]["index"])' 2>/dev/null || echo NA
}

case "${1:-up}" in
up)
	log "cleaning any previous network state"
	$COMPOSE down -v >"$LOG/down.log" 2>&1

	log "building images (first build takes several minutes; log: $LOG/build.log)"
	$COMPOSE build >"$LOG/build.log" 2>&1 || {
		echo "build failed; see $LOG/build.log"
		exit 1
	}

	log "starting bootstrap + init + 12 validators (log: $LOG/up.log)"
	$COMPOSE up -d >"$LOG/up.log" 2>&1 || {
		echo "up failed; see $LOG/up.log"
		exit 1
	}

	log "waiting for all 12 node APIs"
	for i in $(seq 1 60); do
		ok=0
		for p in "${PORTS[@]}"; do api_ok "$p" && ok=$((ok + 1)); done
		[ "$ok" -eq 12 ] && break
		log "  $ok/12 APIs up"
		sleep 5
	done
	[ "$ok" -eq 12 ] || {
		echo "FAIL: only $ok/12 node APIs responded; try: $0 status, $0 logs"
		exit 1
	}
	log "all 12 node APIs up"

	log "waiting for block production"
	h0=$(bvn_height 26660 BVN1)
	for i in $(seq 1 30); do
		sleep 4
		h=$(bvn_height 26660 BVN1)
		if [ "$h" != NA ] && [ "$h0" != NA ] && [ "$h" -gt "$h0" ]; then
			log "READY: blocks are being produced (height $h0 -> $h)"
			log "load test: go run ./test/cmd/load -s http://127.0.0.1:26660/v2 -t 50 -d 1800 -dashboard"
			exit 0
		fi
		[ "$h0" = NA ] && h0=$h
	done
	echo "FAIL: nodes are up but the block height is not advancing (height=$h)"
	echo "inspect with: $0 logs bvn1-val1"
	exit 1
	;;

status)
	$COMPOSE ps
	for pb in 26660:BVN1 26664:BVN2 26668:BVN3; do
		p=${pb%%:*} b=${pb##*:}
		echo "$b (port $p): api=$(api_ok "$p" && echo up || echo DOWN) height=$(bvn_height "$p" "$b")"
	done
	;;

logs)
	shift
	$COMPOSE logs --tail 100 "$@"
	;;

down)
	$COMPOSE down -v
	;;

*)
	echo "usage: $0 [up|down|status|logs [service]]"
	exit 1
	;;
esac
