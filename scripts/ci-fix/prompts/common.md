# ci-fix task `{task}` (issue #{issue})

You are one of several Claude Code sessions started by `{script}`, each in
its own tmux window and git worktree. A monitor in tmux window `monitor`
watches all of them. It decides when your task is finished. You do not.

- Worktree: `{worktree}` (stay in it)
- Branch: `{branch}`, from `{base}`
- Other tasks being worked in parallel: {others}
- CI evidence: {evidence}. Job logs, with timestamps stripped, are in `{ci_logs}`.
- Your task's state and the monitor's messages to you: `{state}`

## How you are judged

The monitor runs exactly this, from your worktree:

    {script} check {task}

For a test task it runs it {passes} times in a row, each a fresh process,
and every run must pass. Run it yourself before you report done.
`--repeat N` repeats it.

Report your status. Do it when you start, when you learn something the
operator should see (keep the note to one line), and when you finish:

    {script} report {task} --state working --note "reproduced locally: ..."
    {script} report {task} --state done    --note "<root cause> -> <fix>; check passes xN"
    {script} report {task} --state blocked --note "<the decision Paul must make>"

Report `done` only after your work is committed and the tree is clean. The
monitor rejects a dirty tree. If it then tells you validation failed, read
the file it names, fix the cause, and report done again.

## Rules (from the project owner, Paul — not negotiable)

1. **Root cause, proven.** Reproduce the failure locally before changing
   anything. Do not build a conclusion on one observation. Run the code.
   In your done note and commit message, say what you verified and what
   you did NOT verify.
2. **Never weaken a test to make it pass.** Do not delete or loosen an
   assertion, add `t.Skip`, or retry until it passes. Do not raise a
   timeout, height, round count or run length unless you show, with
   measurements, that the budget is wrong and the code is right. If you do
   raise one, the commit says why the old budget could not hold. A test
   that passes while the mechanism it names cannot work is worse than a
   red one.
3. **A production fix comes with a test that fails without it.** Often that
   is the failing test itself. Say which.
4. **Work against the spec.** Read the relevant part of `docs/spec/` first.
   If you change behaviour, the spec changes in the same commit. Where the
   code and the spec differ and you are not fixing that difference, record
   it in `docs/spec/DIFFERENCES.md`. Do not silently fix it.
5. **Commit as you go.** Messages follow the repo's style:
   `#{issue} <what is true now, as a sentence>`, with a body giving the root
   cause and the evidence. Add files by name. Never `git add -A` or
   `git add .`: soak runs and profiles are lying around. Do not push. Do not
   create MRs or issues, and do not comment on GitLab (`glab` for reading is
   fine). Do not edit other repositories.
6. **Share the machine.** Run every Go test of `pkg/consensus/...` or
   `test/...` through a slot: either `{script} check {task}` or
   `{script} slot -- go test ...`. The slots keep parallel sessions from
   loading the box into false timeouts. Do not run soaks, Docker networks or
   devnets. Stop any background process you start.
7. **Stay in your lane.** Do not fix another task's tests. If your root
   cause turns out to be shared with another task, say so in a `working`
   note naming that task, so the operator sees it. Then fix it on your
   branch only if it is needed for your own tests.
8. **Blocked means blocked.** If the right fix is a design decision (a spec
   change, a trade-off, behaviour that is arguably correct), report
   `blocked` with the question. Do not guess.
