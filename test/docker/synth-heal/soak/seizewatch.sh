#!/usr/bin/env bash
# Watch the soak for a synthetic channel seizing (churn), then exit so the
# session is re-invoked to debug it. Healthy healing keeps stuck<=~1; a seizure
# is the wedge tx failing to clear (stuck climbs) or a persistent, growing gap.
here="$(cd "$(dirname "$0")" && pwd)"
STUCK_TRIP=5       # heal_stuck_tries at/above this = churn (healthy transient is 1)
GAP_TRIP=60        # a synthetic stream gap this large that persists = seizure
prev_deliv=""
for i in $(seq 1 2000); do
  data="$(curl -s -m5 http://127.0.0.1:8099/data 2>/dev/null)"
  verdict="$(printf '%s' "$data" | python3 -c '
import sys,json
try: d=json.load(sys.stdin)
except Exception: print("NODATA"); sys.exit()
h=d.get("heals") or {}
mx=(d.get("matrix") or {}).get("flows",{}).get("synthetic",{})
stuck=h.get("stuck",0) or 0
worst=None
for s,row in mx.items():
  for dd,c in row.items():
    g=(c.get("recv",0) or 0)-(c.get("deliv",0) or 0)
    if worst is None or g>worst[2]: worst=(s,dd,g,c.get("deliv",0))
ws="%s->%s gap=%d deliv=%s"%(worst[0],worst[1],worst[2],worst[3]) if worst else "none"
print("stuck=%d stuckStream=%s worst=%s"%(stuck,h.get("stuckStream",""),ws))
' 2>/dev/null)"
  ts="$(date -u +%FT%T)"
  echo "$ts $verdict"
  stuck="$(printf '%s' "$verdict" | grep -oE 'stuck=[0-9]+' | head -1 | cut -d= -f2)"
  wgap="$(printf '%s' "$verdict" | grep -oE 'gap=[0-9]+' | head -1 | cut -d= -f2)"
  stuck="${stuck:-0}"; wgap="${wgap:-0}"
  if [ "$stuck" -ge "$STUCK_TRIP" ] || [ "$wgap" -ge "$GAP_TRIP" ]; then
    echo "SEIZED at $ts :: $verdict"
    exit 0
  fi
  sleep 45
done
echo "watch window elapsed with no seizure"
