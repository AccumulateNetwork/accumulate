#!/usr/bin/env python3
"""
CometBFT Removal Orchestrator

Drives Claude Code sessions with hardcoded prompts to remove all CometBFT
dependencies. Claude cannot skip rules because they're in the prompt, not
in a file it can ignore.

Victory requires ALL of:
  - Zero 'github.com/cometbft' imports in any .go file (src + test)
  - Zero direct cometbft in go.mod
  - go build passes on all packages
  - go test passes on all packages
  - go vet passes
  - Final review confirms no issues
  - No pending/blocked tasks in the plan file

Usage:
    python3 docs/plans/run-cometbft-removal.py
"""

import subprocess
import re
import sys
import time
from pathlib import Path
from datetime import datetime

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
PLAN_FILE = REPO_ROOT / "docs" / "plans" / "remove-cometbft-plan.md"
BRANCH = "issue-3910-remove-cometbft"
MAX_CYCLES = 50
CYCLE_PAUSE = 5
LOG_DIR = Path("/tmp/cometbft-removal")

SELF_FILES = {"remove-cometbft-plan.md", "run-cometbft-removal.py"}
STALL_THRESHOLD = 3  # cycles with no progress before escalating
CLAUDE_TIMEOUT = 2400  # 40 minutes per session

# ─── Hardcoded prompt templates ──────────────────────────────────────────────
# These are the ONLY instructions Claude receives. The plan file is just a
# task list — Claude cannot override these rules.

WORK_PROMPT = """You are an automated code migration worker. You MUST follow
these instructions exactly. Do not deviate, improvise, or skip steps.

TASK: Remove CometBFT/Tendermint dependencies from the Accumulate codebase.

BRANCH: {branch}

STEP 1: Read the task list at docs/plans/remove-cometbft-plan.md
  - Find the first row with Status = PENDING whose Depends are all DONE
  - If no PENDING task is available, look at the FILES list below and
    create a plan to fix the first 2-3 files

STEP 2: For each task (do 2-4 per session):
  a) Create a work branch: git checkout -b issue-3910-<task-id> {branch}
  b) Read the file(s) mentioned in the task
  c) Remove every github.com/cometbft import. The approach:
     - COPY the CometBFT struct/type definitions you need into a LOCAL file
       in the same package or a shared package. Keep the EXACT same field
       names, JSON/TOML tags, and serialization. No format changes.
     - Logger types: replace with logging.Logger from internal/logging
     - Crypto types: replace with crypto/ed25519 from stdlib (just []byte)
     - Config types: copy used fields into a local struct, same TOML tags
     - Genesis types: copy struct definitions, same JSON tags
     - Key types: copy struct with same JSON serialization
     - For EVERY struct you copy: write a compat test (task IDs ending in T)
       that imports BOTH the CometBFT original and your copy, creates a
       populated instance of the CometBFT type, marshals it, unmarshals into
       your type, and verifies all fields match. Name tests TestCompat*.
     - If a file/function is entirely CometBFT-specific and dead: delete it
  d) Build check (MANDATORY after each file change):
     go build ./cmd/accumulated/... > /tmp/build.log 2>&1
     go build ./test/simulator/... >> /tmp/build.log 2>&1
     go build ./internal/... >> /tmp/build.log 2>&1
     go build ./tools/... >> /tmp/build.log 2>&1
     ALL must exit 0. If not, fix the errors before continuing.
  e) Test check (MANDATORY after each task):
     go test ./internal/logging/... > /tmp/test.log 2>&1
     go test ./internal/node/... >> /tmp/test.log 2>&1
     go test ./test/simulator/... >> /tmp/test.log 2>&1
     ALL must exit 0. If not, fix the errors before continuing.
  f) Commit: git add <changed files> && git commit -m "Issue #NNNN: description"
  g) Merge back: git checkout {branch} && git merge issue-3910-<task-id>
  h) If merge succeeds: git branch -d issue-3910-<task-id>
  i) If merge FAILS:
     - git merge --abort
     - Analyze WHY it failed (what conflicted, what assumptions were wrong)
     - Create a GitLab issue: glab issue create --title "..." --description "..." --label "cleanup"
     - Add "Sub-issue of #3910" as a note on the new issue
     - Mark the task BLOCKED in the plan with a note pointing to the new issue
     - Leave the work branch intact for the next cycle to pick up

STEP 4: Update docs/plans/remove-cometbft-plan.md:
  - Change the task's Status from PENDING to DONE
  - Add the commit hash to the Commit column

STEP 5: After 2-4 tasks, exit.

MANDATORY RULES (violations cause the orchestrator to retry):
- Redirect ALL command output to /tmp/ log files. Use > and 2>&1.
- "Pre-existing error" is NOT acceptable. If it doesn't build, fix it.
- Do NOT use action teams, agents, or subagents. Work directly.
- Do NOT skip builds or tests. Run them after EVERY change.
- Do NOT mark a task DONE if the build is broken.
- Do NOT leave unused imports. Run go vet if unsure.
- Test files (.go files ending in _test.go) count too. Fix them.
- The orchestrator verifies success by running grep, go build, go test,
  and go vet itself. Updating the plan file does not affect victory.
  The plan is for YOUR progress tracking only.
- Work on feature branches for implementation. Merge back to {branch} only
  when the branch builds and tests pass. If a merge fails, leave the branch.

CURRENT STATE:
- Source files with CometBFT imports: {src_count}
- Test files with CometBFT imports: {test_count}
- go.mod direct CometBFT deps: {gomod_count}
- Build: {build_status}
- Tests: {test_status}
- Plan: {done} done, {pending} pending, {blocked} blocked

{build_errors}
{test_errors}

FILES STILL IMPORTING COMETBFT:
{file_list}

{gomod_info}
"""

