# Glassroot report
- Report digest: ` sha256:42286dee5d560ce50007cb0ee9c71aed215a04fab0d2439e7ef642913f365e07 `
- Run ID: ` run-0001 `
- Overall effective disposition: ` requires-review `
- Manifest verification: ` expected-manifest-digest `

## Notices
- ` fake-runner `: ` The fake runner is for tests and is not a security boundary. `
- ` governance-findings-present `: ` Configuration or waiver governance findings are present. `
- ` network-deny-not-enforced `: ` The runner did not report enforced network deny. `
- ` no-target-code-executed `: ` The runner reported that no target code was executed. `
- ` observer-limitations-present `: ` Observer or capture limitations are present. `
- ` passed-is-not-proof-of-safety `: ` A passed disposition does not prove the code is safe. `
- ` synthetic-evidence `: ` Evidence is synthetic test data, not observed target behavior. `
- ` waivers-applied `: ` One or more findings are effectively waived but remain visible. `

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
- Total findings: ` 8 `

### Finding
- Finding ID: ` finding-0aaed9cd1c3bf9a563ba6a11af73ef0269ecd4d3be283cbe18ae6edc7c16109d `
- Origin: ` builtin-policy `
- Rule: ` GR-FS-001 v1alpha1 `
- Title: ` New executable file or artifact `
- Severity: ` high `
- Confidence: ` low `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `
- ` scenario `: ` build `
- ` delta record `: ` delta-f88e96dbc40d7481d18c5d79857af23b723d87f88ca91b1a9dce00702d9929c7 `
- ` evidence `: ` head/build/1 seq=11 event=evt-f53b082e5d5a0de9485019b6a0b2c58476e8449f47af3263d93f7148868df600 stream=attempts/head/build/repetition-0001/events.jsonl digest=sha256:e53610475deafd4ce40ef0080f78e049055d9b5c332e13125e3b68f6f9cb5f9d `

### Finding
- Finding ID: ` finding-8a46c9bf52e088442cdd89dfb7a6e4c67b53aeb4b6bf33a33ff86d4f0f827820 `
- Origin: ` trusted-configuration `
- Rule: ` GR-CONFIG-001 v1 `
- Title: ` Trusted security configuration changed in head `
- Severity: ` high `
- Confidence: ` high `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `

### Finding
- Finding ID: ` finding-1ac2afb46943f3cac5aadc2ad38b2c3e7aff3136eeb76c4c556ffb68d9b14517 `
- Origin: ` waiver-governance `
- Rule: ` GR-WAIVER-001 v1 `
- Title: ` Waiver governance issue `
- Severity: ` high `
- Confidence: ` high `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `

### Finding
- Finding ID: ` finding-31c8dbaeba632dd6d2e6e23d431b1b3673d5eae566136ec7f176d1a28f83a42a `
- Origin: ` waiver-governance `
- Rule: ` GR-WAIVER-001 v1 `
- Title: ` Waiver governance issue `
- Severity: ` high `
- Confidence: ` high `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `

### Finding
- Finding ID: ` finding-201fc9863da46181019613ced1a1c260dbe8dc6903acf316907e3741a9e13b2c `
- Origin: ` builtin-policy `
- Rule: ` GR-ART-001 v1alpha1 `
- Title: ` New or changed artifact `
- Severity: ` medium `
- Confidence: ` low `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `
- ` scenario `: ` build `
- ` delta record `: ` delta-f88e96dbc40d7481d18c5d79857af23b723d87f88ca91b1a9dce00702d9929c7 `
- ` evidence `: ` head/build/1 seq=11 event=evt-f53b082e5d5a0de9485019b6a0b2c58476e8449f47af3263d93f7148868df600 stream=attempts/head/build/repetition-0001/events.jsonl digest=sha256:e53610475deafd4ce40ef0080f78e049055d9b5c332e13125e3b68f6f9cb5f9d `

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

### Finding
- Finding ID: ` finding-f1d1f24700f67940b33a502d99d2605e7528592700829c37d2546712d22b3137 `
- Origin: ` waiver-governance `
- Rule: ` GR-WAIVER-001 v1 `
- Title: ` Waiver governance issue `
- Severity: ` medium `
- Confidence: ` high `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `

### Finding
- Finding ID: ` finding-50868309030c5f8fd05ede5750b07e8218aba9071ecdedb116e0beb313f13959 `
- Origin: ` builtin-policy `
- Rule: ` GR-NET-001 v1alpha1 `
- Title: ` New or changed network behavior `
- Severity: ` high `
- Confidence: ` low `
- Original disposition: ` requires-review `
- Effective disposition: ` waived `
- Waived: ` true `
- Base observed: ` false `
- Head observed: ` true `
- ` scenario `: ` test `
- ` delta record `: ` delta-3037b41d02883144442454837fa74a4bb635cb7b55c55e6110197f68c2783eb5 `
- Applied waiver: ` known-network `
- Waiver owner: ` mattneel `
- Waiver reason: ` Known deterministic fixture behavior pending removal. `
- Waiver issued at: ` 2026-06-23T00:00:00Z `
- Waiver expires at: ` 2026-07-23T00:00:00Z `
- ` evidence `: ` head/test/1 seq=8 event=evt-5b4d54df9b65f3c36c9363c52c55aa635226ddb09f06f267111c0d691e5a4459 stream=attempts/head/test/repetition-0001/events.jsonl digest=sha256:19a6a00ce17e79d05fc47984c0ca15b540bf2d9e49a9254dcad4b0f488cb620d `

## Behavioral delta
- Total delta records: ` 7 `

### Delta record
- Delta record ID: ` delta-f88e96dbc40d7481d18c5d79857af23b723d87f88ca91b1a9dce00702d9929c7 `
- Kind: ` added `
- Fact kind: ` artifact-activity `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- ` head fact `: ` artifact-activity sha256:35b87fa9b678955a58adb401415b7037988598e3101be055e059e2dced84b37f `
- ` artifact path `: ` \u{0040}workdir/bin/glassroot `
- ` artifact digest `: ` sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa `
- ` artifact executable `: ` true `
- ` head evidence `: ` head/build/1 seq=11 event=evt-f53b082e5d5a0de9485019b6a0b2c58476e8449f47af3263d93f7148868df600 stream=attempts/head/build/repetition-0001/events.jsonl digest=sha256:e53610475deafd4ce40ef0080f78e049055d9b5c332e13125e3b68f6f9cb5f9d `

