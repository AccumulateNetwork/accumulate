#!/usr/bin/env python3
"""Drive lint and test fixes to green with one Claude session per task.

A plan (plan.json in the state directory) lists tasks. Each task is a lint
check or a group of Go tests on a branch. The monitor runs in tmux window
"monitor" of session "ci-fix". It gives every task its own git worktree and
a tmux window running Claude with the task's prompt. When a worker reports
done, the monitor runs the task's check itself, `passes` times in a row. It
closes the window once the check holds, and sends the worker the failure if
it does not.

When every task on the fix branch holds, the monitor merges their branches
into the fix branch and runs the gate locally: lint plus every test package
the tasks touch, whole. It then pushes the branch and waits for CI. A red
gate gets its own worker. Green CI closes every session.

Workers use the same script:
    ci-fix.py check <task>              run the task's check in the cwd
    ci-fix.py report <task> --state done --note "..."
    ci-fix.py slot -- go test ...       run a heavy command in a shared slot

Operator:
    ci-fix.py plan ...                  write plan.json (see --help)
    ci-fix.py run                       start the tmux session and open it
    ci-fix.py status                    print the table once
    ci-fix.py stop [--keep-worktrees]   close every session
"""

import argparse
import datetime as dt
import fcntl
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import threading
import time
import uuid
from pathlib import Path

SCRIPT = Path(__file__).resolve()
PROMPTS = SCRIPT.parent / "prompts"
SESSION = "ci-fix"


def sh(cmd, cwd=None, check=False, capture=True, env=None, timeout=None):
    r = subprocess.run(cmd, cwd=cwd, shell=isinstance(cmd, str), text=True,
                       capture_output=capture, env=env, timeout=timeout)
    if check and r.returncode != 0:
        raise SystemExit(f"command failed ({r.returncode}): {cmd}\n{r.stdout}{r.stderr}")
    return r


def repo_root():
    top = sh(["git", "rev-parse", "--show-toplevel"], check=True).stdout.strip()
    common = sh(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                cwd=top, check=True).stdout.strip()
    return Path(common).parent  # the main checkout, also from inside a worktree


ROOT = repo_root()
STATE = ROOT / ".git" / "ci-fix"
PLAN = STATE / "plan.json"
EVENTS = STATE / "events.log"


def now():
    return dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def event(msg):
    STATE.mkdir(parents=True, exist_ok=True)
    with open(EVENTS, "a") as f:
        f.write(f"{now()} {msg}\n")


def load_json(p, default=None):
    try:
        return json.loads(Path(p).read_text())
    except (FileNotFoundError, json.JSONDecodeError):
        return default


def save_json(p, v):
    p = Path(p)
    p.parent.mkdir(parents=True, exist_ok=True)
    tmp = p.with_suffix(".tmp")
    tmp.write_text(json.dumps(v, indent=2))
    tmp.replace(p)


def load_plan():
    plan = load_json(PLAN)
    if not plan:
        raise SystemExit(f"no plan at {PLAN}; run `ci-fix.py plan` first")
    return plan


def task_dir(tid):
    return STATE / "tasks" / tid


def task_state(tid):
    return load_json(task_dir(tid) / "state.json", {})


def set_task_state(tid, **kv):
    st = task_state(tid)
    st.update(kv)
    st["updated"] = now()
    save_json(task_dir(tid) / "state.json", st)
    return st


# ---------------------------------------------------------------- slots

class Slot:
    """One of N machine-wide run slots, so parallel sessions running heavy
    tests do not overload the box into false timeouts."""

    def __init__(self, n):
        self.n, self.f = n, None

    def __enter__(self):
        (STATE / "slots").mkdir(parents=True, exist_ok=True)
        while True:
            for i in range(self.n):
                f = open(STATE / "slots" / str(i), "w")
                try:
                    fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    self.f = f
                    return self
                except BlockingIOError:
                    f.close()
            time.sleep(2)

    def __exit__(self, *a):
        fcntl.flock(self.f, fcntl.LOCK_UN)
        self.f.close()


# ---------------------------------------------------------------- checks

LINT_LINE = re.compile(r"^(\S+\.go):(\d+):(\d+): (.*)$")


def changed_files(cwd, base):
    r = sh(["git", "diff", "--name-only", f"{base}...HEAD"], cwd=cwd)
    return set(r.stdout.split())


def tree_clean(cwd):
    return sh(["git", "status", "--porcelain"], cwd=cwd).stdout.strip() == ""