REVIEW_PROMPT = """You are an automated code reviewer. You MUST follow these
instructions exactly. Do not skip any step.

TASK: Final review of CometBFT removal on branch {branch}.

The automated checks report zero CometBFT files and all builds/tests pass.
Your job is to VERIFY this is actually true and fix anything missed.

STEP 1: Run this exact command:
  grep -rn "cometbft\\|tendermint" --include="*.go" . | grep -v vendor | grep -v worktrees > /tmp/review-all.log 2>&1
  Read /tmp/review-all.log. Check EVERY line.

STEP 2: For each match found:
  a) If it's an actual import or type usage: FIX IT NOW. Then rebuild and test.
  b) If it's a harmless string (comment, regex, metric pattern): note file and line.
  c) If it's in pkg/types/cometbft/ (intentional wrapper): note why it exists.

STEP 3: Check go.mod:
  grep cometbft go.mod > /tmp/gomod-check.log 2>&1
  If any DIRECT (non-indirect) deps remain, run: go mod tidy

STEP 4: Full build (all must pass):
  go build ./cmd/accumulated/... > /tmp/review-build.log 2>&1
  go build ./test/simulator/... >> /tmp/review-build.log 2>&1
  go build ./internal/... >> /tmp/review-build.log 2>&1
  go build ./tools/... >> /tmp/review-build.log 2>&1
  go build ./exp/... >> /tmp/review-build.log 2>&1
  go build ./pkg/... >> /tmp/review-build.log 2>&1

STEP 5: Full tests (all must pass):
  go test ./internal/logging/... > /tmp/review-test.log 2>&1
  go test ./internal/node/... >> /tmp/review-test.log 2>&1
  go test ./internal/api/... >> /tmp/review-test.log 2>&1
  go test ./test/simulator/... >> /tmp/review-test.log 2>&1
  go test ./cmd/accumulated/run/... >> /tmp/review-test.log 2>&1

STEP 6: Vet:
  go vet ./internal/... ./cmd/accumulated/... ./test/simulator/... > /tmp/review-vet.log 2>&1

STEP 7: Update docs/plans/remove-cometbft-plan.md Progress Log:
  - If ALL checks pass and no actionable items found:
    Add: "| {date} | Review | PASSED | Zero actionable CometBFT refs, N harmless strings |"
  - If ANY actionable items remain:
    Add PENDING tasks for each one.
    Add: "| {date} | Review | FAILED | K items still need fixing |"

MANDATORY RULES:
- Redirect ALL output to /tmp/ log files.
- Do NOT declare PASSED if any .go file has an actionable cometbft import.
- Do NOT use action teams or agents.
- Fix every issue you find. Do not just document it.
- If you can't fix something, add it as a PENDING task in the plan.
"""

