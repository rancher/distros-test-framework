# Migration Summary — `upstream/main` → `qa-infra-RC-1`

This branch becomes the RC that replaces `main`. It carries the **new framework
architecture/pattern** plus **all of main's content** (tests, fixes, features)
since the branches diverged (`ca7ff7d`, 2025-08-26).

The migration was done **piece by piece** (P1–P8), each reviewed and committed
separately on top of a baseline commit.

## Architecture mapping (main → this branch)

| main | this branch |
|---|---|
| `pkg/*` | `internal/pkg/*` |
| `shared/*` | **split** → `internal/resources/*`, `internal/report/`, `internal/provisioning/{driver,legacy,qainfra}/` |
| `modules/*` | `infrastructure/legacy/*` |
| `cmd/`, `entrypoint/`, `scripts/`, `workloads/`, `.github/`, `docs/`, `Makefile` | same path |

Common helpers were de-duplicated into `entrypoint/shared.go`
(`SetupClusterInfra`, `AfterSuite`, `ReportAfterSuite`, `CheckIngressCompat`, …),
and the field/structure of `driver.Cluster` was reorganised
(`Aws.AccessKeyID`, `SSH.User`, `SSH.KeyName`, `Bastion`, `SplitRolesConfig.Enabled`).

## Verification (final, committed state)

| Check | Result |
|---|---|
| `go build ./...` | exit 0 |
| `go vet ./...` (incl. tests) | exit 0 |
| `golangci-lint run ./...` | 0 issues |

## Footprint

- vs pre-migration HEAD (`144eb5b`): **138 files, +5,899 / −2,602**
- migration commits only (P1–P8): **98 files, +4,104 / −1,141**

---

## Commits

Commit order as built: `sync → p1 → p2 → p3 → p6 → p5 → p8 → p4 → p7`.

### `8cafe143` — sync with upstream *(baseline, not a migration piece)*
**69 files, +2,076 / −1,742.** The branch's new-architecture baseline that was
already in the working tree, committed first so the migration sits on top of it.
Includes `entrypoint/shared.go` (+197, the de-dup helper), the entrypoint suites
slimmed ~90 lines each as code moved into `shared.go`, the qa-infra provisioning
(`inventory.go` new, `ansible.go`, `opentofu.go`), the partial install-script
relocation, and `MIGRATION_AUDIT.md`. Establishes the "different pattern" target.

