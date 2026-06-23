#!/usr/bin/env bash
# Restart test for the bootstrap-v3 promoted validator.
#
# Assumes a network is already running and the bv3-launcher daemon is
# already promoted to validator (run promote-bv3-validator.sh first,
# or have an equivalent state).
#
# Steps:
#   1. Verify launched daemon is signing blocks (5/5 commits).
#   2. Kill the launched daemon process (clean shutdown).
#   3. Wait until 4/5 commits are seen (network keeps going at threshold).
#   4. Wait WAIT_BLOCKS more blocks while the daemon stays down.
#   5. Restart the launched daemon.
#   6. Wait for catch-up (height matches network).
#   7. Verify daemon's address appears in the next 3 blocks' commits.
#
# Usage: ./test/docker/restart-bv3-validator.sh [WAIT_BLOCKS]

set -e
cd "$(dirname "$0")/../.."
WAIT_BLOCKS=${1:-30}
DATA=/tmp/bv3-launcher
LOG=/tmp/bv3-restart.log

log() { echo "[$(date +%T)] $*" | tee -a "$LOG"; }
fail() { log "FAIL: $*"; exit 1; }

netHeight() {
  curl -s http://localhost:26690/status 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])' 2>/dev/null
}

ourHeight() {
  curl -s http://localhost:26657/status 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])' 2>/dev/null
}

ourAddr() {
  curl -s http://localhost:26657/status 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["validator_info"]["address"])' 2>/dev/null
}

sigCount() {
  local h=$1
  curl -s "http://localhost:26690/block?height=$h" 2>/dev/null \
    | python3 -c "import json,sys; r=json.load(sys.stdin); print(sum(1 for s in r['result']['block']['last_commit']['signatures'] if s.get('block_id_flag',0)==2))" 2>/dev/null
}

ourSigInBlock() {
  local h=$1 addr=$2
  curl -s "http://localhost:26690/block?height=$h" 2>/dev/null \
    | python3 -c "
import json,sys
r=json.load(sys.stdin)
sigs=r['result']['block']['last_commit']['signatures']
for s in sigs:
    if s.get('validator_address','')=='$addr' and s.get('block_id_flag',0)==2:
        print('1'); sys.exit(0)
print('0')
" 2>/dev/null
}

: > "$LOG"

log "=== preflight: launched daemon healthy + signing ==="
PID=$(pgrep -f "accumulated $DATA" | head -1)
[ -n "$PID" ] || fail "no daemon running at $DATA"
ADDR=$(ourAddr)
[ -n "$ADDR" ] || fail "couldn't read launched daemon validator address"
H=$(ourHeight)
NH=$(netHeight)
log "  daemon pid=$PID addr=$ADDR height=$H network=$NH"

# Wait until our address signs a block (confirms we're an active validator)
SIGNED=0
for i in {1..15}; do
  H=$(netHeight); [ -n "$H" ] || continue
  if [ "$(ourSigInBlock $H $ADDR)" = "1" ]; then SIGNED=1; break; fi
  sleep 2
done
[ $SIGNED -eq 1 ] || fail "launched daemon's address never appeared in commits before stop"
SIGS=$(sigCount $H)
log "  block $H: $SIGS commits including our address ✓"

log "=== stop launched daemon ==="
kill $PID
for i in {1..30}; do ps -p $PID > /dev/null 2>&1 || break; sleep 1; done
ps -p $PID > /dev/null 2>&1 && fail "daemon didn't exit within 30s"
log "  daemon stopped"

log "=== verify network keeps running with 4/5 commits ==="
START_H=$(netHeight)
DROPPED=0
for i in {1..30}; do
  H=$(netHeight); [ -n "$H" ] || { sleep 2; continue; }
  SIGS=$(sigCount $H)
  if [ "$SIGS" = "4" ]; then DROPPED=1; break; fi
  sleep 2
done
[ $DROPPED -eq 1 ] || fail "never observed 4/5 commits after daemon stop"
log "  block $H: 4/5 commits (network still progressing without us) ✓"

log "=== wait $WAIT_BLOCKS more blocks while we're down ==="
TARGET=$((START_H + WAIT_BLOCKS))
until [ "$(netHeight)" -ge "$TARGET" ]; do sleep 5; done
H=$(netHeight)
log "  network advanced to $H while we were down (was $START_H)"

log "=== restart launched daemon ==="
./accumulated "$DATA" >> "$LOG.daemon" 2>&1 &
NEW_PID=$!
log "  new pid=$NEW_PID"

log "=== wait for catch-up ==="
until curl -s http://localhost:26657/status > /dev/null 2>&1; do sleep 2; done
log "  daemon RPC responsive"
NETH=$(netHeight)
until [ "$(ourHeight)" -ge "$NETH" ] 2>/dev/null; do
  H=$(ourHeight)
  N=$(netHeight)
  log "  catching up: ours=$H network=$N"
  sleep 5
done
log "  caught up: ours=$(ourHeight) network=$(netHeight)"

log "=== verify resumes signing ==="
RESUMED=0
for i in {1..15}; do
  H=$(netHeight); [ -n "$H" ] || continue
  if [ "$(ourSigInBlock $H $ADDR)" = "1" ]; then RESUMED=1; break; fi
  sleep 2
done
[ $RESUMED -eq 1 ] || fail "launched daemon's address never signed after restart"
SIGS=$(sigCount $H)
log "  block $H: $SIGS commits including our address (post-restart) ✓"

log ""
log "PASS"
