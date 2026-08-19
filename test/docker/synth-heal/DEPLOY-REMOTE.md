# Running a soak on the lab server, over SSH

The synth-heal rig is 13 containers and will saturate a laptop for as long as
it runs. Run it on **thelio** instead. This is the procedure, including the
five things that have actually gone wrong on this host.

## The host

| | |
|---|---|
| SSH alias | **`thelio-fast`** — 10.42.0.2, a direct 1 Gb wired link |
| Alternative | `thelio` / `76-fun` — 192.168.86.122, over WiFi |
| Hostname | `76-fun` |
| Capacity | 24 cores, 91 GB RAM |
| Repo | `~/go/src/gitlab.com/AccumulateNetwork/accumulate` |
| Docker | 29.7.2, user `paul` is in the `docker` group |
| Go | **`/usr/local/go/bin/go`** — see the PATH trap below |
| tmux | present |

Use `thelio-fast`. A 220 MB image ships across it in about three seconds.

## Procedure

### 1. Put the code on the host

The rig runs `soak.sh` and builds the loadgen with `go run` from the repo, so
the **repo must match the image**. Check out the exact thing under test:

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate
  git fetch --tags --force origin
  git checkout -q v1.4.6.3
  git describe --dirty
"'
```

Note `--force`, and **no `&&` chaining**. This host holds three tags that
diverge from origin (`v1.4.3-fix-the-fix`, `v1.5.0-experimental`,
`v1.5.0_experimental`), so a plain `git fetch --tags` prints
`! [rejected] ... (would clobber existing tag)` and **exits non-zero** — which
silently short-circuits anything chained after it with `&&`. The fetch has
otherwise succeeded; only those three tags are skipped.

If `git describe --dirty` reports `-dirty`, see "Provenance" below before
building — the label ends up in the run manifest.

### 2. Get the image onto the host

Either build there:

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate
  docker build --build-arg GIT_DESCRIBE=\$(git describe --dirty) \
               --build-arg GIT_COMMIT=\$(git rev-parse HEAD) \
               -t acc-synthheal:test -f Dockerfile .
"'
```

…or ship one you already built, which is faster and guarantees parity:

```bash
docker save acc-synthheal:test | ssh thelio-fast 'docker load'
```

`docker load` re-serialises the manifest, so the **image ID will differ** on
the two hosts. That is not a mismatch. Verify content, not IDs:

```bash
docker run --rm --entrypoint sh acc-synthheal:test -c 'sha256sum $(command -v accumulated)'
```

Run that on both sides; the hashes must match.

### 3. Launch, detached

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker/synth-heal/soak
  tmux new-session -d -s soak \"bash -lc \\\"cd \$PWD && ./soak.sh why-i-am-running-this\\\"\"
"'
```

**Do not pass `DURATION` unless you have a reason.** `soak.sh` defaults to
`DURATION=24h TPS=2`, which is the intended shape of a real run. Overriding it
to something shorter produces a run that looks complete and is not.

### 4. Watch it

`soakmon` binds `127.0.0.1` on thelio, so tunnel it from your workstation and
leave the tunnel up for the whole run:

```bash
ssh -N -L 8099:127.0.0.1:8099 thelio-fast
```

Then <http://127.0.0.1:8099>.

**Never leave the network running without the monitor attached, and never
leave it running with no load driving it.** `soak.sh` does the opposite on
exit — it kills `soakmon` and leaves the containers up — so after a run you
must either re-attach a monitor or tear down. An unobserved network burning
CPU is not a test; it is just heat.

### 5. Evaluate, then tear down

Evaluate **before** tearing down, because container logs die with the
containers and they are the only record of induced drops:

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker/synth-heal/soak
  RD=\$(readlink -f runs/latest); mkdir -p \$RD/container-logs
  for n in \$(docker ps -a --filter name=acc- --format \"{{.Names}}\"); do
    docker logs \$n > \$RD/container-logs/\$n.log 2>&1
  done
  grep -hcE \"dropping (synthetic|sequenced) envelope\" \$RD/container-logs/*.log | paste -sd+ | bc
"'
```

Then:

```bash
ssh thelio-fast 'bash -lc "
  cd ~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker/synth-heal
  docker compose -f soak/docker-compose.yml down -v --remove-orphans
  docker ps -aq --filter name=acc- | xargs -r docker rm -f
"'
```

Run artifacts under `runs/<id>/` are the permanent record and survive
teardown. The containers are not.

## Traps, all of which have bitten

### Go is installed but invisible to SSH

`/usr/local/go/bin` is added to PATH by `~/.bashrc` and `~/.profile`, neither
of which a **non-interactive** SSH command sources. So this lies:

```bash
$ ssh thelio-fast 'command -v go'      # prints nothing
$ ssh thelio-fast 'bash -lc "go version"'
go version go1.24.4 linux/amd64        # it was there all along
```

A `tmux new-session` inherits that same stripped PATH. Launching `soak.sh`
without `bash -lc` gets you:

```
nohup: failed to run command 'go': No such file or directory
== soak finished driver-exit=127 ==
```

39 seconds after start, with 13 healthy containers and no load. **Always
launch through `bash -lc`.**

### `pgrep -f` matches your own command line

`pgrep -f soak.sh` run over SSH matches the SSH command *containing* that
string, so it reports processes that do not exist — and `pkill -f soak.sh`
kills your own session (exit 255) before killing anything else. Use
`ps -eo args | grep "[s]oakmon"`, `pgrep -x`, or check the listening port:

```bash
ss -tln | grep :8099        # soakmon is up, definitively
```

### The manifest says `-dirty` after every run

`soak.sh` appends each run's row to `runs/INDEX.md`, a **tracked** file. So a
completed run leaves the tree dirty and the *next* run records
`v1.4.6.3-dirty`. Not a mystery and not a problem — but if you want clean
provenance, stash or commit that row before building.

### Timestamps mix UTC and local

`soak.sh`'s own start/finish lines are UTC; the loadgen's progress lines are
**local**. A run starting `00:53 UTC` whose last progress line reads `04:00`
did not stop three hours in — that is `04:00 CDT` = `09:00 UTC`, right at the
8-hour mark. Compute elapsed from the run directory, not by comparing the two.

## What the results mean

Read `soak/MONITOR-AUDIT.md` before quoting any number. In short:

**Trustworthy** — heal counts, block heights, loadgen stats, and drop counts
derived from container logs.

**Not measured** — wedges, pull errors, stuck, deferred, and the whole flow
matrix. Those panels read Prometheus metrics no node exports (accumulate#4094,
#4095), so they display `0` regardless of what happened. A verdict that quotes
"0 wedges" is quoting nothing.