def run_logged(cmd, cwd, log, env=None, timeout=None):
    with open(log, "a") as f:
        f.write(f"\n$ {cmd if isinstance(cmd, str) else shlex.join(cmd)}\n")
        f.flush()
        try:
            r = subprocess.run(cmd, cwd=cwd, shell=isinstance(cmd, str), text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               env=env, timeout=timeout)
            out, code = r.stdout, r.returncode
        except subprocess.TimeoutExpired as e:
            out = (e.stdout or b"").decode() if isinstance(e.stdout, bytes) else (e.stdout or "")
            code = 124
        f.write(out)
    return code, out


def check_lint(task, cwd, log):
    """CI's lint job: golangci-lint, then (on a clean tree) tidy, generate
    and imports. Scope "changed" keeps only findings in files the branch
    changes, for MR branches that must not fix the base's findings."""
    problems = []
    code, out = run_logged(["go", "run", "./tools/cmd/golangci-lint", "run", "--timeout", "20m"],
                           cwd, log)
    found = [m for m in (LINT_LINE.match(l) for l in out.splitlines()) if m]
    if task.get("scope") == "changed":
        mine = changed_files(cwd, task["base"])
        found = [m for m in found if m.group(1) in mine]
    problems += [m.group(0) for m in found]
    if code != 0 and not found and task.get("scope") != "changed":
        problems.append(f"golangci-lint exited {code} with no parsed finding; see {log}")

    if not tree_clean(cwd):
        problems.append("tree not clean: tidy/generate/imports not checked (commit first)")
        return problems
    run_logged(["go", "mod", "tidy"], cwd, log)
    if sh(["git", "diff", "--quiet", "go.mod", "go.sum"], cwd=cwd).returncode != 0:
        problems.append("go.mod/go.sum are not tidy")
    sh(["git", "checkout", "--", "go.mod", "go.sum"], cwd=cwd)
    code, _ = run_logged(["go", "generate", "./..."], cwd, log)
    if code != 0:
        problems.append(f"go generate failed ({code})")
    diff = sh(["git", "status", "--porcelain"], cwd=cwd).stdout.strip()
    if diff:
        problems.append("generated files are not up to date: " + " ".join(
            l[3:] for l in diff.splitlines())[:300])
        sh(["git", "checkout", "--", "."], cwd=cwd)
        sh(["git", "clean", "-fdq"], cwd=cwd)  # the tree was clean before generate
    dirs = [d for d in sorted(os.listdir(cwd)) if os.path.isdir(os.path.join(cwd, d)) and not d.startswith(".")]
    code, out = run_logged(["go", "run", "github.com/rinchsan/gosimports/cmd/gosimports", "-l"] +
                           [d + "/" for d in dirs], cwd, log)
    bad = [l for l in out.splitlines() if l.endswith(".go")]
    if task.get("scope") == "changed":
        mine = changed_files(cwd, task["base"])
        bad = [b for b in bad if b in mine]
    problems += [f"{b}: not gosimports-formatted" for b in bad]
    return problems


FAIL_LINE = re.compile(r"^\s*--- FAIL: (\S+) \(")
HUNG = re.compile(r"panic: test timed out after (\S+)")
RUNNING = re.compile(r"^[ \t]+(Test\S*) \(", re.M)


def check_tests(task, cwd, log):
    """CI's go test job, narrowed to the task's tests: same tags, same
    binary timeout, same environment, a fresh process every run."""
    env = dict(os.environ, ACC_ASSERT_IDENTITY_BOUNDARY="1")
    cmd = ["go", "test", "-tags", "testnet", "-count=1", "-timeout", task.get("timeout", "30m"), "-v"]
    if task.get("tests"):
        cmd += ["-run", "^(" + "|".join(task["tests"]) + ")$"]
    cmd += task["packages"]
    code, out = run_logged(cmd, cwd, log, env=env)
    problems = [f"FAIL {m.group(1)}" for m in (FAIL_LINE.match(l) for l in out.splitlines()) if m]
    hung = HUNG.search(out)
    if hung:
        running = re.search(r"running tests:\n((?:[ \t]+\S+ \(.*\)\n)+)", out)
        problems.append(f"binary timed out after {hung.group(1)}; running: "
                        + (" ".join(RUNNING.findall(running.group(1))) if running else "?"))
    if code != 0 and not problems:
        tail = " | ".join(out.strip().splitlines()[-5:])
        problems.append(f"go test exited {code}: {tail[:400]}")
    return problems