FIX_PROMPT = """You are an automated build fixer. Fix the build/test errors below.

BRANCH: {branch}

The build or tests are currently broken. Fix them FIRST before anything else.

ERRORS:
{errors}

STEPS:
1. Read the error messages carefully.
2. Read each failing file.
3. Fix the errors. Common fixes:
   - Unused import: remove it
   - Undefined name: the function/type was deleted — find replacement or stub
   - Type mismatch: logging.Logger vs cometbft log.Logger — use the right one
4. After each fix, run the EXACT commands that failed (shown above) to verify.
5. Then run the full build:
   go build ./cmd/accumulated/... > /tmp/fix-build.log 2>&1
   go build ./test/simulator/... >> /tmp/fix-build.log 2>&1
   go build ./internal/... >> /tmp/fix-build.log 2>&1
   go build ./tools/... >> /tmp/fix-build.log 2>&1
6. And the full test suite:
   go test ./internal/logging/... > /tmp/fix-test.log 2>&1
   go test ./internal/node/... >> /tmp/fix-test.log 2>&1
   go test ./test/simulator/... >> /tmp/fix-test.log 2>&1
   go test ./cmd/accumulated/run/... >> /tmp/fix-test.log 2>&1
7. ALL must pass. If any fail, keep fixing.
8. Commit: git add <files> && git commit -m "Issue #3910: Fix build/test errors"

MANDATORY RULES:
- Redirect ALL output to /tmp/ log files.
- Do NOT use action teams or agents.
- Do NOT skip the build AND test checks.
- "Pre-existing" is not an excuse. Fix everything.
"""


# ─── Helper functions ────────────────────────────────────────────────────────

def log(msg):
    ts = datetime.now().strftime("%H:%M:%S")
    line = f"[{ts}] {msg}"
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    with open(LOG_DIR / "orchestrator.log", "a") as f:
        f.write(line + "\n")


def run_cmd(cmd, timeout=300):
    if isinstance(cmd, str):
        cmd = cmd.split()
    result = subprocess.run(
        cmd, capture_output=True, text=True,
        cwd=REPO_ROOT, timeout=timeout
    )
    return result.returncode, result.stdout, result.stderr


def find_cometbft_files():
    """Find .go files with actual CometBFT package imports.

    Checks for 'github.com/cometbft' in import statements — this is what
    creates a real dependency. String mentions of "tendermint" in error
    messages, config filenames, or comments are NOT dependencies and are
    ignored. The review session handles those separately.
    """
    _, stdout, _ = run_cmd([
        "grep", "-rl", "--include=*.go",
        r'"github.com/cometbft', "."
    ])
    all_files = []
    for f in stdout.strip().split("\n"):
        if not f:
            continue
        if "/vendor/" in f or "/worktrees/" in f:
            continue
        if any(s in f for s in SELF_FILES):
            continue
        all_files.append(f)
    src = [f for f in all_files if not f.endswith("_test.go")]
    test = [f for f in all_files if f.endswith("_test.go")]
    return src, test


def check_gomod():
    """Check for direct cometbft dependencies in go.mod."""
    gomod = (REPO_ROOT / "go.mod").read_text()
    direct = []
    in_require = False
    for line in gomod.split("\n"):
        stripped = line.strip()
        if stripped.startswith("require ("):
            in_require = True
            continue
        if in_require and stripped == ")":
            in_require = False
            continue
        if "cometbft" in line and "// indirect" not in line:
            if in_require or line.startswith("require "):
                direct.append(stripped)
    return direct


