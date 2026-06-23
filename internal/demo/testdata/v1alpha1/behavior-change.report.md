# Glassroot report
- Report digest: ` sha256:c50b8f791602981e5d874f8d1a87190b3abf0d3301c72e4e8a4d0dfd5a2db2bd `
- Run ID: ` gr12-behavior-change `
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
- Total findings: ` 7 `

### Finding
- Finding ID: ` finding-75a0294622ee9f683d7fe66627285424c3f7e97e39163e368a9b603ff4f9a1f1 `
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
- ` scenario `: ` install `
- ` delta record `: ` delta-75b56f711cd76f4cc6ef80d5e856213d4b44d8c95e79fc0a9494bcc6d403cfdd `
- ` evidence `: ` head/install/1 seq=18 event=evt-13005d22ded6d3f15de4cfb9ed1b5b1fcd0b78915d6288c13173be9060846669 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` evidence `: ` head/install/2 seq=29 event=evt-04d17159ad4d45916a7c6f02528b59ff2ce4fff63491312a857852e6ffb5f767 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Finding
- Finding ID: ` finding-05eae5f722ae23efdbba5f6f91bf2b66cce1756d7710df8b7b0ee3fced68e15d `
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
- ` scenario `: ` install `
- ` delta record `: ` delta-7d5bf2101be56e51c75ab3f979a67c137b037d4f2590321370be630bab3fdae6 `
- ` evidence `: ` head/install/1 seq=20 event=evt-25d0307764b677aed5d1763d79239a5fd08dfdf37932538503be25264e714766 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` evidence `: ` head/install/2 seq=31 event=evt-94fa3457f9d6e51899ee64a55bd3e525d07a5b4b0341c6331bff544e43e34f71 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Finding
- Finding ID: ` finding-bd044191894a54449a674c6f9c799376a50def8b9b0f59c37246d9bcf4b1647b `
- Origin: ` builtin-policy `
- Rule: ` GR-NET-001 v1alpha1 `
- Title: ` New or changed network behavior `
- Severity: ` high `
- Confidence: ` low `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `
- ` scenario `: ` install `
- ` delta record `: ` delta-735edebce7bdb5d8b1e525e9a9586811f83e1864f8260af0207eaf25173163cf `
- ` evidence `: ` head/install/1 seq=19 event=evt-e4edac6a5048effaa75fd427847c20c26af8153be64090a7d77fd2fd9688f55d stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` evidence `: ` head/install/2 seq=30 event=evt-443f8687dafa7c4b8c689164f909845b906a84b3860cb19ad8a28a934de43a2b stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Finding
- Finding ID: ` finding-e68c0b8e1266a22ed0e0e51fbbe2684c1b76f0c8cb108259b36f3903e1754735 `
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
- ` scenario `: ` install `
- ` delta record `: ` delta-7d5bf2101be56e51c75ab3f979a67c137b037d4f2590321370be630bab3fdae6 `
- ` evidence `: ` head/install/1 seq=20 event=evt-25d0307764b677aed5d1763d79239a5fd08dfdf37932538503be25264e714766 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` evidence `: ` head/install/2 seq=31 event=evt-94fa3457f9d6e51899ee64a55bd3e525d07a5b4b0341c6331bff544e43e34f71 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Finding
- Finding ID: ` finding-d82d9141440a53ab89ec9269b18aa9f9d34c8956b6ad7d439f1b0570cbb9f52c `
- Origin: ` builtin-policy `
- Rule: ` GR-ART-001 v1alpha1 `
- Title: ` New or changed artifact `
- Severity: ` medium `
- Confidence: ` low `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` true `
- Head observed: ` true `
- ` scenario `: ` install `
- ` delta record `: ` delta-ba8caca10924a45f5fbebd431707b6ff728bf16066c620cc9fd5f3fbba2a786a `
- ` evidence `: ` head/install/1 seq=16 event=evt-4c8da9ea9286d0dab076cde0ac19d408c9913b3faccbeb56288512fde4328bbf stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` evidence `: ` head/install/2 seq=27 event=evt-c8f6e0fe42c3aee6476599ea8353babe75ae79d22fa46fa0b45d6eec0d5fac11 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

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

### Finding
- Finding ID: ` finding-9d0660fdf2a658a0af307a4be0b5a27341dbe29ab5aeb0600475ba55106a3eff `
- Origin: ` builtin-policy `
- Rule: ` GR-PROC-001 v1alpha1 `
- Title: ` New process or executable `
- Severity: ` medium `
- Confidence: ` low `
- Original disposition: ` requires-review `
- Effective disposition: ` requires-review `
- Waived: ` false `
- Base observed: ` false `
- Head observed: ` true `
- ` scenario `: ` install `
- ` delta record `: ` delta-7bdd55264a8ce6065b1e47be004eb8ff6777cfb63aa25ada1bc389cc5b541963 `
- ` evidence `: ` head/install/1 seq=17 event=evt-cdcd81b9fd65d8e0ca600e7c8f9b7d663920ca2a9d067f3cb79f5d3d785846e9 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` evidence `: ` head/install/2 seq=28 event=evt-b570df208315ed326faeb437f45ce19c95bd37246812b46eaa809e84dcc4135c stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

## Behavioral delta
- Total delta records: ` 7 `

### Delta record
- Delta record ID: ` delta-7d5bf2101be56e51c75ab3f979a67c137b037d4f2590321370be630bab3fdae6 `
- Kind: ` added `
- Fact kind: ` artifact-activity `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` head fact `: ` artifact-activity sha256:95f90329e135855e9264fa097cfc457e498b279cfcb0f5b1cf5d0829ac32ca4a `
- ` artifact path `: ` \u{0040}workdir/bin/demo-helper `
- ` artifact digest `: ` sha256:89fe672c1baa15f1dc41d2e1cfffe23e7927f82e75c26466093cda119b8789a2 `
- ` artifact executable `: ` true `
- ` head fact `: ` artifact-activity sha256:95f90329e135855e9264fa097cfc457e498b279cfcb0f5b1cf5d0829ac32ca4a `
- ` artifact path `: ` \u{0040}workdir/bin/demo-helper `
- ` artifact digest `: ` sha256:89fe672c1baa15f1dc41d2e1cfffe23e7927f82e75c26466093cda119b8789a2 `
- ` artifact executable `: ` true `
- ` head evidence `: ` head/install/1 seq=20 event=evt-25d0307764b677aed5d1763d79239a5fd08dfdf37932538503be25264e714766 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=31 event=evt-94fa3457f9d6e51899ee64a55bd3e525d07a5b4b0341c6331bff544e43e34f71 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Delta record
- Delta record ID: ` delta-75b56f711cd76f4cc6ef80d5e856213d4b44d8c95e79fc0a9494bcc6d403cfdd `
- Kind: ` added `
- Fact kind: ` filesystem-write `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` head fact `: ` filesystem-write sha256:a28f7039ed1a0cfc36b1db5a1d96af1d112a61e1dee69ea465b1555df0520d0d `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/bin/demo-helper `
- ` path namespace `: ` workdir-root `
- ` executable `: ` true `
- ` head fact `: ` filesystem-write sha256:a28f7039ed1a0cfc36b1db5a1d96af1d112a61e1dee69ea465b1555df0520d0d `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/bin/demo-helper `
- ` path namespace `: ` workdir-root `
- ` executable `: ` true `
- ` head evidence `: ` head/install/1 seq=18 event=evt-13005d22ded6d3f15de4cfb9ed1b5b1fcd0b78915d6288c13173be9060846669 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=29 event=evt-04d17159ad4d45916a7c6f02528b59ff2ce4fff63491312a857852e6ffb5f767 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Delta record
- Delta record ID: ` delta-735edebce7bdb5d8b1e525e9a9586811f83e1864f8260af0207eaf25173163cf `
- Kind: ` added `
- Fact kind: ` network-connection `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` head fact `: ` network-connection sha256:cbbe822926978c0234a8178ca4b7e4afab5e85dc006fe24f0c839bc1eb27812c `
- ` network operation `: ` connect `
- ` network destination `: ` canary.invalid `
- ` network result `: ` denied `
- ` head fact `: ` network-connection sha256:cbbe822926978c0234a8178ca4b7e4afab5e85dc006fe24f0c839bc1eb27812c `
- ` network operation `: ` connect `
- ` network destination `: ` canary.invalid `
- ` network result `: ` denied `
- ` head evidence `: ` head/install/1 seq=19 event=evt-e4edac6a5048effaa75fd427847c20c26af8153be64090a7d77fd2fd9688f55d stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=30 event=evt-443f8687dafa7c4b8c689164f909845b906a84b3860cb19ad8a28a934de43a2b stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Delta record
- Delta record ID: ` delta-0f01e678e15782202816f1f15fe160ebb5a04c80207aded2d90eedbd4c3a30ca `
- Kind: ` added `
- Fact kind: ` process-exit `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` head fact `: ` process-exit sha256:fc9b2ccbde59e5b130cf9b346c90014bcf29b0943f000c1ad932b3a93935538b `
- ` process operation `: ` exit `
- ` process id `: ` proc-85331084b58919591b369695451abdc41a03b04373bab60231a3ab4cf3bbaf85 `
- ` executable `: ` \u{0040}workdir/bin/demo-parent `
- ` head fact `: ` process-exit sha256:fc9b2ccbde59e5b130cf9b346c90014bcf29b0943f000c1ad932b3a93935538b `
- ` process operation `: ` exit `
- ` process id `: ` proc-85331084b58919591b369695451abdc41a03b04373bab60231a3ab4cf3bbaf85 `
- ` executable `: ` \u{0040}workdir/bin/demo-parent `
- ` head evidence `: ` head/install/1 seq=21 event=evt-5aaedfb347f043ec5a666c866e34ba8db8514b5daf781893d01d5e4c331aed09 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=32 event=evt-93cddfdd2b5c4923d268fe1943add8f75679f57b692bd12ea695e3f87eaa3f2d stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Delta record
- Delta record ID: ` delta-7bdd55264a8ce6065b1e47be004eb8ff6777cfb63aa25ada1bc389cc5b541963 `
- Kind: ` added `
- Fact kind: ` process-start `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` false `
- Head observed: ` true `
- Base occurrence: ` coverage=none repeatability=not-assessable total=0 min=0 max=0 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` head fact `: ` process-start sha256:f3357ef67c8ef81bd35391b7f1f66dae99cff8cb6ad1503e88d6af0056720f82 `
- ` process operation `: ` start `
- ` process id `: ` proc-85331084b58919591b369695451abdc41a03b04373bab60231a3ab4cf3bbaf85 `
- ` executable `: ` \u{0040}workdir/bin/demo-helper `
- ` head fact `: ` process-start sha256:f3357ef67c8ef81bd35391b7f1f66dae99cff8cb6ad1503e88d6af0056720f82 `
- ` process operation `: ` start `
- ` process id `: ` proc-85331084b58919591b369695451abdc41a03b04373bab60231a3ab4cf3bbaf85 `
- ` executable `: ` \u{0040}workdir/bin/demo-helper `
- ` head evidence `: ` head/install/1 seq=17 event=evt-cdcd81b9fd65d8e0ca600e7c8f9b7d663920ca2a9d067f3cb79f5d3d785846e9 stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=28 event=evt-b570df208315ed326faeb437f45ce19c95bd37246812b46eaa809e84dcc4135c stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Delta record
- Delta record ID: ` delta-ba8caca10924a45f5fbebd431707b6ff728bf16066c620cc9fd5f3fbba2a786a `
- Kind: ` modified `
- Fact kind: ` artifact-activity `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` true `
- Head observed: ` true `
- Base occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` base fact `: ` artifact-activity sha256:d88b8d7a27c100ff6d40c4f3c620d74c2f7cc5656322904acda491dc1da35c7d `
- ` artifact path `: ` \u{0040}workdir/out/result.txt `
- ` artifact digest `: ` sha256:baeebb879d1cfc74ae3b095890d568e8d1bbfe1f7ff129d6e9aea8ac97339b1a `
- ` artifact executable `: ` false `
- ` base fact `: ` artifact-activity sha256:d88b8d7a27c100ff6d40c4f3c620d74c2f7cc5656322904acda491dc1da35c7d `
- ` artifact path `: ` \u{0040}workdir/out/result.txt `
- ` artifact digest `: ` sha256:baeebb879d1cfc74ae3b095890d568e8d1bbfe1f7ff129d6e9aea8ac97339b1a `
- ` artifact executable `: ` false `
- ` head fact `: ` artifact-activity sha256:7e3f5e35c8170909417be491bbe472314811f520372fde543b431c5ca47bf728 `
- ` artifact path `: ` \u{0040}workdir/out/result.txt `
- ` artifact digest `: ` sha256:2077b36ddd633fbdbc66c7201c9fd38292f6ac538313cbe514467468cb62275a `
- ` artifact executable `: ` false `
- ` head fact `: ` artifact-activity sha256:7e3f5e35c8170909417be491bbe472314811f520372fde543b431c5ca47bf728 `
- ` artifact path `: ` \u{0040}workdir/out/result.txt `
- ` artifact digest `: ` sha256:2077b36ddd633fbdbc66c7201c9fd38292f6ac538313cbe514467468cb62275a `
- ` artifact executable `: ` false `
- ` changed field `: ` artifact.digest `
- ` base evidence `: ` base/install/1 seq=4 event=evt-114abe958cea23d6146a32f385f86a575804e393d5f3dcced6b717bb9b403eaf stream=attempts/base/install/repetition-0001/events.jsonl digest=sha256:ef5fa308a0092615c8ecae4791d38c3d65840bc0d3afeb254403441b64eaa288 `
- ` base evidence `: ` base/install/2 seq=10 event=evt-caea637cbb481af335f82ee7221ea994f66f3a6f4b764acdb9f3f570252d38e6 stream=attempts/base/install/repetition-0002/events.jsonl digest=sha256:5c3a3aa070554c63dbb021f862a4655a0a03efbdf1e266b970136defd541b7e3 `
- ` head evidence `: ` head/install/1 seq=16 event=evt-4c8da9ea9286d0dab076cde0ac19d408c9913b3faccbeb56288512fde4328bbf stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=27 event=evt-c8f6e0fe42c3aee6476599ea8353babe75ae79d22fa46fa0b45d6eec0d5fac11 stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

