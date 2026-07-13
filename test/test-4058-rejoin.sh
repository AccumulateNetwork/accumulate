#!/usr/bin/env bash
# #4058: fast-sync rejoin past the DAG GC horizon.
#
# A DAG-BFT node's consensus state is in-memory only: a restarted node begins
# at round zero and catches up round-by-round from its peers, who only retain
# the last DAGGCDepth rounds. Once the outage (or the network's age) exceeds
# that window, plain restart wedges forever. This test proves the fastsync
# rejoin path fixes it:
#
#   1. Run the 12-node network with a small DN GC depth.
#   2. Stop one validator and let the network run far past the horizon.
#   3. Control: restart it plainly and confirm it wedges.
#   4. Fastsync its directory database (verified spine -> proven state) and
#      write the consensus rejoin seed.
#   5. Restart and confirm it rejoins: seeds the round, catches up, and
#      produces directory blocks again.
#
# The victim's BVN side is expected to stay wedged — BVN fastsync is #4058
# phase 3b (BVN roots ride the DN spine via PartitionAnchorReceipt).
#
# Usage: ./test/test-4058-rejoin.sh [--skip-build]

set -uo pipefail
cd "$(dirname "$0")/.." # repo root

COMPOSE="docker compose -f test/docker/docker-compose.yml"
LOG=/tmp/4058-rejoin
GC_DEPTH=${GC_DEPTH:-300}
VICTIM=bvn1-val4
VDIR=bvn1-4
OUTAGE=${OUTAGE:-180} # seconds; rounds advance ~5-10/s, so this is >> GC_DEPTH
mkdir -p "$LOG"

log() { echo "[$(date +%T)] $*" | tee -a "$LOG/test.log"; }
fail() { log "FAIL: $*"; exit 1; }

dn_height() {
    curl -s -m 5 -X POST "http://127.0.0.1:${1:-26660}/v3" \
        -H 'Content-Type: application/json' \
        -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://dn.acme/ledger"}}' \
        | jq -r '.result.account.index // 0' 2>/dev/null || echo 0
}

# ——— 1. Build and initialize ———————————————————————————————————————————————

if [ "${1:-}" != "--skip-build" ]; then
    log "Building images (this takes a few minutes)"
    $COMPOSE build >"$LOG/build.log" 2>&1 || fail "build failed — see $LOG/build.log"
fi

log "Tearing down any previous network"
$COMPOSE down -v >>"$LOG/compose.log" 2>&1

log "Starting bootstrap and generating configs"
$COMPOSE up -d bootstrap >>"$LOG/compose.log" 2>&1
$COMPOSE up init >"$LOG/init.log" 2>&1 || fail "init failed — see $LOG/init.log"

log "Setting dag-gc-depth=$GC_DEPTH on every node"
$COMPOSE run --rm --no-deps --entrypoint sh bvn1-val1 -c "
    for f in /root/.accumulate/*/accumulate.toml; do
        sed -i 's/^  type = \"coreValidator\"/  dag-gc-depth = $GC_DEPTH\n  type = \"coreValidator\"/' \$f
    done
    grep -c dag-gc-depth /root/.accumulate/*/accumulate.toml" >"$LOG/inject.log" 2>&1 \
    || fail "config injection failed — see $LOG/inject.log"

log "Starting validators"
$COMPOSE up -d >>"$LOG/compose.log" 2>&1

log "Waiting for the network to produce blocks"
for i in $(seq 1 60); do
    h=$(dn_height)
    [ "${h:-0}" -gt 5 ] && break
    [ "$i" = 60 ] && fail "network never started producing blocks"
    sleep 5
done
log "DN height: $(dn_height)"

# ——— 2. Traffic + outage past the horizon ——————————————————————————————————

log "Starting traffic"
nohup go run ./test/cmd/heal-traffic -s http://127.0.0.1:26660/v3 -i 1s -d 60m \
    >"$LOG/traffic.log" 2>&1 &