def check_task(task, cwd, repeat=1):
    """Run the task's check `repeat` times, stopping at the first failure.
    Returns (ok, problems, log)."""
    d = task_dir(task["id"]) / "checks"
    d.mkdir(parents=True, exist_ok=True)
    log = d / f"{now()}-{os.getpid()}.log"
    for i in range(repeat):
        with Slot(load_plan().get("slots", 3)):
            start = time.time()
            if task["kind"] == "lint":
                problems = check_lint(task, cwd, log)
            elif task["kind"] == "tests":
                problems = check_tests(task, cwd, log)
            else:  # gate: lint plus every package whole
                problems = check_lint(dict(task, scope="all"), cwd, log)
                if not problems:
                    problems = check_tests(dict(task, tests=None), cwd, log)
        with open(log, "a") as f:
            f.write(f"\n== run {i + 1}/{repeat}: {'PASS' if not problems else 'FAIL'} "
                    f"in {time.time() - start:.0f}s\n" + "\n".join(problems) + "\n")
        if problems:
            return False, [f"run {i + 1}/{repeat}: {p}" for p in problems], log
    return True, [], log


# ---------------------------------------------------------------- CI evidence

def glab_json(path):
    r = sh(["glab", "api", path], cwd=ROOT)
    return json.loads(r.stdout) if r.returncode == 0 and r.stdout.strip() else None


def latest_pipeline(ref=None, sha=None):
    q = f"sha={sha}" if sha else f"ref={ref}"
    ps = glab_json(f"projects/:id/pipelines?{q}&per_page=1") or []
    return ps[0] if ps else None


def failed_jobs(pid):
    jobs = glab_json(f"projects/:id/pipelines/{pid}/jobs?per_page=100") or []
    return [j for j in jobs if j["status"] == "failed" and not j.get("allow_failure")]


STAMP = re.compile(r"^\S+Z \S+ ")


def job_log(jid):
    p = STATE / "ci" / f"{jid}.log"
    if not p.exists():
        p.parent.mkdir(parents=True, exist_ok=True)
        r = sh(["glab", "api", f"projects/:id/jobs/{jid}/trace"], cwd=ROOT)
        p.write_text("\n".join(STAMP.sub("", l) for l in r.stdout.splitlines()))
    return p


def ci_failures(pid):
    """Lint findings and failing tests (package-qualified) from a pipeline."""
    lint, tests, hung = [], set(), set()
    for j in failed_jobs(pid):
        text = job_log(j["id"]).read_text(errors="replace")
        if j["name"] == "lint":
            lint += [m.group(0) for m in (LINT_LINE.match(l) for l in text.splitlines()) if m]
        for m in re.finditer(r"^FAIL (\S+)\.(Test\w+) \(", text, re.M):
            tests.add((m.group(1), m.group(2)))
        for m in re.finditer(r"panic: test timed out[^\n]*\n\s*running tests:\n((?:[ \t]+\S+ \(.*\)\n)+)", text):
            hung.update(RUNNING.findall(m.group(1)))
    return sorted(set(lint)), sorted(tests), sorted(hung)


# ---------------------------------------------------------------- plan