### Delta record
- Delta record ID: ` delta-6407d865f865146c61acd81e209f7bb89b6073a456961e57760ff1b2e9c05d1e `
- Kind: ` modified `
- Fact kind: ` filesystem-write `
- Source: ` synthetic-test-generated `
- Basis: ` complete-observation `
- Base observed: ` true `
- Head observed: ` true `
- Base occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- Head occurrence: ` coverage=complete repeatability=stable total=2 min=1 max=1 `
- ` base fact `: ` filesystem-write sha256:d1841c7e7fbe99b06cb0f3b3bfe81ae1a2454ecba9383864bdad9f3575be2fad `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/out/result.txt `
- ` path namespace `: ` workdir-root `
- ` executable `: ` false `
- ` base fact `: ` filesystem-write sha256:d1841c7e7fbe99b06cb0f3b3bfe81ae1a2454ecba9383864bdad9f3575be2fad `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/out/result.txt `
- ` path namespace `: ` workdir-root `
- ` executable `: ` false `
- ` head fact `: ` filesystem-write sha256:d8f60e828e6aac78a304ab6b65e98c2123620b2a92f538e12b16b322f11e3dfd `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/out/result.txt `
- ` path namespace `: ` workdir-root `
- ` executable `: ` false `
- ` head fact `: ` filesystem-write sha256:d8f60e828e6aac78a304ab6b65e98c2123620b2a92f538e12b16b322f11e3dfd `
- ` filesystem operation `: ` write `
- ` filesystem path `: ` \u{0040}workdir/out/result.txt `
- ` path namespace `: ` workdir-root `
- ` executable `: ` false `
- ` changed field `: ` filesystem.digest `
- ` base evidence `: ` base/install/1 seq=3 event=evt-a157b086374c1ff367016bb77dd169c6bb3cdf52635f28564b47b1ef7f87857c stream=attempts/base/install/repetition-0001/events.jsonl digest=sha256:ef5fa308a0092615c8ecae4791d38c3d65840bc0d3afeb254403441b64eaa288 `
- ` base evidence `: ` base/install/2 seq=9 event=evt-dec49e461cd95b4e0b8f38b2bfb2f6a02af5b4455ab10873a63459ce365382a8 stream=attempts/base/install/repetition-0002/events.jsonl digest=sha256:5c3a3aa070554c63dbb021f862a4655a0a03efbdf1e266b970136defd541b7e3 `
- ` head evidence `: ` head/install/1 seq=15 event=evt-2b545748e7bf0d4ee239b23224c79f9190bf239c9a7ad9990d30e6863107807b stream=attempts/head/install/repetition-0001/events.jsonl digest=sha256:a23498b58a64bbea05c450ba57e64a4a4caa65737edfa04024924ae3e929489b `
- ` head evidence `: ` head/install/2 seq=26 event=evt-8cd45ac79d767740afd7f5c6c2263c3b14726a55269d2bbb4b8c44abe1271d1f stream=attempts/head/install/repetition-0002/events.jsonl digest=sha256:713293745a6eb2a8cf95574e70dc5c8b2982a5fcac468f8b807a4ed8aa709347 `

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