### Delta record
- Delta record ID: ` delta-b7ecc85980bd011b40395226dbec7f7795188e9e643685749b1731237cd07174 `
- Kind: ` removed `
- Fact kind: ` filesystem-write `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` true `
- Head observed: ` false `
- Base occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- Head occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- ` base fact `: ` filesystem-write sha256:b42265169039cf486cb73573dc514b87a4219e7c10bbfef8960e4aaed55bd894 `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/bin/glassroot `
- ` path namespace `: ` workdir-root `
- ` executable `: ` false `
- ` base evidence `: ` base/build/1 seq=5 event=evt-4be47782a351fb5e425a6424b545e7d5e02684e34459793a6268eb0528201b77 stream=attempts/base/build/repetition-0001/events.jsonl digest=sha256:fd2370101fa90ec37fd4854957dc2269ed5e7c029a4616f886656b956bf87659 `

### Delta record
- Delta record ID: ` delta-3aa69b37dd444bfe873e6219c96dd3ff67ef8ee5f845ae7fd19747e7fcd414d6 `
- Kind: ` modified `
- Fact kind: ` scenario-completed `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` true `
- Head observed: ` true `
- Base occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- Head occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- ` base fact `: ` scenario-completed sha256:c8ff5eda34f0933d0e7a6f56155d6c180361abd0bb9ad827a1d88a0593bf48b5 `
- ` scenario status `: ` passed `
- ` head fact `: ` scenario-completed sha256:624e637bce35608a03eaefa31ed36f1e996f9754f135041606dbc9ada6f8bb54 `
- ` scenario status `: ` passed `
- ` changed field `: ` scenario.durationMillis `
- ` base evidence `: ` base/build/1 seq=6 event=evt-a85b58a06a1ce8d76ba4240c496f394c61f90d327bac913c4b73bd78d7f3459a stream=attempts/base/build/repetition-0001/events.jsonl digest=sha256:fd2370101fa90ec37fd4854957dc2269ed5e7c029a4616f886656b956bf87659 `
- ` head evidence `: ` head/build/1 seq=12 event=evt-e0a932c137a4fe74d6a9354e51af80bf4fc3e21f3d4c9bdd95a38baba1405fb0 stream=attempts/head/build/repetition-0001/events.jsonl digest=sha256:e53610475deafd4ce40ef0080f78e049055d9b5c332e13125e3b68f6f9cb5f9d `

### Delta record
- Delta record ID: ` delta-3037b41d02883144442454837fa74a4bb635cb7b55c55e6110197f68c2783eb5 `
- Kind: ` added `
- Fact kind: ` network-connection `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- ` head fact `: ` network-connection sha256:118cceafd402ec9771a65802270d98a54a6eb026ae55966ececf985496e67446 `
- ` network operation `: ` connect `
- ` network destination `: ` canary.example.invalid `
- ` network result `: ` denied `
- ` head evidence `: ` head/test/1 seq=8 event=evt-5b4d54df9b65f3c36c9363c52c55aa635226ddb09f06f267111c0d691e5a4459 stream=attempts/head/test/repetition-0001/events.jsonl digest=sha256:19a6a00ce17e79d05fc47984c0ca15b540bf2d9e49a9254dcad4b0f488cb620d `

### Delta record
- Delta record ID: ` delta-22710470607ef3cb81e6a805fa7a2d572317349a1371f1ecbfdcf7566f33e2df `
- Kind: ` added `
- Fact kind: ` scenario-completed `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- ` head fact `: ` scenario-completed sha256:de2b2c5cecd70fd52282af912b13e3aacea0983c490e7e44ecb1647bec1292d2 `
- ` scenario status `: ` failed `
- ` head evidence `: ` head/test/1 seq=9 event=evt-cfca48915c7d705be42b94a4e670e71776044d5dc3e35cd6d923bbd2e345546b stream=attempts/head/test/repetition-0001/events.jsonl digest=sha256:19a6a00ce17e79d05fc47984c0ca15b540bf2d9e49a9254dcad4b0f488cb620d `

### Delta record
- Delta record ID: ` delta-3b367e4d6383d9fa4ae64f73a3d05d62499abc5aef19ee40a376c81bb002808f `
- Kind: ` removed `
- Fact kind: ` process-start `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` true `
- Head observed: ` false `
- Base occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- Head occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- ` base fact `: ` process-start sha256:2ee320d119a12a6f80e6300c3677f84fae987e805e9cfd1ef30bd0c62a781fda `
- ` process operation `: ` start `
- ` process id `: ` proc-29d38cbe70581b433a955aee951b3ddc47850c01ef3766788118d5cebe79efba `
- ` executable `: ` \u{0040}workdir/bin/tester `
- ` base evidence `: ` base/test/1 seq=2 event=evt-a90c37095ea81d5236bde6df71362a657ab3b9fa7a2b833591bec867b4c36a49 stream=attempts/base/test/repetition-0001/events.jsonl digest=sha256:d76405061d691434dc4d1d6259e81bc00d08824092d8c9cd46e3920f84116fcf `

### Delta record
- Delta record ID: ` delta-c5b65d1e0806051163cd3f6e9bb3df016c8e8ada0a2d544ce660a8349314069d `
- Kind: ` removed `
- Fact kind: ` scenario-completed `
- Source: ` synthetic-test-generated `
- Basis: ` single-sample `
- Base observed: ` true `
- Head observed: ` false `
- Base occurrence: ` coverage=complete repeatability=single-sample total=1 min=1 max=1 `
- Head occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- ` base fact `: ` scenario-completed sha256:ec95d691dec3cb8eb7fa3534b71844a426634b47b21f7fa63227a91852bb89fc `
- ` scenario status `: ` passed `
- ` base evidence `: ` base/test/1 seq=3 event=evt-e8c92c3915a9d39a2fc51caca079cb4f646c399dff88b32115aa71f2a369bdc6 stream=attempts/base/test/repetition-0001/events.jsonl digest=sha256:d76405061d691434dc4d1d6259e81bc00d08824092d8c9cd46e3920f84116fcf `

## Authorities
- Trusted config path: ` .glassroot/pipeline.yaml `
- Trusted config head state: ` modified-valid `
- Trusted waiver path: ` .glassroot/waivers.yaml `
- Trusted waiver base state: ` valid `
- Trusted waiver head state: ` modified-valid `

## Limitations
- ` synthetic-no-target-execution `: ` No target code was executed by this runner. `
- ` synthetic-no-target-execution `: ` No target code was executed by this runner. `
- ` synthetic-no-target-execution `: ` No target code was executed by this runner. `
- ` synthetic-observations `: ` All observations are synthetic test data, not observed repository behavior. `
- ` synthetic-observations `: ` All observations are synthetic test data, not observed repository behavior. `
- ` synthetic-observations `: ` All observations are synthetic test data, not observed repository behavior. `

## Safety statement
A passed disposition does not prove the code is safe. Report digests are deterministic equality values only.