def cmd_plan(a):
    """Build the plan from the latest pipeline on the base, and one lint task
    per MR branch whose own files fail lint."""
    base = a.base
    fix = f"issue-{a.issue}-ci-green"
    tasks = []
    p = latest_pipeline(ref=base.removeprefix("origin/")) if not a.pipeline else {"id": a.pipeline}
    lint, tests, hung = ci_failures(p["id"])
    if a.pipeline_extra:
        for pid in a.pipeline_extra:
            l2, t2, h2 = ci_failures(pid)
            lint, tests, hung = sorted(set(lint + l2)), sorted(set(tests + t2)), sorted(set(hung + h2))
    if lint:
        tasks.append(dict(id="lint", kind="lint", scope="all", findings=lint))
    # Group tests by the file that declares them: one session per file.
    groups = {}
    for pkg, t in tests + [(None, h) for h in hung]:
        if pkg is None:
            pkg = next((p_ for p_, t_ in tests if t_ == t), None) or _find_pkg(t)
        f = _find_file(pkg, t)
        groups.setdefault(f, (pkg, set()))[1].add(t)
    for f, (pkg, ts) in sorted(groups.items()):
        rel = pkg.split("accumulate/", 1)[-1]
        name = Path(f).stem.removesuffix("_test").replace("_", "-") if f else "tests"
        tasks.append(dict(id=f"test-{name}", kind="tests", packages=[f"./{rel}"],
                          tests=sorted(ts), file=f, hung=sorted(set(ts) & set(hung))))
    merged = {}
    for t in a.merge_group or []:
        ids = t.split(",")
        keep = next(x for x in tasks if x["id"] == ids[0])
        for other in [x for x in tasks if x["id"] in ids[1:]]:
            keep["tests"] = sorted(set(keep["tests"]) | set(other["tests"]))
            keep["packages"] = sorted(set(keep["packages"]) | set(other["packages"]))
            keep["file"] += " " + other["file"]
            tasks.remove(other)
            merged[other["id"]] = keep["id"]
    for t in tasks:
        t.update(base=base, target=fix, branch=f"{fix}-{t['id']}",
                 passes=a.passes if t["kind"] == "tests" else 1,
                 model=a.lint_model if t["kind"] == "lint" else a.model,
                 evidence=f"pipeline {p['id']}" + (f" + {a.pipeline_extra}" if a.pipeline_extra else ""))
    for br in a.mr_branch or []:
        mp = latest_pipeline(ref=br)
        if not mp:
            continue
        l, _, _ = ci_failures(mp["id"])
        mine = set(sh(["git", "diff", "--name-only", f"{base}...origin/{br}"], cwd=ROOT).stdout.split())
        l = [x for x in l if x.split(":")[0] in mine]
        if l:
            tasks.append(dict(id=f"lint-{br}", kind="lint", scope="changed", findings=l,
                              base=base, target=None, branch=br, existing=True, passes=1,
                              model=a.lint_model, evidence=f"pipeline {mp['id']}"))
    packages = sorted({p_ for t in tasks if t["kind"] == "tests" for p_ in t["packages"]})
    plan = dict(issue=a.issue, base=base, fix_branch=fix, slots=a.slots,
                max_workers=a.max_workers, push=not a.no_push, packages=packages,
                worktrees=str(Path(a.worktrees).resolve()), effort=a.effort,
                created=now(), tasks=tasks,
                gate=dict(id="gate", kind="gate", base=base, branch=fix, packages=packages,
                          passes=1, model=a.model, scope="all"))
    if PLAN.exists() and not a.force:
        raise SystemExit(f"{PLAN} exists; pass --force to replace it (it resets nothing else)")
    save_json(PLAN, plan)
    print(json.dumps(plan, indent=2))


def _find_pkg(test):
    r = sh(["git", "grep", "-l", f"^func {test}(", "--", "*_test.go"], cwd=ROOT)
    f = r.stdout.split()[0] if r.stdout.split() else ""
    mod = sh(["go", "list", "-m"], cwd=ROOT).stdout.strip()
    return f"{mod}/{Path(f).parent}" if f else mod


def _find_file(pkg, test):
    rel = pkg.split("accumulate/", 1)[-1] if pkg else "."
    r = sh(["git", "grep", "-l", f"^func {test}(", "--", f"{rel}/*_test.go"], cwd=ROOT)
    return r.stdout.split()[0] if r.stdout.split() else ""


# ---------------------------------------------------------------- tmux

def tmux(*args):
    return sh(["tmux", *args])


def windows():
    r = tmux("list-windows", "-t", SESSION, "-F", "#{window_name}")
    return set(r.stdout.split()) if r.returncode == 0 else set()


def tell(win, text):
    """Type one line into a worker's Claude prompt and submit it."""
    tmux("send-keys", "-t", f"{SESSION}:{win}", "-l", text)
    time.sleep(0.5)
    tmux("send-keys", "-t", f"{SESSION}:{win}", "Enter")


# ---------------------------------------------------------------- workers

def worktree_for(plan, task):
    return Path(plan["worktrees"]) / task["id"]


def ensure_worktree(plan, task):
    wt = worktree_for(plan, task)
    if wt.exists():
        return wt
    sh(["git", "fetch", "-q", "origin"], cwd=ROOT)
    wt.parent.mkdir(parents=True, exist_ok=True)
    br = task["branch"]
    local = sh(["git", "rev-parse", "--verify", "-q", f"refs/heads/{br}"], cwd=ROOT).returncode == 0
    if task.get("existing") and not local:
        sh(["git", "worktree", "add", "-b", br, str(wt), f"origin/{br}"], cwd=ROOT, check=True)
    elif local:  # never reset a local branch: it may hold unpushed work
        sh(["git", "worktree", "add", str(wt), br], cwd=ROOT, check=True)
    else:
        sh(["git", "worktree", "add", "-b", br, str(wt), task["base"]], cwd=ROOT, check=True)
    return wt


