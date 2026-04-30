#!/usr/bin/env bash
# End-to-end test: wipe network, fresh up, bootstrap-v3 a follower,
# promote it to validator, verify.
#
# Tracking issue: #4012. Sub-issues: #4013 (promote tool), #4014
# (this script), #4015 (verifications).
#
# Run from repo root:
#   ./test/docker/promote-bv3-validator.sh

set -e
cd "$(dirname "$0")/../.."

LOG=/tmp/bv3-promote.log
DATA=/tmp/bv3-launcher
COMPOSE="docker compose -f test/docker/docker-compose.bootstrap-v3.yml"

log() { echo "[$(date +%T)] $*" | tee -a "$LOG"; }
fail() { log "FAIL: $*"; exit 1; }

: > "$LOG"

log "=== build binaries ==="
go build -o accumulated ./cmd/accumulated/ >>"$LOG" 2>&1 || fail "build accumulated"
go build -o promote-validator ./tools/cmd/promote-validator/ >>"$LOG" 2>&1 || fail "build promote-validator"
go build -o keyhash ./tools/cmd/keyhash/ >>"$LOG" 2>&1 || fail "build keyhash"

log "=== kill any prior launched daemon ==="
pkill -9 -f "accumulated $DATA" 2>/dev/null || true
sleep 2

log "=== wipe + fresh docker ==="
$COMPOSE down -v >>"$LOG" 2>&1
$COMPOSE up -d >>"$LOG" 2>&1 || fail "docker up"

log "=== wait for network ready ==="
until curl -s http://localhost:26690/status 2>/dev/null | grep -q '"latest_block_height"'; do sleep 3; done
log "network ready, height=$(curl -s http://localhost:26690/status | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])')"

log "=== wait for at least 3 major-block snapshots (so signed anchors propagate) ==="
until [ "$(curl -s http://localhost:26680/v3/snapshot/directory 2>/dev/null | wc -l)" -ge 3 ]; do sleep 10; done
log "snapshots: $(curl -s http://localhost:26680/v3/snapshot/directory | tr '\n' ' ')"

log "=== run bootstrap-v3 launcher ==="
rm -rf "$DATA"; mkdir -p "$DATA"
timeout 60 ./accumulated bootstrap \
  --network BootstrapV3Test --partition Directory \
  --via-snapshot http://localhost:26680 \
  --peer ws://localhost:26680/v3 \
  --peer-anchor-pool acc://bvn-bvn1.acme/anchors \
  --tm-rpc-servers http://localhost:26690,http://localhost:26691 \
  --tm-p2p-peers localhost:26696,localhost:26697 \
  --data-dir "$DATA" >>"$LOG" 2>&1 || fail "bootstrap"
log "bootstrap done"

log "=== start launched daemon ==="
./accumulated "$DATA" >>"$LOG.daemon" 2>&1 &
DAE=$!
echo "$DAE" > /tmp/bv3-pid

log "=== wait for daemon RPC ==="
until curl -s http://localhost:26657/status 2>/dev/null | grep -q '"sync_info"'; do sleep 2; done

log "=== wait for catch-up to current network height ==="
TARGET=$(curl -s http://localhost:26690/status | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])')
until [ "$(curl -s http://localhost:26657/status 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])' 2>/dev/null)" -ge "$TARGET" ]; do
  sleep 5
done
LH=$(curl -s http://localhost:26657/status | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])')
log "launched node caught up to $LH (target was $TARGET)"

log "=== extract launched validator AS1 key ==="
VALADDR=$(awk '/\[configurations.validator-key\]/ {found=1; next} found && /address = / {print; exit}' "$DATA/accumulate.toml" | sed -E 's/.*"(AS1[^"]+)".*/\1/')
[ -n "$VALADDR" ] || fail "couldn't extract validator address from accumulate.toml"
log "new validator AS1: $VALADDR"

log "=== extract docker validator operator keys ==="
OP1=$(docker exec acc-bv3-bvn1-val1 grep 'address = "AS1' /root/.accumulate/bvn1-1/accumulate.toml | head -1 | sed -E 's/.*"(AS1[^"]+)".*/\1/')
OP2=$(docker exec acc-bv3-bvn1-val2 grep 'address = "AS1' /root/.accumulate/bvn1-2/accumulate.toml | head -1 | sed -E 's/.*"(AS1[^"]+)".*/\1/')
OP3=$(docker exec acc-bv3-bvn1-val3 grep 'address = "AS1' /root/.accumulate/bvn1-3/accumulate.toml | head -1 | sed -E 's/.*"(AS1[^"]+)".*/\1/')
log "op1=$OP1"
log "op2=$OP2"
log "op3=$OP3"

log "=== submit promotion ==="
set -o pipefail
./promote-validator \
  --endpoint ws://localhost:26680/v3 \
  --network BootstrapV3Test \
  --partition Directory \
  --new-validator-key "$VALADDR" \
  --operator-key "$OP1" --operator-key "$OP2" --operator-key "$OP3" \
  2>&1 | tee -a "$LOG"
PROMOTE_RC=${PIPESTATUS[0]}
set +o pipefail
[ "$PROMOTE_RC" -eq 0 ] || fail "promotion submit (rc=$PROMOTE_RC)"
log "promotion submitted"

log "=== compute keyhash for verification ==="
NEWHASH=$(./keyhash "$VALADDR") || fail "compute keyhash"
log "looking for keyhash $NEWHASH in operators/1"

log "=== verify validator promotion ==="
for i in {1..30}; do
  if curl -s -X POST http://localhost:26680/v3 -H 'Content-Type: application/json' \
      -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://dn.acme/operators/1","query":{"queryType":"default"}}}' \
      | grep -qi "$NEWHASH"; then
    log "VERIFIED: new key in operators/1 keypage"
    break
  fi
  sleep 5
  [ "$i" -eq 30 ] && fail "timeout waiting for keypage update"
done

log "=== verify network definition includes new validator ==="
for i in {1..30}; do
  if curl -s -X POST http://localhost:26680/v3 -H 'Content-Type: application/json' \
      -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://dn.acme/network","query":{"queryType":"default"}}}' \
      | grep -qi "$NEWPUB"; then
    log "VERIFIED: new key in network definition"
    break
  fi
  sleep 5
  [ "$i" -eq 30 ] && fail "timeout waiting for network update"
done

log "=== check CometBFT validator set on val1 ==="
curl -s "http://localhost:26690/validators?height=0" 2>/dev/null | python3 -c "
import json,sys
r = json.load(sys.stdin)
vs = r['result']['validators']
print(f'validators in network: {len(vs)}')
for v in vs:
    print(' ', v['address'], 'voting_power='+v['voting_power'])
" | tee -a "$LOG"

log "=== summary ==="
log "  daemon process: $(ps -p $DAE -o pid,etime= 2>&1 | tail -1)"
log "  launched node height: $(curl -s http://localhost:26657/status | python3 -c 'import json,sys; print(json.load(sys.stdin)[\"result\"][\"sync_info\"][\"latest_block_height\"])')"
log "  network height: $(curl -s http://localhost:26690/status | python3 -c 'import json,sys; print(json.load(sys.stdin)[\"result\"][\"sync_info\"][\"latest_block_height\"])')"
log ""
log "PASS"
