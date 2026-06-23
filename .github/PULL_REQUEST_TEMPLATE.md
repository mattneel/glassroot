## Summary

### Narrow behavior changed

Describe the smallest behavior or documentation change this pull request makes.

### Threat or user need addressed

Explain the threat, user need, maintenance need, or project bootstrap requirement this addresses.

## Security impact

Describe the security impact in free form. Include whether this touches a trust boundary, changes a security invariant, alters evidence or policy behavior, changes workflow permissions, or affects credential handling. If there is no security impact, explain why.

## Trust boundaries touched

- [ ] Runner or workload isolation
- [ ] Source materialization or Git handling
- [ ] Evidence integrity, provenance, or bounds
- [ ] Policy, waivers, findings, or dispositions
- [ ] Report rendering or sanitization
- [ ] Publisher, GitHub integration, or credentials
- [ ] Workflows, dependencies, or release/signing
- [ ] None

## Security invariants affected

List the relevant invariant numbers from `KICKSTART.md`, or state that none are affected and why.

## Test and adversarial evidence

Describe the tests, checks, or adversarial cases run for this change. Include command output summaries for relevant checks.

## Added surface area

- [ ] Dependency
- [ ] Permission
- [ ] Network path
- [ ] Executable
- [ ] Generated artifact
- [ ] Workflow change
- [ ] Serialized schema change
- [ ] ADR required or updated
- [ ] None

If any box other than `None` is checked, explain the reason and review impact.

## Remaining limitations or uncertainty

List unsupported behavior, known gaps, follow-up work, or uncertainty that reviewers should consider.

## Verification

- [ ] `mise exec -- make verify` passed

Paste or summarize the verification output:

```text

```

## DCO sign-off

- [ ] Every commit includes a DCO `Signed-off-by:` trailer from `git commit --signoff`.