def render_prompt(plan, task, extra=""):
    others = [t["id"] for t in plan["tasks"] if t["id"] != task["id"]]
    fields = dict(
        task=task["id"], kind=task["kind"], issue=plan["issue"], base=task["base"],
        branch=task["branch"], worktree=str(worktree_for(plan, task)),
        script=str(SCRIPT), others=", ".join(others) or "none",
        passes=task.get("passes", 1), evidence=task.get("evidence", ""),
        findings="\n".join(f"- `{f}`" for f in task.get("findings", [])) or "- (none recorded)",
        tests="\n".join(f"- `{t}`" for t in task.get("tests") or []) or "- (whole packages)",
        hung="\n".join(f"- `{t}`" for t in task.get("hung", [])) or "- none",
        packages=" ".join(task.get("packages", [])), file=task.get("file", ""),
        scope=task.get("scope", "all"), ci_logs=str(STATE / "ci"),
        state=str(task_dir(task["id"])), extra=extra,
    )
    body = (PROMPTS / "common.md").read_text() + "\n" + (PROMPTS / f"{task['kind']}.md").read_text()
    return body.format(**fields)


def spawn(plan, task, extra="", resume=False):
    wt = ensure_worktree(plan, task)
    st = task_state(task["id"])
    sid = st.get("session") or str(uuid.uuid4())
    pf = task_dir(task["id"]) / "prompt.md"
    pf.parent.mkdir(parents=True, exist_ok=True)
    claude = ["claude", "--dangerously-skip-permissions", "--model", task.get("model", "opus"),
              "--effort", plan.get("effort", "high"), "-n", f"ci-fix {task['id']}"]
    if resume and st.get("session"):
        claude += ["--resume", sid, "Your session was restarted by the ci-fix monitor. "
                   "Carry on with your task; report your status when you can."]
    else:
        pf.write_text(render_prompt(plan, task, extra))
        claude += ["--session-id", sid, f"Your task is in {pf}. Read it in full, then start."]
    tmux("kill-window", "-t", f"{SESSION}:{task['id']}")
    tmux("new-window", "-d", "-t", SESSION, "-n", task["id"], "-c", str(wt), shlex.join(claude))
    set_task_state(task["id"], session=sid, phase="working", spawned=now(),
                   respawns=st.get("respawns", 0) + (1 if resume else 0))
    event(f"{task['id']}: worker {'resumed' if resume else 'started'} in {wt}")


# ---------------------------------------------------------------- monitor

