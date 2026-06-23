# Glassroot report
- Report digest: ` sha256:fe73bfd569061146fe3b4b26cf6f17db6b14c06cdb1f85167ed1bcc26c41d116 `
- Run ID: ` gr12-control `
- Overall effective disposition: ` requires-review `
- Manifest verification: ` expected-manifest-digest `

## Notices
- ` fake-runner `: ` The fake runner is for tests and is not a security boundary. `
- ` network-deny-not-enforced `: ` The runner did not report enforced network deny. `
- ` no-target-code-executed `: ` The runner reported that no target code was executed. `
- ` observer-limitations-present `: ` Observer or capture limitations are present. `
- ` passed-is-not-proof-of-safety `: ` A passed disposition does not prove the code is safe. `
- ` synthetic-evidence `: ` Evidence is synthetic test data, not observed target behavior. `

## Runner
- Name: ` fake `
- Version: ` v1 `
- Isolation tier: ` fake `
- Executes target code: ` false `
- Synthetic evidence: ` true `
- Enforces network deny: ` false `

## Completeness
- Execution complete: ` true `
- Evidence complete: ` true `
- Bundle transaction valid: ` true `

## Findings
- Total findings: ` 1 `

### Finding
- Finding ID: ` finding-2191475b86b8ff9a9d4d7fb4c894d3debdebf62cb8ae8ba6895a6cae6388a934 `
- Origin: ` builtin-policy `
- Rule: ` GR-OBS-001 v1alpha1 `
- Title: ` Observation coverage incomplete or weakened `
- Severity: ` medium `
- Confidence: ` high `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` false `
- ` limitation synthetic-evidence `: ` Policy evaluation is based on synthetic evidence; it is not target workload behavior. `
- ` limitation synthetic-no-target-execution `: ` No target code was executed; absence of ordinary behavioral deltas is not a real workload pass. `

## Behavioral delta
- Total delta records: ` 0 `

## Authorities
- Trusted config path: ` .glassroot/pipeline.yaml `
- Trusted config head state: ` unchanged `
- Trusted waiver path: ` .glassroot/waivers.yaml `
- Trusted waiver base state: ` absent `
- Trusted waiver head state: ` absent-on-both `

## Limitations
- ` synthetic-no-target-execution `: ` No target code was executed by this runner. `
- ` synthetic-no-target-execution `: ` No target code was executed by this runner. `
- ` synthetic-no-target-execution `: ` No target code was executed by this runner. `
- ` synthetic-observations `: ` All observations are synthetic test data, not observed repository behavior. `
- ` synthetic-observations `: ` All observations are synthetic test data, not observed repository behavior. `
- ` synthetic-observations `: ` All observations are synthetic test data, not observed repository behavior. `

## Safety statement
A passed disposition does not prove the code is safe. Report digests are deterministic equality values only.
