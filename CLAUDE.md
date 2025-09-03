## Issue Tracking and Work Management

All tracking in: `~/go/src/gitlab.com/AccumulateNetwork/tracking_repo/`

### Workflow
1. **Start**: Update `../tracking_repo/repositories/accumulate/active.md`
2. **Work**: Log in `../tracking_repo/WORK_LOG.md` with commits
3. **Complete**: Create MR, update time in `../tracking_repo/TIME_TRACKING.md`
4. **Merge**: Only then move to `../tracking_repo/repositories/accumulate/completed.md` (after merge to main or other open branches)

### Branch Rules
- Create MR when work complete
- **NEVER** delete branches with active MRs
- Complete issues after merge to main or other open branches

Reference: `~/go/src/gitlab.com/AccumulateNetwork/tracking_repo/CLAUDE.md`
- the devnet is a test platform built by the AccumulateNetwork/Devnet repository on gitlab.com  We run commands like devnet start 3684-crosschain-healing --dpm 5