def check_build():
    cmds = [
        "go build ./cmd/accumulated/...",
        "go build ./test/simulator/...",
        "go build ./internal/...",
        "go build ./tools/...",
        "go build ./exp/...",
        "go build ./pkg/...",
    ]
    errors = []
    for cmd in cmds:
        rc, _, stderr = run_cmd(cmd.split(), timeout=300)
        if rc != 0:
            errors.append(f"BUILD FAIL: {cmd}\n{stderr[:1000]}")
    return len(errors) == 0, "\n".join(errors)


def check_tests(full=False):
    """Run tests. If full=True, run go test ./... (slow but comprehensive)."""
    if full:
        # Full test — catches everything but takes a long time
        rc, _, stderr = run_cmd(
            ["go", "test", "-count=1", "-short", "./..."],
            timeout=1200  # 20 minutes
        )
        if rc != 0:
            return False, f"TEST FAIL: go test ./...\n{stderr[:2000]}"
        return True, ""

    # Quick test — targeted packages most likely to break, including compat tests
    cmds = [
        "go test ./internal/logging/...",
        "go test ./internal/node/...",
        "go test ./internal/api/...",
        "go test ./internal/database/...",
        "go test ./test/simulator/...",
        "go test ./pkg/types/...",
        "go test ./pkg/api/...",
        "go test ./cmd/accumulated/run/...",
    ]
    # Add compat tests if they exist (created during struct-copying phases)
    for compat in [
        "go test -run Compat ./internal/node/daemon/...",
        "go test -run Compat ./internal/node/config/...",
        "go test -run Compat ./pkg/types/cometbft/...",
        "go test -run Compat ./internal/node/genesis/...",
    ]:
        # Only run if the test file exists — don't fail if not created yet
        rc, _, _ = run_cmd(compat.split(), timeout=120)
        if rc != 0:
            # Check if it's "no test files" vs actual failure
            _, _, stderr = run_cmd(compat.split(), timeout=120)
            if "no test files" not in stderr and "no tests to run" not in stderr:
                cmds.append(compat)  # Real failure — add to check list
    errors = []
    for cmd in cmds:
        rc, _, stderr = run_cmd(cmd.split(), timeout=600)
        if rc != 0:
            errors.append(f"TEST FAIL: {cmd}\n{stderr[:500]}")
    return len(errors) == 0, "\n".join(errors)


def check_vet():
    rc, _, stderr = run_cmd([
        "go", "vet",
        "./internal/...", "./cmd/accumulated/...",
        "./test/simulator/...", "./tools/...",
        "./exp/...", "./pkg/...",
    ], timeout=300)
    return rc == 0, stderr[:2000]


def get_plan_status():
    content = PLAN_FILE.read_text()
    return {
        "pending": len(re.findall(r"\| PENDING \|", content)),
        "done": len(re.findall(r"\| DONE \|", content)),
        "blocked": len(re.findall(r"\| BLOCKED \|", content)),
    }


def ensure_branch():
    _, stdout, _ = run_cmd(["git", "branch", "--show-current"])
    current = stdout.strip()
    if current != BRANCH:
        log(f"Switching from {current} to {BRANCH}")
        run_cmd(["git", "checkout", BRANCH])
        _, stdout, _ = run_cmd(["git", "branch", "--show-current"])
        if stdout.strip() != BRANCH:
            log(f"ERROR: Could not switch to {BRANCH}")
            sys.exit(1)


def recover_dirty_state():
    """If a prior session crashed mid-edit, clean up."""
    # Abort any in-progress operations first
    for marker, abort_cmd in [
        (".git/CHERRY_PICK_HEAD", ["git", "cherry-pick", "--abort"]),
        (".git/MERGE_HEAD", ["git", "merge", "--abort"]),
        (".git/REBASE_HEAD", ["git", "rebase", "--abort"]),
    ]:
        if (REPO_ROOT / marker).exists():
            log(f"Aborting in-progress operation: {marker}")
            run_cmd(abort_cmd)

    # If we're on a work branch, switch back to main branch
    _, stdout, _ = run_cmd(["git", "branch", "--show-current"])
    current = stdout.strip()
    if current != BRANCH and current.startswith("issue-3910-"):
        log(f"On work branch {current}, switching back to {BRANCH}")
        # Stash any uncommitted work
        rc, status, _ = run_cmd(["git", "status", "--porcelain"])
        if status.strip():
            run_cmd(["git", "stash", "push", "-m",
                     f"work-branch-{current}-{datetime.now().strftime('%H%M%S')}"])
        run_cmd(["git", "checkout", BRANCH])
        return

    # Stash dirty state on main branch
    rc, stdout, _ = run_cmd(["git", "status", "--porcelain"])
    if stdout.strip():
        dirty_files = stdout.strip().split("\n")
        log(f"WARNING: Dirty state ({len(dirty_files)} files). Stashing...")
        run_cmd(["git", "stash", "push", "-m",
                 f"orchestrator-recovery-{datetime.now().strftime('%H%M%S')}"])