class Monitor:
    NUDGE_AFTER = 45 * 60

    def __init__(self):
        self.plan = load_plan()
        self.busy = {}  # task id -> validation thread
        self.lock = threading.Lock()
        self.final = None
        for t in self.all_tasks():  # a validation the last monitor died in is redone
            if task_state(t["id"]).get("phase") == "validating":
                set_task_state(t["id"], phase="working", status="done")

    def all_tasks(self):
        return self.plan["tasks"] + [self.plan["gate"]]

    def validate(self, task, wt):
        def run():
            ok, problems, log = False, [], None
            try:
                dirty = sh(["git", "status", "--porcelain"], cwd=wt).stdout.split("\n")
                dirty = [d for d in dirty if d.strip()]
                if dirty:
                    problems = ["the worktree is not clean; commit what belongs to the fix and "
                                "remove the rest, then report done: " + "; ".join(dirty[:10])]
                elif task["kind"] != "gate" and not task.get("existing") and sh(
                        ["git", "rev-list", "--count", f"{task['base']}..HEAD"], cwd=wt).stdout.strip() == "0":
                    problems = ["no commit on the branch"]
                else:
                    set_task_state(task["id"], phase="validating")
                    ok, problems, log = check_task(task, wt, repeat=task.get("passes", 1))
            except Exception as e:  # keep the monitor alive; show the error as the failure
                problems = [f"validation error: {e}"]
            head = sh(["git", "rev-parse", "--short", "HEAD"], cwd=wt).stdout.strip()
            if ok:
                set_task_state(task["id"], phase="validated", validated_head=head,
                               last_check=f"PASS x{task.get('passes', 1)} @ {head}")
                event(f"{task['id']}: validated at {head}")
                if task.get("existing") and self.plan.get("push"):
                    r = sh(["git", "push", "origin", task["branch"]], cwd=wt)
                    event(f"{task['id']}: push {'ok' if r.returncode == 0 else 'FAILED: ' + r.stderr.strip()[:200]}")
                if task["id"] != "gate":
                    tmux("kill-window", "-t", f"{SESSION}:{task['id']}")
            else:
                fb = task_dir(task["id"]) / "validation-failed.md"
                fb.write_text(f"# Validation of {task['id']} failed at {head} ({now()})\n\n"
                              + "\n".join(f"- {p}" for p in problems)
                              + (f"\n\nFull output: {log}\n" if log else "\n"))
                set_task_state(task["id"], phase="working", status="working",
                               last_check="FAIL: " + (problems[0] if problems else "?")[:120])
                event(f"{task['id']}: validation failed: {problems[:3]}")
                if task["id"] in windows():
                    tell(task["id"], f"The ci-fix monitor's validation failed. Read {fb}, "
                         "fix it, commit, and report done again.")
            with self.lock:
                self.busy.pop(task["id"], None)
        th = threading.Thread(target=run, daemon=True)
        with self.lock:
            self.busy[task["id"]] = th
        th.start()

    def step_task(self, task):
        tid = task["id"]
        st = task_state(tid)
        phase = st.get("phase", "pending")
        if phase == "validated" or tid in self.busy:
            return
        live = windows()
        working = sum(1 for t in self.plan["tasks"] if task_state(t["id"]).get("phase") in ("working", "validating"))
        if phase == "pending":
            if working < self.plan.get("max_workers", 4):
                spawn(self.plan, task)
            return
        wt = worktree_for(self.plan, task)
        if st.get("status") == "done" and st.get("phase") == "working":
            set_task_state(tid, status="validating")
            self.validate(task, wt)
            return
        if tid not in live and st.get("status") not in ("blocked",):
            if st.get("respawns", 0) < 3:
                spawn(self.plan, task, resume=True)
            elif st.get("phase") != "dead":
                set_task_state(tid, phase="dead")
                event(f"{tid}: worker exited 3 times; left for the operator")
            return
        quiet = time.time() - _ts(st.get("updated"))
        if quiet > self.NUDGE_AFTER and time.time() - _ts(st.get("nudged")) > self.NUDGE_AFTER:
            set_task_state(tid, nudged=now())
            tell(tid, f"ci-fix monitor: no status from you in {quiet / 60:.0f} minutes. "
                 f"Report it now with `{SCRIPT} report {tid} --state working|done|blocked --note ...`.")

    def integrate(self):
        """Merge every validated task branch into the fix branch's worktree."""
        gate = self.plan["gate"]
        wt = ensure_worktree(self.plan, gate)
        for t in self.plan["tasks"]:
            if t.get("existing"):
                continue
            if sh(["git", "merge-base", "--is-ancestor", t["branch"], "HEAD"], cwd=wt).returncode == 0:
                continue
            r = sh(["git", "merge", "--no-ff", "--no-edit", "-m",
                    f"#{self.plan['issue']} Merge {t['branch']} into {gate['branch']}", t["branch"]], cwd=wt)
            if r.returncode != 0:
                files = sh(["git", "diff", "--name-only", "--diff-filter=U"], cwd=wt).stdout.split()
                sh(["git", "merge", "--abort"], cwd=wt)
                return f"merging {t['branch']} conflicts in: {' '.join(files)}"
            event(f"gate: merged {t['branch']}")
        return None

    def step_gate(self):
        g = self.plan["gate"]
        st = task_state("gate")
        phase = st.get("phase", "pending")
        if "gate" in self.busy:
            return
        if phase == "pending":
            conflict = self.integrate()
            if conflict:
                spawn(self.plan, g, extra=f"## Why you were started\n\n{conflict}. Merge the "
                      "remaining task branches by hand, resolving each conflict so both fixes hold.")
                return
            set_task_state("gate", phase="working", status="done", spawned=now())
        if phase == "working" and st.get("status") == "done":
            set_task_state("gate", status="validating")
            self.validate(g, worktree_for(self.plan, g))
            return
        if phase == "validated":
            self.step_ci(g, st)
            return
        if phase == "working" and st.get("status") == "working" and "gate" not in windows() \
                and st.get("last_check", "").startswith("FAIL"):
            spawn(self.plan, g, extra="## Why you were started\n\nThe local gate failed. "
                  f"Read {task_dir('gate') / 'validation-failed.md'}.")

    def step_ci(self, g, st):
        wt = worktree_for(self.plan, g)
        head = sh(["git", "rev-parse", "HEAD"], cwd=wt).stdout.strip()
        if not self.plan.get("push"):
            self.final = f"gate passed locally at {head[:9]}; push disabled"
            return
        if st.get("pushed") != head:
            r = sh(["git", "push", "-u", "origin", g["branch"]], cwd=wt)
            if r.returncode != 0:
                set_task_state("gate", last_check=f"push failed: {r.stderr.strip()[:120]}")
                return
            set_task_state("gate", pushed=head, ci="pending")
            event(f"gate: pushed {head[:9]}")
            return
        p = latest_pipeline(sha=head)
        if not p or p["status"] in ("created", "pending", "running", "waiting_for_resource", "preparing"):
            set_task_state("gate", ci=f"pipeline {p['id'] if p else '?'} {p['status'] if p else 'not yet'}")
            return
        bad = failed_jobs(p["id"])
        if not bad and p["status"] in ("success", "failed"):  # failed only via allow_failure jobs
            set_task_state("gate", ci=f"pipeline {p['id']} green")
            self.final = f"CI green: pipeline {p['id']} at {head[:9]} on {g['branch']}"
            return
        names = ", ".join(j["name"] for j in bad) or p["status"]
        logs = "\n".join(f"- {j['name']}: {job_log(j['id'])}" for j in bad)
        lint, tests, hung = ci_failures(p["id"])
        detail = "\n".join([f"- lint: `{l}`" for l in lint] + [f"- test: `{p_}.{t}`" for p_, t in tests]
                           + [f"- hung: `{t}`" for t in hung])
        set_task_state("gate", phase="working", status="working", ci=f"pipeline {p['id']} red: {names}",
                       last_check=f"FAIL: CI {names}")
        event(f"gate: CI pipeline {p['id']} red: {names}")
        spawn(self.plan, g, extra=f"## Why you were started\n\nCI pipeline {p['id']} failed on "
              f"{head[:9]}: {names}.\n\n{detail}\n\nJob logs:\n{logs}")

    def finish(self):
        msg = self.final
        event(f"done: {msg}")
        summary = STATE / f"summary-{now()}.txt"
        summary.write_text(render_status() + f"\n{msg}\n")
        sh(["notify-send", "ci-fix", msg])
        for w in windows() - {"monitor"}:
            tmux("kill-window", "-t", f"{SESSION}:{w}")
        print(f"\n{msg}\nSummary: {summary}\nClosing the session in 60s.")
        time.sleep(60)
        tmux("kill-session", "-t", SESSION)

    def loop(self):
        event("monitor started")
        while True:
            self.plan = load_plan()
            for t in self.plan["tasks"]:
                self.step_task(t)
            fix_tasks = [t for t in self.plan["tasks"] if not t.get("existing")]
            if all(task_state(t["id"]).get("phase") == "validated" for t in fix_tasks):
                self.step_gate()
            if self.final and all(task_state(t["id"]).get("phase") == "validated" for t in self.plan["tasks"]):
                self.finish()
                return
            sys.stdout.write("\033[H\033[2J" + render_status(self.busy))
            sys.stdout.flush()
            time.sleep(10)


