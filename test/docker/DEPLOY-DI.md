# Deploying the DAG-BFT network on the lab server

This is the CometBFT-free path. The existing `synth-heal/DEPLOY-REMOTE.md` covers
the CometBFT rig on `main`; almost none of its *mechanics* change here, but the
image, the compose file, the load generator and the verification all do.

Everything below has been run end to end on `76-fun`. Where something bit, it is
recorded as a trap rather than smoothed over.

## Status: the network deploys, and does not yet work

**It builds, starts, forms consensus, and then stops writing blocks under load.**
See **#4103**. Deploy it to work on that; do not deploy it expecting a usable
network. What is true today:

| | |
|---|---|
| Image builds | yes, since #4101 |
| 13 containers start and report healthy | yes |
| Consensus forms and commits | yes — Bullshark commits continuously |
| Blocks are written to the ledger | **no, once load is applied** |
| Container health reflects any of this | **no** — 13/13 healthy while the chain is stopped |

## The host

| | |
|---|---|
| SSH alias | **`thelio-fast`** — 10.42.0.2, direct wired link |
| Alternative | `thelio` / `76-fun` — 192.168.86.122, WiFi |
| Capacity | 24 cores, 91 GB RAM |
| Repo | `~/go/src/gitlab.com/AccumulateNetwork/accumulate` |
| Go | `/usr/local/go/bin/go` — **only on a login shell**, see traps |
| Docker | 29.7.2, user `paul` in the `docker` group |

## Procedure

### 1. Put the branch on the host

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate
  git fetch --force origin
  git reset --hard origin/<branch>
  git log --oneline -1
"'
```

`--force` and no `&&` chaining, for the reason `DEPLOY-REMOTE.md` documents: this
host holds three tags that diverge from origin, so a plain `git fetch --tags`
exits non-zero and silently short-circuits anything chained after it.

### 2. Build the image

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate
  docker build --build-arg GIT_DESCRIBE=\$(git describe --dirty --tags --always) \
               --build-arg GIT_COMMIT=\$(git rev-parse HEAD) \
               -t acc-dagbft:di -f Dockerfile .
"'
```

Roughly 420 MB. The compose file builds inline from the same `Dockerfile`, so
this step is really a pre-warm — but it surfaces build failures where you can
read them, instead of inside a compose log.

**This only works from #4101 onward.** Before that fix the Dockerfile still ran
`go install github.com/cometbft/cometbft/cmd/cometbft`, whose module the CometBFT
removal had already deleted from `go.mod`, so every image build on this branch
failed. CI builds binaries and never builds the image, which is why the removal
merged with this broken.

### 3. Start the network — WITH the monitor, always

**There is no supported way to run this network without the monitor, and the
monitor means one the OPERATOR IS WATCHING — not a process on the server.**
The standing requirement is that the dashboard is shown, live, before load
starts. The order is fixed: open the tunnel first, verify the URL answers from
the operator's machine, hand it over, and only then start the test. A run
whose dashboard was only reachable after it ended fails the requirement just
as completely as a run with no monitor at all — both have happened on this
harness, and both are why this paragraph exists.

`up.sh` brings the network and the monitor up as one action, builds before
`up` (see the stale-image trap below), and tears the network back down if the
monitor fails to start rather than running blind:

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker
  ./up.sh
"'
```

For a soak, `soak/soak.sh` does the same and more. A bare `docker compose up`
is for the compose file's own debugging only, and never for a run whose
outcome anyone will read.

13 containers: `acc-bootstrap` plus 12 validators, 4 per BVN across 3 BVNs.

### 4. Verify it is actually running — health is not enough

**Do not trust `healthy`.** Every container reported healthy throughout #4103,
with the chain stopped. The only verification that means anything is that block
height advances:

```bash
scp test/docker/blockrate.py thelio-fast:/tmp/
ssh thelio-fast 'bash -lc "python3 /tmp/blockrate.py --api http://localhost:26660/v3 --duration 60 --interval 10"'
```

It reads each partition's system ledger index and reports blocks/sec. A partition
at `0.0 b/s` is stopped, whatever `docker ps` says. `blockrate.py` exits non-zero
when nothing advanced anywhere, so it can gate a script.

### 5. Apply load

Build the generator once, then run it detached:

```bash
ssh thelio-fast 'bash -lc "cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker && go build -o /tmp/loadtest parallel-loadtest.go"'
ssh thelio-fast 'cd /tmp && setsid nohup /tmp/loadtest -start-tps 2 -end-tps 2 -duration 3600s > /tmp/lt.log 2>&1 < /dev/null &'
```

Check the rate it actually achieved, not the one requested:

```bash
ssh thelio-fast 'tail -2 /tmp/lt.log'
# Progress: submitted=384 success=384 failure=0 tps_total=2 target=2
```

`tps_total` must match `target`. Before #4102 it did not: the generator divided
the target by its 48 workers in integer arithmetic and clamped up, so every
target below 48 produced 48 TPS while still printing the number you asked for.

**`success` counts accepted submissions, not executed transactions.** During
#4103 it climbed steadily at zero failures while no blocks were written. It is
not a liveness signal.

### 6. Tear down

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker
  docker compose down -v --remove-orphans
  docker ps -aq --filter name=acc- | xargs -r docker rm -f
"'
```

Collect container logs **before** this if the run is worth anything — they die
with the containers and are the only record of what the nodes did.

## Traps

### Go is invisible to a non-interactive SSH command

`/usr/local/go/bin` is added to PATH by `~/.bashrc` and `~/.profile`, neither of
which a non-interactive SSH command sources:

```bash
ssh thelio-fast 'go version'              # "command not found"
ssh thelio-fast 'bash -lc "go version"'   # go1.24.4
```

I recorded "no Go toolchain on the server" on this basis and was wrong. **Always
`bash -lc`.**

### Backgrounding over SSH kills the process

`nohup ... &` inside an `ssh` command dies when the session closes, silently and
with no log written. Use `setsid nohup ... < /dev/null &`, or tmux.

### tmux through nested SSH quoting is not worth it

`tmux new-session -d -s x "bash -lc \"...\""` through an SSH command needs three
levels of escaping and failed silently twice here — no session, no log, no error.
`setsid nohup` is one level and works. If you want tmux, ssh in and start it
interactively.

### The host saturates, and then SSH commands time out

Twelve validators alone drove load average to **28–32 on 24 cores**, each node
holding 47–58% of a core. At that point longer SSH commands return nothing at
all rather than failing visibly. Keep remote commands short, and treat "no
output" as "the box is busy", not "the command did nothing".

Memory is not the constraint: 604 MB of a 4 GB limit on the busiest node.

### Block indices in log errors are not the ledger height

```
ERROR Loading block ledger failed
  cannot locate ledger for block 31953   (BVN1 ledger index: 3991)
```

Consecutive indices, tens of thousands above the real height, tens of thousands
of times per minute. That gap is #4103's central symptom, not noise to filter.

## What to measure, and what is not measured

`blockrate.py` gives block production, which is the one signal that would have
caught #4103 early. Everything the synth-heal monitor says about wedges, pull
errors, stuck and deferred is **not measured** on any branch — those panels read
Prometheus metrics no node exports (#4094, #4095), and display `0` regardless.
That applies here too. See `synth-heal/soak/MONITOR-AUDIT.md`.
