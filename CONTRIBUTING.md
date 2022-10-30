# Contributing

When contributing to this repository, please first discuss the change you wish to make via issue,
email, or any other method with the owners of this repository before making a change.

Please note we have a code of conduct, please follow it in all your interactions with the project.

## Commits

In general, the majority of commits should be between 100 and 1000 total lines
changed (additions + deletions). A large number of small commits may indicate
that the author is breaking up a large change into many small commits, which can
make it harder to follow the changes. Very large changes should be broken up
into smaller commits. There are valid cases where small commits are OK, such as
fixing typos or bugs.

Commit messages should be of the form `<type>(<scope>): <description>`. If the
commit cannot be fully described in one line, add a blank line followed by a
longer description. For example:

+ Simple

```markdown
  feat(api): add token issuers
```

+ With additional details

```markdown
  feat(api): add token issuers

  Add an API call to create a new token issuer.
```

The commit type should be one of the following (in order of priority):

| Code    | Description
| ------- | -----------
| `test`  | Only impacts tests
| `ci`    | Only impacts CI/CD
| `docs`  | Only impacts documentation
| `feat`  | Adds or updates features
| `fix`   | Fixes an issue
| `perf`  | Improves performance without adding/changing features or fixing bugs
| `chore` | Anything else, including refactoring, style changes, and code quality improvements

Based on [this list][2].

The following is an incomplete list of scopes:

| Code      | Description
| --------- | -----------
| `prot`    | Changes the protocol, such as transactions, accounts, and validators
| `api`     | Changes `internal/api`
| `general` | General changes (such as documentation and CI/CD)

[1]: https://www.conventionalcommits.org/en/v1.0.0/
[2]: https://github.com/conventional-changelog/commitlint/tree/master/%40commitlint/config-conventional#problems

## Submitting Changes

+ When you are ready to submit your work, create a Pull Request on GitHub
  merging your branch into `develop`. If you are a core team member, your branch
  name must be or begin with the Jira task. For example,
  `AC-123-anon-account-tx`.
+ The pull request description must clearly describe the changes made. If the
  changes affect functionality, the description must include how the changes can
  be verified. Otherwise, the description must state that the changes do not
  affect functionality.

## Reviewing Changes

+ In general, a pull request should receive at least some constructive
  criticism, in the form of a requested change - everyone has room for
  improvement.
+ **The most important aspects to consider when reviewing changes are
  readability and verification.**
+ A pull request must be small enough that it can be reasonably reviewed. If the
  request is too large, ask the author to split the changes into multiple pull
  requests.
+ If the code is difficult to read, leave a comment. This could be a request to
  add additional comments, or to rewrite a function in a more readable way.
+ If the code is not verified or verifiable, leave a comment. This could be a
  request to add unit tests, or to rewrite a function in a more testable way, or
  it could be a comment that the reviewer was unable to debug the changes.

## Code of Conduct

### Our Pledge

In the interest of fostering an open and welcoming environment, we as
contributors and maintainers pledge to making participation in our project and
our community a harassment-free experience for everyone, regardless of age, body
size, disability, ethnicity, gender identity and expression, level of experience,
nationality, personal appearance, race, religion, or sexual identity and
orientation.

### Our Standards

Examples of behavior that contributes to creating a positive environment
include:

* Using welcoming and inclusive language
* Being respectful of differing viewpoints and experiences
* Gracefully accepting constructive criticism
* Focusing on what is best for the community
* Showing empathy towards other community members

Examples of unacceptable behavior by participants include:

* The use of sexualized language or imagery and unwelcome sexual attention or
advances
* Trolling, insulting/derogatory comments, and personal or political attacks
* Public or private harassment
* Publishing others' private information, such as a physical or electronic
address, without explicit permission
* Other conduct which could reasonably be considered inappropriate in a
professional setting

### Our Responsibilities

Project maintainers are responsible for clarifying the standards of acceptable
behavior and are expected to take appropriate and fair corrective action in
response to any instances of unacceptable behavior.

Project maintainers have the right and responsibility to remove, edit, or
reject comments, commits, code, wiki edits, issues, and other contributions
that are not aligned to this Code of Conduct, or to ban temporarily or
permanently any contributor for other behaviors that they deem inappropriate,
threatening, offensive, or harmful.

### Scope

This Code of Conduct applies both within project spaces and in public spaces
when an individual is representing the project or its community. Examples of
representing a project or community include using an official project e-mail
address, posting via an official social media account, or acting as an appointed
representative at an online or offline event. Representation of a project may be
further defined and clarified by project maintainers.

### Enforcement

Instances of abusive, harassing, or otherwise unacceptable behavior may be
reported by contacting the project team at [INSERT EMAIL ADDRESS]. All
complaints will be reviewed and investigated and will result in a response that
is deemed necessary and appropriate to the circumstances. The project team is
obligated to maintain confidentiality with regard to the reporter of an incident.
Further details of specific enforcement policies may be posted separately.

Project maintainers who do not follow or enforce the Code of Conduct in good
faith may face temporary or permanent repercussions as determined by other
members of the project's leadership.

### Attribution

This Code of Conduct is adapted from the [Contributor Covenant][homepage], version 1.4,
available at [http://contributor-covenant.org/version/1/4][version]

[homepage]: http://contributor-covenant.org
[version]: http://contributor-covenant.org/version/1/4/