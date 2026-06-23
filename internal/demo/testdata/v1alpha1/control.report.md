# Glassroot report
- Report digest: ` sha256:d89f852b544fb726d41beb1048522503875a888903228d39a647b9621bc126df `
- Run ID: ` gr12-control `
- Overall effective disposition: ` passed `
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
- Total findings: ` 0 `

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