def _ts(s):
    if not s:
        return 0
    return dt.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc).timestamp()


def _age(s):
    if not s:
        return "-"
    m = int((time.time() - _ts(s)) / 60)
    return f"{m}m" if m < 120 else f"{m // 60}h{m % 60:02d}"


def render_status(busy=()):
    plan = load_plan()
    live = windows()
    rows = [("task", "branch", "phase", "worker", "status", "upd", "last check / note")]
    for t in plan["tasks"] + [plan["gate"]]:
        st = task_state(t["id"])
        phase = st.get("phase", "pending") + ("*" if t["id"] in busy else "")
        note = st.get("last_check", "")
        if st.get("note"):
            note = (note + " | " if note else "") + st["note"]
        if t["id"] == "gate" and st.get("ci"):
            note = st["ci"] + " | " + note
        rows.append((t["id"], t["branch"], phase, "up" if t["id"] in live else "-",
                     st.get("status", "-"), _age(st.get("updated")), note[:110]))
    w = [max(len(str(r[i])) for r in rows) for i in range(6)]
    out = [f"ci-fix #{plan['issue']}  base {plan['base']}  fix {plan['fix_branch']}  "
           f"{now()}   (* = validating; windows: ctrl-b w)", ""]
    for r in rows:
        out.append("  ".join(str(r[i]).ljust(w[i]) for i in range(6)) + "  " + r[6])
    out.append("")
    try:
        out += ["recent events:"] + ["  " + l for l in EVENTS.read_text().splitlines()[-12:]]
    except FileNotFoundError:
        pass
    return "\n".join(out) + "\n"


# ---------------------------------------------------------------- commands

def find_task(tid):
    plan = load_plan()
    for t in plan["tasks"] + [plan["gate"]]:
        if t["id"] == tid:
            return t
    raise SystemExit(f"no task {tid}; tasks: {[t['id'] for t in plan['tasks']]} + gate")