### `cc92ab6d` — P1: CI / Jenkins / Docker *(13 files, +366 / −150)*
- **Jenkinsfiles → == main**: `batch_install/airgap/upgrade` adopt main's
  data-driven `testToJobMap` refactor; `batch_os_validation` added (183 lines,
  #316/#318 batch-OS); `post_release_captain` (#273).
- **`release_checks.sh` → == main** (#277/#292/#314): `set_url`/`PRIME_URL`
  staging-for-RC, upgrade-installer count, **asset counts (see Decision #1)**.
- **Shell deltas**: `e2e_report.sh` (#309 log finder), `test_runner.sh` (charts
  flags), `kill-all_test.sh` + `uninstall_test.sh` (#326 killall path split).
- **Dockerfiles**: kept branch's newer base-image pin + **OpenTofu/Ansible**
  build tooling, and **added checksum hardening to the OpenTofu install**
  (matching main's terraform/kubectl/golangci pattern).
- *Why:* main is the CI source of truth; Dockerfiles must retain new-arch tooling.

### `013b45b7` — P2: Workflows · workloads · docs · Makefile *(15 files, +108 / −27)*
- **5 `.github/workflows` → == main**: brings **GH-Actions SHA pinning** (#321,
  supply-chain) + `master`→`main` branch refs.
- **Workloads → == main**: SUC plans (#327 split-role, #300 timeout),
  `nvidia-operator.yaml` (`version: v25.10.0` pin), `nvidia-benchmark.yaml`;
  `docs/nvidia.md` (#286 comprehensive doc).
- **Makefile**: kept branch (new-arch `infrastructure/legacy` + qainfra clean
  targets) + grafted main's charts test args, fixing main's missing `\`
  line-continuation and its `-expectedChartsValueUpgrade` typo.

### `67b3d773` — P3: modules → infrastructure/legacy *(19 files, +725 / −433)*
3-way merged main's `modules/*` changes onto branch `infrastructure/legacy/*`
(add main's changes, keep branch's). 15/21 ended `== main`; 6 kept
branch-specific for real reasons:
- **rke2 install**: adopted main's `profile_setup` (broader-OS CIS) + 5-min
  start-retry; **kept** branch's no-echo `rhel_password` security one-liner.
- **airgap `get_artifacts.sh` / `bastion_prepare.sh`**: branch supersets kept
  (extra checksum hardening, `pipefail`, fixed main's `result==""` retry bug)
  plus main's tee-logging / checksum-file guard. **(see Decision #2)**
- `k3s/master/instances_server.tf`: `ssh-keyscan → /tmp/known_hosts || true`.
- **Rename**: `cluster-level-pss.yaml` → `admission-config.yaml` (+ main's
  `EventRateLimit` admission plugin).

### `cdb6bd77` — P4: shared split *(8 files, +182 / −62)* — foundation
8 surgical grafts scattering main's `shared/*` into the new structure:
- `internal/resources/transfer.go`: `RunScp` → `prepareScpKey` (copy key to a
  writable path, `sync.Once`) + `scp -o StrictHostKeyChecking=no` (#326).
- `internal/resources/node.go`: + `CheckNodeCPUThreshold` /
  `parseNodeCPUPercentages` (#329 — this **unblocked P7**).
- `internal/resources/pod.go`: + `CleanupPod`. `logger.go`: `%w` passthrough.
  `ssh.go`: `ReturnLogError`→`fmt.Errorf` (no double-log) + log-level trims.
- `internal/report/report.go`: `nodeSummaryData(+flags)`; **union** of branch's
  `sudo cat` (perms) + main's warn-not-fail (#326 killall); NVIDIA section (#286);
  `SplitRoles.Add`→`.Enabled`.
- `driver/cluster.go` + `legacy/legacy.go`: `SplitRolesConfig.Add`→`Enabled` +
  `RoleOrder`; `addSplitRole` mappings-table refactor (#310).
- `RestartCluster`: no-op (both sides already removed it).

### `5d185a06` — P5: Go helpers *(5 files, +215 / −49)*
- `internal/pkg/customflag/{config,validate,validatejenkins}.go`: charts-version
  feature (#274) — `ExpectedChartsValue(Upgrade)` fields + Jenkins flag parsing
  (backs the P1/P2 charts flags); nvidia version flag (#285).
- `internal/pkg/assert/validate.go`, `internal/pkg/aws/ec2.go` (#278).

### `21b83270` — P6: qase reporter overhaul (#309) *(9 files, +1,793 / −186)*
- `internal/pkg/qase/process.go` (+442; `ciArch` threading, `FailureDetails`),
  `create.go`, `report.go`, **`slack.go` new (627)**, `cmd/qase/main.go`
  (Qase+Slack graceful degradation), **`cmd/rerunpoller/main.go` new (490)**.
- Also bundled the `make pre-commit` **lint cleanup** of
  `internal/provisioning/qainfra/inventory.go` (funlen split + `writeLine`
  helper) and `ansible.go` (appendCombine / nlreturn).

### `7a0ffd0f` — P7: testcase logic *(11 files, +488 / −193)*
9 clean 3-way merges + 2 airgap conflicts resolved. Beyond imports:
- **Field renames**: `cluster.BastionConfig`→`.Bastion`, `Aws.AwsUser`→`SSH.User`,
  `Aws.KeyName`→`SSH.KeyName`.
- **Repo-path adaptation**: `BasePath()+/modules/`→`/infrastructure/legacy/`
  (with a caught-and-reverted over-match of kernel `/lib/modules/` paths in
  `nvidia.go`).
- Features: `TestNodeCPUThreshold` (#329), nvidia per-OS driver + report (#286),
  tarball image checks (#310), node-status `timeouts` (#327), cluster
  reset/restore assertion unification (#305), cert-rotate timing (#286).

### `aa3b22f7` — P8: entrypoint suites *(18 files, +227 / −41)*
- **13 test files** 3-way merged (main's new tests + branch refactor):
  versionbump `cni*`/`components` (#274 charts), `upgradesuc` (#327),
  `validatecluster`/`reboot`/`restart` (#329 top-cmd), `dualstack`, `tarball`.
- **5 diverged suites** kept branch's `shared.go` refactor + grafted main's
  additions: charts flags, ENV_MODULE auto-set, `TestUninstallPolicy(…, true)`,
  airgap `validateAirgap` (#310 multus-Windows CNI), and **nvidia report
  integration** (`flags.Nvidia.Version` + `ReportAfterSuite` / `AfterSuite`).

---

## Key decisions & flags

### Decision #1 — `release_checks.sh` K3s asset count → **keep main's `16`**
`scripts/release_checks.sh` counts the files attached to a GitHub release
(`.assets | length`) and asserts an exact number. RKE2 = `74` (same on both).
K3s: main = `16`, branch previously = `19`.

**Decision: keep main's `16`** (already the committed value). If a current K3s
release actually ships a different number of assets, this assertion is the place
to update — it is a release-process magic number (main's `16` came from #314).

### Decision #2 — airgap branch-ahead supersets → **keep the branch's (more secure)**
`infrastructure/legacy/airgap/setup/get_artifacts.sh` (403 vs main 321) and
`bastion_prepare.sh` (145 vs main 144) are **supersets** of main: the branch has
the **same function set** as main plus extra checksum/safe-download hardening,
`set -eo pipefail`, and a fixed docker-install retry (main's `[ "$result" == "" ]`
check is never true). Confirmed the branch lacks **no** main function or behavior.

**Decision: keep the branch's versions.** Forcing `== main` would regress
hardening and re-introduce main's retry bug. Trade-off: these files now drift
from upstream and should ideally be upstreamed to main later.

### Other notes
- **Dockerfile base-image digest**: kept the branch's newer pin (not main's
  older one); all of main's checksum hardening is present.
- **P6 commit** bundles the `inventory.go`/`ansible.go` lint fixes (not strictly
  "reporter") — cosmetic grouping only.
- **P4-before-P7 ordering**: P7's `CheckNodeCPUThreshold` dependency is satisfied
  — P4 (`cdb6bd77`) precedes P7 (`7a0ffd0f`) in history.