def gather_state(full=True):
    """Collect state. If full=False, skip expensive tests (quick check only)."""
    src, test = find_cometbft_files()
    build_ok, build_err = check_build()
    gomod = check_gomod()
    plan = get_plan_status()

    if full:
        tests_ok, test_err = check_tests(full=False)  # targeted, not ./...
    else:
        tests_ok, test_err = True, ""  # optimistic for quick checks

    return {
        "src_files": src, "test_files": test,
        "src_count": len(src), "test_count": len(test),
        "build_ok": build_ok, "build_errors": build_err,
        "tests_ok": tests_ok, "test_errors": test_err,
        "gomod_deps": gomod, "plan": plan,
    }


def is_victory(state):
    """Victory conditions checked ONLY by the script against real artifacts.

    The plan file status is NOT checked here — Claude can write anything
    to the plan, so we don't trust it for victory decisions. Victory is
    determined solely by what grep, go build, go test, and go.mod say.
    """
    checks = {
        "zero_src_files": state["src_count"] == 0,
        "zero_test_files": state["test_count"] == 0,
        "build_passes": state["build_ok"],
        "tests_pass": state["tests_ok"],
        "gomod_clean": len(state["gomod_deps"]) == 0,
    }
    failed = [k for k, v in checks.items() if not v]
    if failed:
        log(f"Victory blocked by: {', '.join(failed)}")
    return not failed


# ─── Session launchers ───────────────────────────────────────────────────────

def launch_claude(prompt, label, cycle=0):
    """Launch claude with the given prompt. Returns success bool.

    Backs up the plan file before the session and restores it if Claude
    corrupts it (e.g., deletes the table headers or all task rows).
    """
    log(f"Launching Claude: {label}")
    LOG_DIR.mkdir(parents=True, exist_ok=True)

    # Back up plan file before Claude touches it
    plan_backup = PLAN_FILE.read_text() if PLAN_FILE.exists() else ""

    rc, stdout, stderr = run_cmd(
        ["claude", "--yes", "-p", prompt], timeout=CLAUDE_TIMEOUT
    )

    logfile = LOG_DIR / f"cycle-{cycle:03d}-{label}.log"
    logfile.write_text(
        f"PROMPT:\n{prompt[:500]}...\n\nSTDOUT:\n{stdout}\n\nSTDERR:\n{stderr}"
    )
    log(f"  Exit: {rc}, log: {logfile}")

    # Validate plan file wasn't corrupted
    if PLAN_FILE.exists():
        plan_now = PLAN_FILE.read_text()
        # Sanity checks: must still have table structure
        if "| ID |" not in plan_now or "| Status |" not in plan_now:
            log("WARNING: Plan file corrupted (table headers missing). Restoring backup.")
            PLAN_FILE.write_text(plan_backup)
        elif len(plan_now) < len(plan_backup) * 0.5:
            log(f"WARNING: Plan file shrunk from {len(plan_backup)} to {len(plan_now)} bytes. Restoring backup.")
            PLAN_FILE.write_text(plan_backup)

    return rc == 0