def cmd_check(a):
    task = find_task(a.task)
    ok, problems, log = check_task(task, os.getcwd(), repeat=a.repeat)
    print("PASS" if ok else "FAIL")
    for p in problems:
        print(" ", p)
    print(f"log: {log}")
    sys.exit(0 if ok else 1)


def cmd_report(a):
    find_task(a.task)
    set_task_state(a.task, status=a.state, note=a.note)
    event(f"{a.task}: reported {a.state}: {a.note}")
    print(f"recorded: {a.task} {a.state}")


def cmd_slot(a):
    cmd = a.cmd[1:] if a.cmd and a.cmd[0] == "--" else a.cmd
    with Slot(load_plan().get("slots", 3)):
        sys.exit(subprocess.call(cmd))


def cmd_run(a):
    load_plan()
    if SESSION in sh(["tmux", "ls", "-F", "#{session_name}"]).stdout.split():
        raise SystemExit(f"tmux session {SESSION} already exists; attach with `tmux attach -t {SESSION}`")
    tmux("new-session", "-d", "-s", SESSION, "-n", "monitor", "-c", str(ROOT),
         f"{shlex.quote(sys.executable)} {shlex.quote(str(SCRIPT))} monitor; "
         "echo monitor exited; read")
    print(f"started tmux session {SESSION}")
    if not a.no_open and os.environ.get("DISPLAY") and shutil.which("gnome-terminal"):
        subprocess.Popen(["gnome-terminal", "--title", "ci-fix", "--", "tmux", "attach", "-t", SESSION],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        print("opened a terminal attached to it")
    else:
        print(f"attach with: tmux attach -t {SESSION}")


def cmd_stop(a):
    plan = load_plan()
    tmux("kill-session", "-t", SESSION)
    event("stopped by operator")
    if not a.keep_worktrees:
        for t in plan["tasks"] + [plan["gate"]]:
            wt = worktree_for(plan, t)
            if wt.exists():
                r = sh(["git", "worktree", "remove", str(wt)], cwd=ROOT)
                print(f"{wt}: {'removed' if r.returncode == 0 else r.stderr.strip()}")
    print("stopped")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("plan", help="write plan.json from CI evidence")
    p.add_argument("--issue", type=int, required=True)
    p.add_argument("--base", default="origin/dagbft-integration")
    p.add_argument("--pipeline", type=int, help="pipeline to read failures from (default: latest on base)")
    p.add_argument("--pipeline-extra", type=int, action="append", help="more pipelines to union in")
    p.add_argument("--merge-group", action="append", help="task ids to merge into the first: a,b,c")
    p.add_argument("--mr-branch", action="append", help="MR branch to give a lint task for its own files")
    p.add_argument("--passes", type=int, default=3, help="consecutive passes a test task must hold")
    p.add_argument("--slots", type=int, default=3, help="heavy commands run at once, machine-wide")
    p.add_argument("--max-workers", type=int, default=6)
    p.add_argument("--model", default="opus")
    p.add_argument("--lint-model", default="sonnet")
    p.add_argument("--effort", default="high")
    p.add_argument("--worktrees", default=str(ROOT.parent / f"{ROOT.name}-ci-fix"))
    p.add_argument("--no-push", action="store_true")
    p.add_argument("--force", action="store_true")
    p.set_defaults(fn=cmd_plan)

    p = sub.add_parser("run", help="start the tmux session and open it")
    p.add_argument("--no-open", action="store_true")
    p.set_defaults(fn=cmd_run)

    sub.add_parser("monitor", help="the monitor loop (run inside tmux)").set_defaults(
        fn=lambda a: Monitor().loop())
    sub.add_parser("status", help="print the table once").set_defaults(
        fn=lambda a: print(render_status(), end=""))

    p = sub.add_parser("check", help="run a task's check in the current directory")
    p.add_argument("task")
    p.add_argument("--repeat", type=int, default=1)
    p.set_defaults(fn=cmd_check)

    p = sub.add_parser("report", help="worker: record your status")
    p.add_argument("task")
    p.add_argument("--state", choices=["working", "done", "blocked"], required=True)
    p.add_argument("--note", required=True)
    p.set_defaults(fn=cmd_report)

    p = sub.add_parser("slot", help="run a heavy command in a shared slot")
    p.add_argument("cmd", nargs=argparse.REMAINDER)
    p.set_defaults(fn=cmd_slot)

    p = sub.add_parser("stop", help="close every session and remove the worktrees")
    p.add_argument("--keep-worktrees", action="store_true")
    p.set_defaults(fn=cmd_stop)

    a = ap.parse_args()
    a.fn(a)


if __name__ == "__main__":
    main()
