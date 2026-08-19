#!/usr/bin/env bash
# Watch the soak for a synthetic channel seizing (churn), then exit so the
# session is re-invoked to debug it. Healthy healing keeps stuck<=~1; a seizure
# is the wedge tx failing to clear (stuck climbs) or a persistent, growing gap.
here="$(cd "$(dirname "$0")" && pwd)"
STUCK_TRIP=5       # heal_stuck_tries at/above this = churn (healthy transient is 1)
GAP_TRIP=60        # a synthetic stream gap this large that persists = seizure
UNDELIV_TRIP=20    # consecutive polls (~15 min) with produced > received = stalled stream
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
und=None
for kind in ("synthetic","anchor"):
  for s,row in ((d.get("matrix") or {}).get("flows",{}).get(kind) or {}).items():
    for dd,c in row.items():
      u=(c.get("sent",0) or 0)-(c.get("recv",0) or 0)
      if u>0 and (und is None or u>und[2]): und=(kind,"%s->%s"%(s,dd),u)
ws="%s->%s gap=%d deliv=%s"%(worst[0],worst[1],worst[2],worst[3]) if worst else "none"
us="%s %s undeliv=%d"%(und[0],und[1],und[2]) if und else "none"
print("stuck=%d stuckStream=%s worst=%s undeliv=%s"%(stuck,h.get("stuckStream",""),ws,us))
' 2>/dev/null)"
  ts="$(date -u +%FT%T)"
  echo "$ts $verdict"
  stuck="$(printf '%s' "$verdict" | grep -oE 'stuck=[0-9]+' | head -1 | cut -d= -f2)"
  wgap="$(printf '%s' "$verdict" | grep -oE 'gap=[0-9]+' | head -1 | cut -d= -f2)"
  undeliv="$(printf '%s' "$verdict" | grep -oE 'undeliv=[0-9]+' | head -1 | cut -d= -f2)"
  stuck="${stuck:-0}"; wgap="${wgap:-0}"; undeliv="${undeliv:-0}"

  # produced-at-source minus received-at-destination. This is the only signal
  # that sees a lost PREFIX: when the first messages of a stream are dropped and
  # no later message ever follows, the destination forms no pending window, so
  # recv==deliv==0 and every gap-based check reads healthy forever. Require
  # persistence so ordinary in-flight messages do not trip it.
  if [ "$undeliv" -gt 0 ]; then und_streak=$(( ${und_streak:-0} + 1 )); else und_streak=0; fi

  if [ "$stuck" -ge "$STUCK_TRIP" ] || [ "$wgap" -ge "$GAP_TRIP" ]; then
    echo "SEIZED at $ts :: $verdict"
    exit 0
  fi
  if [ "$und_streak" -ge "$UNDELIV_TRIP" ]; then
    echo "SEIZED at $ts :: stalled stream, undelivered for $und_streak polls :: $verdict"
    exit 0
  fi
  sleep 45
done
echo "watch window elapsed with no seizure"