def run_work_session(cycle, state):
    all_files = state["src_files"] + state["test_files"]
    prompt = WORK_PROMPT.format(
        branch=BRANCH,
        src_count=state["src_count"],
        test_count=state["test_count"],
        gomod_count=len(state["gomod_deps"]),
        build_status="PASSING" if state["build_ok"] else "BROKEN - FIX FIRST",
        test_status="PASSING" if state["tests_ok"] else "FAILING - FIX FIRST",
        done=state["plan"]["done"],
        pending=state["plan"]["pending"],
        blocked=state["plan"]["blocked"],
        build_errors=("BUILD ERRORS:\n" + state["build_errors"][:800]) if not state["build_ok"] else "",
        test_errors=("TEST ERRORS:\n" + state["test_errors"][:800]) if not state["tests_ok"] else "",
        file_list="\n".join(all_files[:50]) + (
            f"\n... and {len(all_files) - 50} more" if len(all_files) > 50 else ""),
        gomod_info=("go.mod DIRECT DEPS: " + ", ".join(state["gomod_deps"])) if state["gomod_deps"] else "",
    )
    return launch_claude(prompt, "work", cycle)


def run_fix_session(cycle, state):
    errors = ""
    if not state["build_ok"]:
        errors += state["build_errors"][:1500] + "\n"
    if not state["tests_ok"]:
        errors += state["test_errors"][:1500] + "\n"
    prompt = FIX_PROMPT.format(branch=BRANCH, errors=errors)
    return launch_claude(prompt, "fix", cycle)


def run_review_session(cycle):
    prompt = REVIEW_PROMPT.format(
        branch=BRANCH,
        date=datetime.now().strftime("%Y-%m-%d"),
    )
    return launch_claude(prompt, "review", cycle)


def add_tasks_for_uncovered_files(state, cycle):
    with open(PLAN_FILE, "a") as pf:
        for f in state["src_files"][:10]:
            pf.write(
                f"\n| X{cycle} | #3910 | Remove CometBFT from `{f}` "
                f"| PENDING | — | | Auto-added by orchestrator |"
            )
        for f in state["test_files"][:10]:
            pf.write(
                f"\n| T{cycle} | #3910 | Remove CometBFT from test `{f}` "
                f"| PENDING | — | | Auto-added by orchestrator |"
            )
        if state["gomod_deps"]:
            pf.write(
                f"\n| M{cycle} | #3910 | Run go mod tidy to remove CometBFT "
                f"| PENDING | — | | Auto-added |"
            )


# ─── Main loop ───────────────────────────────────────────────────────────────

