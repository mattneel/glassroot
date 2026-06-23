# Security Policy

Glassroot is pre-alpha. It is not yet safe for hostile or untrusted workloads, and it does not yet provide a hardened runner or sandbox security boundary.

## Reporting a vulnerability

Prefer GitHub private vulnerability reporting for security vulnerabilities once it is enabled for this repository. This file does not claim that private vulnerability reporting is currently enabled.

If GitHub private vulnerability reporting is unavailable, contact the repository maintainer @mattneel through a private channel listed on their GitHub profile. Do not include exploit details, proof-of-concept payloads, secrets, or sensitive target information in public issues, discussions, pull requests, or commit messages.

Use public issues for ordinary bugs, documentation errors, feature requests, and questions that do not expose security-sensitive details.

## Security-sensitive scope

Report privately when an issue could affect any of these areas:

- runner isolation or capability claims;
- safe source materialization and Git object handling;
- evidence integrity, provenance, hashing, bounds, or truncation reporting;
- policy evaluation, waivers, findings, or disposition handling;
- report rendering, sanitization, or publisher boundaries;
- GitHub workflows, permissions, webhook handling, or release automation;
- dependencies, supply-chain, or generated-artifact integrity;
- credential, token, signing-key, or secret handling;
- any path that could execute target code, fixture code, or untrusted content.

## What to include privately

When reporting privately, include enough information for maintainers to understand the issue without broad disclosure:

- affected commit, version, branch, or documentation section;
- a concise description of the security impact;
- reproduction steps that avoid publishing exploit details publicly;
- relevant logs or evidence with secrets removed;
- any known workaround or mitigation.

## Response expectations

Glassroot is early-stage and maintainer-led. Maintainers will triage security reports carefully, but this policy does not promise a fixed response or remediation timeline. Vulnerability handling may remain private during coordinated disclosure, especially when public details could help attackers before a fix or mitigation is available.

## Current limitations

Until the relevant milestones are implemented and reviewed, Glassroot should not be used to run external pull requests, hostile repositories, or untrusted workloads. Reports and documentation must not imply that the current development scaffold is a security boundary.
