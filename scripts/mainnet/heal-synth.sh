#!/usr/bin/env bash
# Continuous synthetic-transaction healer for mainnet (#4064 stopgap).
#
# The production conductor heals anchors only — it does NOT auto-heal synthetic
# transactions, so a single dropped DN->BVN synthetic wedges the sequence until
# an operator heals it (incident 2026-07-18: 84 staking-reward txs stuck at
# Directory->Cyclops #5315). This runs the existing, tested heal tool in a loop
# so any wedge self-clears within a minute. Run as a sidecar service on the
# operator/monitoring box — NOT on the validator; it touches no consensus code.
#
# Prereq: build the debug tool from a branch whose BootstrapServers is current:
#   go build -o /usr/local/bin/acc-debug ./tools/cmd/debug
set -uo pipefail

NETWORK="${NETWORK:-mainnet}"
DEBUG_BIN="${DEBUG_BIN:-acc-debug}"
# Current bootstrap peer. Passed explicitly so a stale hardcoded peer map
# (accman regenerated this once — accman#37) cannot break healing. Update if the
# bootstrap identity changes again.
BOOTSTRAP="${BOOTSTRAP:-/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg}"
CACHE="${CACHE:-$HOME/.accumulate-heal}"
mkdir -p "$CACHE"

exec "$DEBUG_BIN" heal synth "$NETWORK" \
  --continuous --wait=false --since 0 \
  --bootstrap "$BOOTSTRAP" \
  --light-db "$CACHE/heal.db" --peer-db "$CACHE/peers.json"