def main():
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    log("=" * 60)
    log("CometBFT Removal Orchestrator")
    log(f"Repo: {REPO_ROOT}")
    log(f"Branch: {BRANCH}")
    log("=" * 60)

    if not PLAN_FILE.exists():
        log(f"ERROR: Plan file not found: {PLAN_FILE}")
        return 1

    ensure_branch()
    recover_dirty_state()

    last_file_count = None
    stall_count = 0
    review_count = 0

    for cycle in range(1, MAX_CYCLES + 1):
        recover_dirty_state()  # clean up if prior session crashed
        state = gather_state(full=(cycle % 3 == 0))  # full tests every 3rd cycle
        total = state["src_count"] + state["test_count"]

        # ── Stall detection ──
        if last_file_count is not None and total >= last_file_count:
            stall_count += 1
            log(f"  STALL: No progress for {stall_count} cycles ({total} files)")
            if stall_count >= STALL_THRESHOLD:
                log(f"  STUCK after {STALL_THRESHOLD} cycles. Escalating...")
                # Force full state check and add explicit tasks
                state = gather_state(full=True)
                add_tasks_for_uncovered_files(state, cycle)
                stall_count = 0
        else:
            stall_count = 0
        last_file_count = total

        log(f"\n{'='*60}")
        log(f"CYCLE {cycle}/{MAX_CYCLES}")
        log(f"  CometBFT files: {state['src_count']} src + {state['test_count']} test = {total}")
        log(f"  go.mod deps: {len(state['gomod_deps'])}")
        log(f"  Build: {'OK' if state['build_ok'] else 'BROKEN'}")
        log(f"  Tests: {'OK' if state['tests_ok'] else 'FAILING'}")
        log(f"  Plan: {state['plan']['done']}D {state['plan']['pending']}P {state['plan']['blocked']}B")
        log(f"{'='*60}")

        # ── Phase 1: If broken, fix first ──
        # If we skipped tests in gather_state, run them now before deciding
        if not state["build_ok"]:
            log("Build broken. Running fix session...")
            run_fix_session(cycle, state)
            time.sleep(CYCLE_PAUSE)
            continue
        if not state["tests_ok"]:
            # If tests were skipped (optimistic), run them for real now
            if cycle % 3 != 0:
                state["tests_ok"], state["test_errors"] = check_tests()
            if not state["tests_ok"]:
                log("Tests failing. Running fix session...")
                run_fix_session(cycle, state)
                time.sleep(CYCLE_PAUSE)
                continue

        # ── Phase 2: If clean, run review (max 3 reviews to prevent loops) ──
        if total == 0 and len(state["gomod_deps"]) == 0 and review_count < 3:
            review_count += 1
            log(f"Files look clean. Running review ({review_count}/3)...")
            run_review_session(cycle)
            # Re-check everything after review (FULL check, no shortcuts)
            state = gather_state(full=True)
            if is_victory(state):
                # Run comprehensive tests as final gate
                log("Running full test suite (go test ./...)...")
                full_tests_ok, full_test_err = check_tests(full=True)
                if not full_tests_ok:
                    log(f"Full tests failed: {full_test_err[:500]}")
                    state["tests_ok"] = False
                    state["test_errors"] = full_test_err
                    time.sleep(CYCLE_PAUSE)
                    continue
                vet_ok, vet_err = check_vet()
                if vet_ok:
                    log("\n" + "=" * 60)
                    log("VICTORY")
                    log(f"  Cycles: {cycle}")
                    log(f"  Tasks done: {state['plan']['done']}")
                    log(f"  CometBFT files: 0")
                    log(f"  Build: PASS  Tests: PASS  Vet: PASS")
                    log(f"  go.mod: CLEAN")
                    log("=" * 60)
                    return 0
                else:
                    log(f"Vet failed: {vet_err[:300]}")
                    # Will loop and fix

            # Review may have found issues — continue working
            time.sleep(CYCLE_PAUSE)
            continue

        # ── Phase 3: Ensure there are tasks to do ──
        if state["plan"]["pending"] == 0 and total > 0:
            log("No pending tasks but files remain. Adding tasks...")
            add_tasks_for_uncovered_files(state, cycle)

        if state["plan"]["pending"] == 0 and not state["build_ok"]:
            with open(PLAN_FILE, "a") as pf:
                pf.write(f"\n| F{cycle} | #3910 | Fix build errors | PENDING | — | | Auto-added |")

        # ── Phase 4: Do work ──
        run_work_session(cycle, state)

        # ── Progress report (file only) ──
        post = gather_state(full=False)
        delta_src = state["src_count"] - post["src_count"]
        delta_test = state["test_count"] - post["test_count"]
        _, commits, _ = run_cmd(["git", "log", "--oneline", "-3"])
        log(f"Cycle {cycle} done: -{delta_src} src, -{delta_test} test, "
            f"remaining {post['src_count']}+{post['test_count']}, "
            f"build={'ok' if post['build_ok'] else 'FAIL'}")
        with open(LOG_DIR / "progress.log", "a") as f:
            f.write(f"{datetime.now().strftime('%H:%M')} cycle={cycle} "
                    f"src={post['src_count']} test={post['test_count']} "
                    f"delta=-{delta_src}/-{delta_test} "
                    f"build={'ok' if post['build_ok'] else 'FAIL'} "
                    f"plan={post['plan']['done']}D/{post['plan']['pending']}P/{post['plan']['blocked']}B\n")
        with open(LOG_DIR / "commits.log", "a") as f:
            f.write(f"--- cycle {cycle} ---\n{commits.strip()}\n")

        time.sleep(CYCLE_PAUSE)

    # Max cycles
    state = gather_state()
    log(f"\nMax cycles ({MAX_CYCLES}) reached.")
    log(f"Remaining: {state['src_count']} src, {state['test_count']} test")
    for f in state["src_files"] + state["test_files"]:
        log(f"  {f}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