TRAFFIC=$!
trap 'kill $TRAFFIC 2>/dev/null' EXIT
sleep 60

H0=$(dn_height)
log "Stopping $VICTIM at DN height $H0"
$COMPOSE stop $VICTIM >>"$LOG/compose.log" 2>&1

log "Outage: ${OUTAGE}s — rounds will pass far beyond the $GC_DEPTH-round horizon"
sleep "$OUTAGE"
H1=$(dn_height)
log "DN height advanced $H0 -> $H1 during the outage"
[ "$H1" -gt "$H0" ] || fail "the network did not advance during the outage"

# ——— 3. Control: a plain restart must wedge ————————————————————————————————

log "Control: plain restart of $VICTIM (expected to wedge)"
# Raw docker start — `compose start` would re-run the init dependency
docker start "acc-$VICTIM" >>"$LOG/compose.log" 2>&1
sleep 90
docker logs "acc-$VICTIM" --since 2m >"$LOG/control.log" 2>&1
CONTROL_BLOCKS=$(grep -c 'Produced block' "$LOG/control.log" || true)
log "Control: produced-block lines after plain restart: $CONTROL_BLOCKS (expected 0)"
$COMPOSE stop $VICTIM >>"$LOG/compose.log" 2>&1

# ——— 4. Fastsync the victim's directory database ———————————————————————————

# The TOML-advertised peer IDs do not match the nodes' actual API p2p
# identities — ask the node itself
PEERID=$(curl -s -m 5 -X POST http://127.0.0.1:26660/v3 \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"node-info","params":{}}' | jq -r '.result.peerID // empty')
[ -n "$PEERID" ] || fail "could not determine bvn1-val1's peer ID"
PEER="/dns/acc-bvn1-val1/tcp/26658/p2p/$PEERID"
log "Fastsync $VICTIM's directory database (peer $PEER)"
$COMPOSE run --rm --no-deps $VICTIM fastsync http://acc-bvn1-val1:26660 \
    --genesis "/root/.accumulate/$VDIR/directory-genesis.snap" \
    --database "/root/.accumulate/$VDIR/dnn/data/accumulate.db" \
    --storage leveldb \
    --partition Directory \
    --rejoin-dir "/root/.accumulate/$VDIR" \
    --peer "$PEER" --node "$PEERID" >"$LOG/fastsync.log" 2>&1 \
    || fail "fastsync failed — see $LOG/fastsync.log"
grep -E 'Synced and verified|Epoch block|Rejoin seed|State tree anchor' "$LOG/fastsync.log" | tee -a "$LOG/test.log"

# ——— 5. Restart and verify the rejoin ——————————————————————————————————————

log "Restarting $VICTIM with the rejoin seed"
docker start "acc-$VICTIM" >>"$LOG/compose.log" 2>&1
sleep 120
docker logs "acc-$VICTIM" --since 3m >"$LOG/rejoin.log" 2>&1

grep -E 'Seeded consensus for fast-sync rejoin|Rejoined consensus' "$LOG/rejoin.log" | tee -a "$LOG/test.log"
REJOIN_BLOCKS=$(grep -c 'Produced block' "$LOG/rejoin.log" || true)
log "Rejoin: produced-block lines after fastsync restart: $REJOIN_BLOCKS (expected > 0)"

# ——— Verdict ———————————————————————————————————————————————————————————————

kill $TRAFFIC 2>/dev/null
if [ "$CONTROL_BLOCKS" -eq 0 ] && [ "$REJOIN_BLOCKS" -gt 0 ]; then
    log "PASS: plain restart wedged, fastsync rejoin recovered"
    log "Network left running — './test/run-dagbft-network.sh down' to tear down"
    exit 0
fi
fail "control=$CONTROL_BLOCKS (want 0) rejoin=$REJOIN_BLOCKS (want >0) — see $LOG"
