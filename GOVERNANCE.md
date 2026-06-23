# Governance

Glassroot uses a simple maintainer-led governance model during its early development.

## Initial maintainer

The initial maintainer is @mattneel.

The maintainer is responsible for setting project direction, reviewing contributions, protecting security boundaries, and deciding when the governance model should evolve.

## Decision making

### Routine changes

Routine documentation, tests, small bug fixes, and narrow implementation changes may be accepted by maintainer review when they preserve the project scope and security invariants in [KICKSTART.md](KICKSTART.md).

### Architectural decisions

Consequential architectural choices require an Architecture Decision Record under [docs/adr/](docs/adr/). This includes changes to execution boundaries, evidence formats, trust zones, serialized schemas, runner capabilities, policy behavior, publisher separation, release/signing design, or other long-lived interfaces.

Use [docs/adr/0000-template.md](docs/adr/0000-template.md) for new ADRs.

### Security-sensitive decisions

Security-sensitive changes require explicit maintainer review. Coding-agent approval alone is insufficient for runners, materialization, evidence integrity, policy, publisher boundaries, workflows, dependencies, credential handling, or any change that could affect the non-negotiable security invariants.

Vulnerabilities may be handled privately during coordinated disclosure. Private handling is appropriate when public discussion could expose users, infrastructure, maintainers, or downstream repositories before a fix or mitigation is ready.

## Contributor participation

Design discussion and development should happen in public whenever doing so does not expose sensitive vulnerability details. Maintainers should explain decisions, document limitations, and preserve an evidence-based review culture.

## Evolution

This governance model is intentionally small. It can evolve as the contributor community grows, the project creates additional roles, or operational responsibilities become broader. Changes to governance should be documented in a pull request and, when consequential, accompanied by an ADR.
