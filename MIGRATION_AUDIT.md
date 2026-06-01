# Migration audit: upstream/main → qa-infra-RC-1

**Diverged**: 2025-08-26 at `ca7ff7dc963e`
**Commits on main since divergence**: 26
**Commits on qa-infra-RC-1 since divergence**: 7

**Path-rename convention (this branch refactored):**
- `pkg/*` on main → `internal/pkg/*` here
- `shared/*` on main → `internal/resources/*` here
- everything else: same path

---

## 1. NEW files on main (no equivalent here)

### `cmd/rerunpoller/main.go`
- 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
- 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)

### `pkg/qase/slack.go`
- 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
- ee795d9 Add.batch os (#316) (fmoral2, 2026-03-13)
- 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)

### `scripts/Jenkinsfile_batch_os_validation`
- ee795d9 Add.batch os (#316) (fmoral2, 2026-03-13)
- cd46233 fix replace tfvars lock (#317) (fmoral2, 2026-03-16)
- 52fcd63 Add.batch os (#318) (fmoral2, 2026-03-19)
- 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)

### `scripts/Jenkinsfile_post_release_captain`
- 6194a18 fix captain and add post release jenkins file (#273) (fmoral2, 2025-09-08)

---

## 2. MODIFIED files on main, grouped

### testcase logic  (11 files)

- `pkg/testcase/certrotate.go`  →  `internal/pkg/testcase/certrotate.go`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
- `pkg/testcase/clusterreset.go`  →  `internal/pkg/testcase/clusterreset.go`  ✓
  - 1 file changed, 18 insertions(+), 17 deletions(-)
  - 5a3494e test fix for cluster reset and restore rke2 (#305) (Archana Ganesh, 2026-01-20)
- `pkg/testcase/clusterrestore.go`  →  `internal/pkg/testcase/clusterrestore.go`  ✓
  - 1 file changed, 4 insertions(+), 15 deletions(-)
  - 5a3494e test fix for cluster reset and restore rke2 (#305) (Archana Ganesh, 2026-01-20)
- `pkg/testcase/node.go`  →  `internal/pkg/testcase/node.go`  ✓
  - 1 file changed, 57 insertions(+), 2 deletions(-)
  - 92728b7 Suc plan change and node status timeout update (#327) (Archana Ganesh, 2026-05-06)
  - 71033de Top command (#329) (ERYN Tennis, 2026-05-21)
- `pkg/testcase/nvidia.go`  →  `internal/pkg/testcase/nvidia.go`  ✓
  - 1 file changed, 196 insertions(+), 27 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 7e76594  fix sles nvidia test (fmoral2, 2026-02-04)
- `pkg/testcase/selinux.go`  →  `internal/pkg/testcase/selinux.go`  ✓
  - 1 file changed, 33 insertions(+), 41 deletions(-)
  - b2a5a58 adding selinux test to validate cluster test (#315) (ERYN Tennis, 2026-04-13)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `pkg/testcase/support/airgap.go`  →  `internal/pkg/testcase/support/airgap.go`  ✓
  - 1 file changed, 103 insertions(+), 37 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `pkg/testcase/support/airgapwindows.go`  →  `internal/pkg/testcase/support/airgapwindows.go`  ✓
  - 1 file changed, 24 insertions(+), 16 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
- `pkg/testcase/support/ipv6only.go`  →  `internal/pkg/testcase/support/ipv6only.go`  ✓
  - 1 file changed, 28 insertions(+), 25 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
- `pkg/testcase/tarball.go`  →  `internal/pkg/testcase/tarball.go`  ✓
  - 1 file changed, 12 insertions(+)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `pkg/testcase/upgradenodereplacement.go`  →  `internal/pkg/testcase/upgradenodereplacement.go`  ✓
  - 1 file changed, 4 insertions(+), 4 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)

### qase reporter  (3 files)

- `pkg/qase/create.go`  →  `internal/pkg/qase/create.go`  ✓
  - 1 file changed, 59 insertions(+), 43 deletions(-)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
- `pkg/qase/process.go`  →  `internal/pkg/qase/process.go`  ✓
  - 1 file changed, 396 insertions(+), 46 deletions(-)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
  - ee795d9 Add.batch os (#316) (fmoral2, 2026-03-13)
- `pkg/qase/report.go`  →  `internal/pkg/qase/report.go`  ✓
  - 1 file changed, 78 insertions(+), 22 deletions(-)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)

### customflag  (3 files)

- `pkg/customflag/config.go`  →  `internal/pkg/customflag/config.go`  ✓
  - 1 file changed, 20 insertions(+), 3 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
  - a739922 add nvidia report (#285) (fmoral2, 2025-10-27)
- `pkg/customflag/validate.go`  →  `internal/pkg/customflag/validate.go`  ✓
  - 1 file changed, 161 insertions(+), 35 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `pkg/customflag/validatejenkins.go`  →  `internal/pkg/customflag/validatejenkins.go`  ✓
  - 1 file changed, 32 insertions(+), 9 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
  - a739922 add nvidia report (#285) (fmoral2, 2025-10-27)

### assert helpers  (1 files)

- `pkg/assert/validate.go`  →  `internal/pkg/assert/validate.go`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - 089ad80 fix k3s-worker-tf (#278) (fmoral2, 2025-10-03)

### aws helpers  (1 files)

- `pkg/aws/ec2.go`  →  `internal/pkg/aws/ec2.go`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - 089ad80 fix k3s-worker-tf (#278) (fmoral2, 2025-10-03)

### shared/resources  (6 files)

- `shared/aux.go`  →  `internal/resources/aux.go`  ⚠ MISSING
  - 1 file changed, 45 insertions(+), 2 deletions(-)
  - 92feab7 Fix.killall rke2 (#326) (fmoral2, 2026-05-07)
- `shared/cluster.go`  →  `internal/resources/cluster.go`  ⚠ MISSING
  - 1 file changed, 83 insertions(+), 10 deletions(-)
  - 089ad80 fix k3s-worker-tf (#278) (fmoral2, 2025-10-03)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 71033de Top command (#329) (ERYN Tennis, 2026-05-21)
- `shared/clusterconfig.go`  →  `internal/resources/clusterconfig.go`  ⚠ MISSING
  - 1 file changed, 4 insertions(+), 2 deletions(-)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `shared/report.go`  →  `internal/resources/report.go`  ⚠ MISSING
  - 1 file changed, 19 insertions(+), 10 deletions(-)
  - a739922 add nvidia report (#285) (fmoral2, 2025-10-27)
  - 92feab7 Fix.killall rke2 (#326) (fmoral2, 2026-05-07)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `shared/ssh.go`  →  `internal/resources/ssh.go`  ✓
  - 1 file changed, 4 insertions(+), 4 deletions(-)
  - 089ad80 fix k3s-worker-tf (#278) (fmoral2, 2025-10-03)
- `shared/terraform.go`  →  `internal/resources/terraform.go`  ⚠ MISSING
  - 1 file changed, 27 insertions(+), 46 deletions(-)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)

### cmd binaries  (1 files)

- `cmd/qase/main.go`  →  `cmd/qase/main.go`  ✓
  - 1 file changed, 38 insertions(+), 6 deletions(-)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)

### entrypoint suites  (18 files)

- `entrypoint/airgap/airgap_suite_test.go`  →  `entrypoint/airgap/airgap_suite_test.go`  ✓
  - 1 file changed, 11 insertions(+), 13 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `entrypoint/airgap/tarball_test.go`  →  `entrypoint/airgap/tarball_test.go`  ✓
  - 1 file changed, 8 insertions(+)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `entrypoint/dualstack/dualstack_test.go`  →  `entrypoint/dualstack/dualstack_test.go`  ✓
  - 1 file changed, 3 insertions(+)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `entrypoint/ipv6only/ipv6only_suite_test.go`  →  `entrypoint/ipv6only/ipv6only_suite_test.go`  ✓
  - 1 file changed, 6 insertions(+)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
- `entrypoint/nvidia/nvidia_suite_test.go`  →  `entrypoint/nvidia/nvidia_suite_test.go`  ✓
  - 1 file changed, 11 insertions(+), 3 deletions(-)
  - a739922 add nvidia report (#285) (fmoral2, 2025-10-27)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
- `entrypoint/nvidia/nvidia_test.go`  →  `entrypoint/nvidia/nvidia_test.go`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - a739922 add nvidia report (#285) (fmoral2, 2025-10-27)
- `entrypoint/rebootinstances/rebootinstances_test.go`  →  `entrypoint/rebootinstances/rebootinstances_test.go`  ✓
  - 1 file changed, 8 insertions(+)
  - 71033de Top command (#329) (ERYN Tennis, 2026-05-21)
- `entrypoint/restartservice/restartservice_test.go`  →  `entrypoint/restartservice/restartservice_test.go`  ✓
  - 1 file changed, 8 insertions(+)
  - 71033de Top command (#329) (ERYN Tennis, 2026-05-21)
- `entrypoint/upgradecluster/upgradesuc_test.go`  →  `entrypoint/upgradecluster/upgradesuc_test.go`  ✓
  - 1 file changed, 11 insertions(+), 2 deletions(-)
  - 92728b7 Suc plan change and node status timeout update (#327) (Archana Ganesh, 2026-05-06)
  - 71033de Top command (#329) (ERYN Tennis, 2026-05-21)
- `entrypoint/validatecluster/validatecluster_suite_test.go`  →  `entrypoint/validatecluster/validatecluster_suite_test.go`  ✓
  - 1 file changed, 2 insertions(+), 2 deletions(-)
  - b2a5a58 adding selinux test to validate cluster test (#315) (ERYN Tennis, 2026-04-13)
- `entrypoint/validatecluster/validatecluster_test.go`  →  `entrypoint/validatecluster/validatecluster_test.go`  ✓
  - 1 file changed, 8 insertions(+)
  - 71033de Top command (#329) (ERYN Tennis, 2026-05-21)
- `entrypoint/versionbump/cnicalico_test.go`  →  `entrypoint/versionbump/cnicalico_test.go`  ✓
  - 1 file changed, 20 insertions(+), 2 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `entrypoint/versionbump/cnicanal_test.go`  →  `entrypoint/versionbump/cnicanal_test.go`  ✓
  - 1 file changed, 21 insertions(+), 3 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `entrypoint/versionbump/cnicilium_test.go`  →  `entrypoint/versionbump/cnicilium_test.go`  ✓
  - 1 file changed, 21 insertions(+), 3 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `entrypoint/versionbump/cniflannel_test.go`  →  `entrypoint/versionbump/cniflannel_test.go`  ✓
  - 1 file changed, 22 insertions(+), 1 deletion(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `entrypoint/versionbump/cnimultuscanal_test.go`  →  `entrypoint/versionbump/cnimultuscanal_test.go`  ✓
  - 1 file changed, 20 insertions(+), 1 deletion(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `entrypoint/versionbump/components_test.go`  →  `entrypoint/versionbump/components_test.go`  ✓
  - 1 file changed, 48 insertions(+), 10 deletions(-)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `entrypoint/versionbump/versionbump_suite_test.go`  →  `entrypoint/versionbump/versionbump_suite_test.go`  ✓
  - 1 file changed, 2 insertions(+)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)

### terraform modules  (20 files)

- `modules/airgap/instance/instance_server.tf`  →  `modules/airgap/instance/instance_server.tf`  ⚠ MISSING
  - 1 file changed, 3 insertions(+)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
- `modules/airgap/setup/bastion_prepare.sh`  →  `modules/airgap/setup/bastion_prepare.sh`  ⚠ MISSING
  - 1 file changed, 68 insertions(+), 7 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `modules/airgap/setup/get_artifacts.sh`  →  `modules/airgap/setup/get_artifacts.sh`  ⚠ MISSING
  - 1 file changed, 185 insertions(+), 47 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `modules/airgap/setup/install_product.sh`  →  `modules/airgap/setup/install_product.sh`  ⚠ MISSING
  - 1 file changed, 10 insertions(+), 8 deletions(-)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `modules/airgap/setup/podman_cmds.sh`  →  `modules/airgap/setup/podman_cmds.sh`  ⚠ MISSING
  - 1 file changed, 54 insertions(+), 33 deletions(-)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `modules/airgap/setup/private_registry.sh`  →  `modules/airgap/setup/private_registry.sh`  ⚠ MISSING
  - 1 file changed, 8 insertions(+), 1 deletion(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `modules/airgap/setup/system_default_registry.sh`  →  `modules/airgap/setup/system_default_registry.sh`  ⚠ MISSING
  - 1 file changed, 1 insertion(+)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `modules/airgap/setup/windows_install.ps1`  →  `modules/airgap/setup/windows_install.ps1`  ⚠ MISSING
  - 1 file changed, 4 insertions(+), 2 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
- `modules/install/join_k3s_agent.sh`  →  `modules/install/join_k3s_agent.sh`  ⚠ MISSING
  - 1 file changed, 83 insertions(+), 10 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `modules/install/join_k3s_master.sh`  →  `modules/install/join_k3s_master.sh`  ⚠ MISSING
  - 1 file changed, 84 insertions(+), 12 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `modules/install/join_rke2_agent.sh`  →  `modules/install/join_rke2_agent.sh`  ⚠ MISSING
  - 1 file changed, 114 insertions(+), 27 deletions(-)
  - 113b6ab fix gomod job (#279) (fmoral2, 2025-10-15)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 90776b4 fix cis-ubuntu and suc job timeout (#300) (fmoral2, 2026-01-09)
- `modules/install/join_rke2_master.sh`  →  `modules/install/join_rke2_master.sh`  ⚠ MISSING
  - 1 file changed, 127 insertions(+), 38 deletions(-)
  - 113b6ab fix gomod job (#279) (fmoral2, 2025-10-15)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 90776b4 fix cis-ubuntu and suc job timeout (#300) (fmoral2, 2026-01-09)
- `modules/install/k3s_master.sh`  →  `modules/install/k3s_master.sh`  ⚠ MISSING
  - 1 file changed, 86 insertions(+), 12 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `modules/install/rke2_master.sh`  →  `modules/install/rke2_master.sh`  ⚠ MISSING
  - 1 file changed, 128 insertions(+), 40 deletions(-)
  - 113b6ab fix gomod job (#279) (fmoral2, 2025-10-15)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 90776b4 fix cis-ubuntu and suc job timeout (#300) (fmoral2, 2026-01-09)
- `modules/ipv6only/scripts/configure.sh`  →  `modules/ipv6only/scripts/configure.sh`  ⚠ MISSING
  - 1 file changed, 26 deletions(-)
  - de99d67 fix airgap tarball, ipv6 code (#301) (mdrahman-suse, 2026-01-21)
- `modules/ipv6only/scripts/prepare.sh`  →  `modules/ipv6only/scripts/prepare.sh`  ⚠ MISSING
  - 1 file changed, 6 insertions(+), 1 deletion(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `modules/k3s/master/cis_master_config.yaml`  →  `modules/k3s/master/cis_master_config.yaml`  ⚠ MISSING
  - 1 file changed, 5 insertions(+), 7 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
- `modules/k3s/master/instances_server.tf`  →  `modules/k3s/master/instances_server.tf`  ⚠ MISSING
  - 1 file changed, 4 insertions(+), 4 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
- `modules/k3s/worker/cis_worker_config.yaml`  →  `modules/k3s/worker/cis_worker_config.yaml`  ⚠ MISSING
  - 1 file changed, 2 insertions(+), 1 deletion(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
- `modules/k3s/worker/instances_worker.tf`  →  `modules/k3s/worker/instances_worker.tf`  ⚠ MISSING
  - 1 file changed, 44 insertions(+), 55 deletions(-)
  - 089ad80 fix k3s-worker-tf (#278) (fmoral2, 2025-10-03)

### workloads  (8 files)

- `workloads/amd64/nvidia-benchmark.yaml`  →  `workloads/amd64/nvidia-benchmark.yaml`  ✓
  - 1 file changed, 1 deletion(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
- `workloads/amd64/nvidia-operator.yaml`  →  `workloads/amd64/nvidia-operator.yaml`  ✓
  - 1 file changed, 4 insertions(+), 1 deletion(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `workloads/amd64/rke2-suc-plan-splitroles.yaml`  →  `workloads/amd64/rke2-suc-plan-splitroles.yaml`  ✓
  - 1 file changed, 12 insertions(+), 1 deletion(-)
  - 92728b7 Suc plan change and node status timeout update (#327) (Archana Ganesh, 2026-05-06)
- `workloads/amd64/rke2-suc-plan.yaml`  →  `workloads/amd64/rke2-suc-plan.yaml`  ✓
  - 1 file changed, 9 insertions(+), 1 deletion(-)
  - 92728b7 Suc plan change and node status timeout update (#327) (Archana Ganesh, 2026-05-06)
- `workloads/amd64/suc.yaml`  →  `workloads/amd64/suc.yaml`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - 90776b4 fix cis-ubuntu and suc job timeout (#300) (fmoral2, 2026-01-09)
- `workloads/arm/rke2-suc-plan-splitroles.yaml`  →  `workloads/arm/rke2-suc-plan-splitroles.yaml`  ✓
  - 1 file changed, 11 insertions(+)
  - 92728b7 Suc plan change and node status timeout update (#327) (Archana Ganesh, 2026-05-06)
- `workloads/arm/rke2-suc-plan.yaml`  →  `workloads/arm/rke2-suc-plan.yaml`  ✓
  - 1 file changed, 11 insertions(+)
  - 92728b7 Suc plan change and node status timeout update (#327) (Archana Ganesh, 2026-05-06)
- `workloads/arm/suc.yaml`  →  `workloads/arm/suc.yaml`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - 90776b4 fix cis-ubuntu and suc job timeout (#300) (fmoral2, 2026-01-09)

### scripts / CI  (17 files)

- `scripts/Dockerfile.build`  →  `scripts/Dockerfile.build`  ✓
  - 1 file changed, 42 insertions(+), 19 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `scripts/Dockerfile.jenkins`  →  `scripts/Dockerfile.jenkins`  ✓
  - 1 file changed, 27 insertions(+), 17 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `scripts/Jenkinsfile`  →  `scripts/Jenkinsfile`  ✓
  - 1 file changed, 27 insertions(+), 6 deletions(-)
  - ee795d9 Add.batch os (#316) (fmoral2, 2026-03-13)
  - cd46233 fix replace tfvars lock (#317) (fmoral2, 2026-03-16)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/Jenkinsfile_batch_airgap_test`  →  `scripts/Jenkinsfile_batch_airgap_test`  ✓
  - 1 file changed, 35 insertions(+), 37 deletions(-)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `scripts/Jenkinsfile_batch_install_test`  →  `scripts/Jenkinsfile_batch_install_test`  ✓
  - 1 file changed, 38 insertions(+), 49 deletions(-)
  - fe72404 Rpm test (#311) (ERYN Tennis, 2026-02-17)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
  - 92feab7 Fix.killall rke2 (#326) (fmoral2, 2026-05-07)
- `scripts/Jenkinsfile_batch_upgrade_test`  →  `scripts/Jenkinsfile_batch_upgrade_test`  ✓
  - 1 file changed, 18 insertions(+), 24 deletions(-)
  - fe72404 Rpm test (#311) (ERYN Tennis, 2026-02-17)
  - 2b9f5b2 Feat/airgap update (#310) (Md Mahbubur Rahman, 2026-05-21)
- `scripts/Jenkinsfile_delete_resources`  →  `scripts/Jenkinsfile_delete_resources`  ✓
  - 1 file changed, 5 insertions(+), 3 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/build.sh`  →  `scripts/build.sh`  ✓
  - 1 file changed, 1 insertion(+), 2 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/configure.sh`  →  `scripts/configure.sh`  ✓
  - 1 file changed, 3 insertions(+), 2 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/docker_run.sh`  →  `scripts/docker_run.sh`  ✓
  - 1 file changed, 15 insertions(+), 9 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/e2e_report.sh`  →  `scripts/e2e_report.sh`  ✓
  - 1 file changed, 2 insertions(+), 2 deletions(-)
  - 5efb75e update reporter  (#309) (fmoral2, 2026-03-02)
- `scripts/install_sonobuoy.sh`  →  `scripts/install_sonobuoy.sh`  ✓
  - 1 file changed, 20 insertions(+), 7 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/kill-all_test.sh`  →  `scripts/kill-all_test.sh`  ✓
  - 1 file changed, 11 insertions(+), 13 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)
  - 92feab7 Fix.killall rke2 (#326) (fmoral2, 2026-05-07)
- `scripts/qase-patch-validation.sh`  →  `scripts/qase-patch-validation.sh`  ✓
  - 1 file changed, 7 insertions(+), 9 deletions(-)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `scripts/release_checks.sh`  →  `scripts/release_checks.sh`  ✓
  - 1 file changed, 32 insertions(+), 9 deletions(-)
  - d6575ef add prime registry tests for k3s and rke2 upgrade images in release check captain script (#277) (Archana Ganesh, 2025-09-23)
  - 9699151 update release check script (#292) (fmoral2, 2025-11-24)
  - d715390 fix asset count (#314) (Archana Ganesh, 2026-02-19)
- `scripts/test_runner.sh`  →  `scripts/test_runner.sh`  ✓
  - 1 file changed, 2 insertions(+)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `scripts/uninstall_test.sh`  →  `scripts/uninstall_test.sh`  ✓
  - 1 file changed, 17 insertions(+)
  - 92feab7 Fix.killall rke2 (#326) (fmoral2, 2026-05-07)

### .github workflows  (5 files)

- `.github/workflows/go-mod-change.yaml`  →  `.github/workflows/go-mod-change.yaml`  ✓
  - 1 file changed, 10 insertions(+), 7 deletions(-)
  - 113b6ab fix gomod job (#279) (fmoral2, 2025-10-15)
  - 7ef93af Pin GH Actions to commit sha (#321) (Chris Wayne, 2026-03-27)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `.github/workflows/qase-e2e-report.yaml`  →  `.github/workflows/qase-e2e-report.yaml`  ✓
  - 1 file changed, 4 insertions(+), 4 deletions(-)
  - 7ef93af Pin GH Actions to commit sha (#321) (Chris Wayne, 2026-03-27)
- `.github/workflows/qase-patch-validation-create.yaml`  →  `.github/workflows/qase-patch-validation-create.yaml`  ✓
  - 1 file changed, 1 insertion(+), 1 deletion(-)
  - 7ef93af Pin GH Actions to commit sha (#321) (Chris Wayne, 2026-03-27)
- `.github/workflows/release-checks.yaml`  →  `.github/workflows/release-checks.yaml`  ✓
  - 1 file changed, 7 insertions(+), 4 deletions(-)
  - 6194a18 fix captain and add post release jenkins file (#273) (fmoral2, 2025-09-08)
  - 7ef93af Pin GH Actions to commit sha (#321) (Chris Wayne, 2026-03-27)
  - 391f0ab Sec.fixes (#322) (fmoral2, 2026-04-14)
- `.github/workflows/run-distros.yaml`  →  `.github/workflows/run-distros.yaml`  ✓
  - 1 file changed, 4 insertions(+), 4 deletions(-)
  - 7ef93af Pin GH Actions to commit sha (#321) (Chris Wayne, 2026-03-27)

### docs / makefile  (2 files)

- `Makefile`  →  `Makefile`  ✓
  - 1 file changed, 3 insertions(+)
  - 6fa2799 adding charts versions test (#274) (ERYN Tennis, 2025-10-02)
- `docs/nvidia.md`  →  `docs/nvidia.md`  ✓
  - 1 file changed, 40 insertions(+), 3 deletions(-)
  - 8645f82 add nvidia report (#286) (fmoral2, 2025-12-10)

---

## 3. Files we have here but not on main (our distinctive work — preserve)

### `docs/qa-infra-integration.md/`  (1 files)
- `docs/qa-infra-integration.md`

### `infrastructure/qainfra/`  (2 files)
- `infrastructure/qainfra/main.tf`
- `infrastructure/qainfra/variables.tf`

### `internal/pkg/`  (6 files)
- `internal/pkg/testcase/cluster.go`
- `internal/pkg/testcase/ipv6only.go`
- `internal/pkg/testcase/privateregistry.go`
- `internal/pkg/testcase/support/aws.go`
- `internal/pkg/testcase/systemdefaultregistry.go`
- `internal/pkg/testcase/tarball.go`

### `internal/provisioning/`  (14 files)
- `internal/provisioning/driver/cluster.go`
- `internal/provisioning/driver/config.go`
- `internal/provisioning/driver/interfaces.go`
- `internal/provisioning/legacy/config.go`
- `internal/provisioning/legacy/legacy.go`
- `internal/provisioning/legacy/provisioner.go`
- `internal/provisioning/legacy/terraform.go`
- `internal/provisioning/provisioning.go`
- `internal/provisioning/qainfra/ansible.go`
- `internal/provisioning/qainfra/config.go`
- … and 4 more

### `internal/resources/`  (14 files)
- `internal/resources/basepath.go`
- `internal/resources/clusteroperations.go`
- `internal/resources/command.go`
- `internal/resources/file.go`
- `internal/resources/helm.go`
- `internal/resources/logger.go`
- `internal/resources/node.go`
- `internal/resources/nodeenv.go`
- `internal/resources/pod.go`
- `internal/resources/process.go`
- … and 4 more

### `scripts/Jenkinsfile_post_release_captain/`  (1 files)
- `scripts/Jenkinsfile_post_release_captain`

