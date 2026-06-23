# Contributing to Glassroot

Glassroot is pre-alpha security tooling. It is not yet suitable for running hostile or untrusted workloads, and early changes must preserve the security posture described in [KICKSTART.md](KICKSTART.md).

## Contribution principles

- Keep each pull request narrow, reviewable, and independently revertible.
- Complete one issue or milestone at a time; do not combine unrelated bootstrap work.
- Prefer small, testable vertical slices over speculative frameworks.
- Do not add dependencies, permissions, network paths, generated artifacts, executable files, runners, or workflow changes without explaining the need in the pull request.
- Treat repository content, logs, filenames, pull request text, and generated output as untrusted data.
- Do not execute target or fixture code unless a milestone explicitly permits it and the runner boundary is documented.

## Developer Certificate of Origin

Glassroot uses the Developer Certificate of Origin (DCO) instead of a CLA initially. Every commit must include a DCO sign-off line.

Sign commits with:

```bash
git commit --signoff
```

This adds a `Signed-off-by:` trailer confirming that you have the right to contribute the work under the project license.

## Local verification

Before submitting a pull request, run:

```bash
mise exec -- make verify
```

Include the result in the pull request template. `make verify` is the local CI-equivalent command and must not execute untrusted fixtures through a non-fake runner.

## Security-sensitive work

Read the non-negotiable security invariants and pull request requirements in [KICKSTART.md](KICKSTART.md) before touching security-sensitive areas. Security-sensitive changes include runners, source materialization, evidence integrity, policy evaluation, report rendering, publisher boundaries, workflow permissions, dependencies, and credential handling.

For security-sensitive code or documentation, coding-agent approval alone is insufficient. At least one human maintainer review is required before merge.

## Reporting vulnerabilities

Do not report security vulnerabilities through public issues. See [SECURITY.md](SECURITY.md) for private reporting guidance. Public issues are appropriate for ordinary bugs, documentation fixes, feature discussion, and non-sensitive questions.

## Pull request checklist

Every Glassroot pull request should answer the questions listed in [KICKSTART.md](KICKSTART.md), including:

1. What narrow behavior changes?
2. Which threat or user need does it address?
3. Does it touch a trust boundary?
4. Which security invariants could it affect?
5. What tests demonstrate expected and adversarial behavior?
6. Does it add dependencies, permissions, network paths, executables, generated artifacts, or workflow changes?
7. Does it change a serialized schema or require an ADR?
8. What remains unsupported or uncertain?

Use the repository pull request template and keep limitations honest.
