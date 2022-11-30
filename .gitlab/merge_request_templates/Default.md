Closes AC-0000. *Add a more detailed description here.*

## Review Checklist

**If any item is not complete, the merge request is not ready to be reviewed and must be marked `Draft:`.**

- [ ] The merge request title is in the format `<change type>(<change scope>): <short description>`
  - For example, `feat(cli): add QR code generation`
  - For details, see [CONTRIBUTING.md](/CONTRIBUTING.md)
- [ ] The description includes `Closes <jira task ID>` (or rarely `Updates <jira task ID>`)
- [ ] The change is fully validated by tests that are run during CI
  - In most cases this means a test in "validate.sh"
  - In some cases, a Go test may be acceptable
  - Validation is not applicable to things like documentation updates
  - Purely UI/UX changes can be manually validated, such as changes to human-readable output
  - For all other changes, automated validation tests are an absolute requirement unless a maintainer specifically explains why they are not in a comment on this merge request
- [ ] The change is marked with one of the validation labels
  - ~"validation::ci/cd" for changes validated by CI tests
  - ~"validation::manual" for changes validated by hand
  - ~"validation::deferred" for changes validated by a follow up merge request
  - ~"validation::not applicable" for changes where validation is not applicable

## Merge Checklist

- [ ] CI is passing
- [ ] Merge conflicts are resolved
- [ ] All discussions are resolved