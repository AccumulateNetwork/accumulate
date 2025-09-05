# Accumulate Core - Development Notes

## MANDATORY: Review Tracking Repository Before Development

**FIRST ACTION before any development work:**
1. **Review**: `~/go/src/gitlab.com/AccumulateNetwork/tracking_repo/CLAUDE.md`
2. **Check**: Latest development rules, file naming standards, TDD requirements
3. **Update**: Any changes in development process or organization
4. **Verify**: Understanding of pre-merge simplification requirements

**This ensures all development follows current standards and organization.**

## Issue Tracking and Work Management

**All tracking in**: `~/go/src/gitlab.com/AccumulateNetwork/tracking_repo/`

### Development Workflow
1. **Start**: Update `../tracking_repo/repositories/accumulate/active.md`
2. **TDD Development**: Follow full TDD process with AI control structures
3. **Work Logging**: Log in `../tracking_repo/WORK_LOG.md` with commits
4. **MANDATORY Simplification**: Remove TDD complexity and AI artifacts before MR
5. **Code Review**: Focus on simplicity, maintainability, business clarity
6. **Create MR**: Update time in `../tracking_repo/TIME_TRACKING.md`
7. **Merge**: Only then move to `../tracking_repo/repositories/accumulate/completed.md`

### Branch Rules
- Create MR when work complete
- **NEVER** delete branches with active MRs
- Complete issues after merge to main or other open branches

### TDD Development Rules
- Follow full TDD process during development with AI control structures
- **MANDATORY**: Simplify and remove AI artifacts before creating merge requests
- Remove TDD scaffolding, excessive interfaces, AI-guidance comments
- Focus final code on business clarity and maintainability
- Maintain ≥80% test coverage after simplification

### Pre-Merge Simplification Checklist (MANDATORY)
Before any merge request, verify:
- [ ] Removed overly granular interfaces created only for AI guidance
- [ ] Consolidated artificially-separated functions  
- [ ] Eliminated excessive abstraction layers
- [ ] Removed redundant validation serving only AI development
- [ ] Cleaned AI-guidance comments and TODO markers
- [ ] Simplified function documentation to business purpose
- [ ] Verified test coverage remains ≥80% after simplification
- [ ] Confirmed business logic is clear and maintainable

### File Naming Standards
- **NEVER** use ALL_CAPS_FILE_NAMES
- **ALWAYS** use lowercase-with-hyphens
- **Reports**: Place in `../tracking_repo/repositories/accumulate/reviews/`

Reference: `~/go/src/gitlab.com/AccumulateNetwork/tracking_repo/CLAUDE.md`

## Repository-Specific Development Notes

### Accumulate Core Repository
- **Current Branch**: 3684-crosschain-healing
- **Devnet Testing**: Use AccumulateNetwork/Devnet repository for test platform
- **Command Example**: `devnet start 3684-crosschain-healing --dpm 5`