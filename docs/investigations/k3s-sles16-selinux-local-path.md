# K3s on SLES 16: SELinux and local-path investigation packet

Status: draft evidence validated against the current upstream `k3s-io/k3s`,
`rancher/local-path-provisioner`, and `k3s-io/k3s-selinux` sources.

Collected on 2026-08-12 from an intentionally preserved Jenkins cluster. All
commands below are read-only unless explicitly marked as a diagnostic SELinux
mode change.

## Executive summary

The run exposed two separate problems that should not be reported as one root
cause:

1. **Initial volume creation is a qa-infra installation gap.** The qa-infra
   direct-binary K3s role installs `k3s-selinux` only when Ansible reports the
   Red Hat OS family. On SLES 16 the task is skipped, the K3s policy is absent,
   `/var/lib/rancher/k3s/storage` remains `var_lib_t`, and the local-path create
   helper is denied by SELinux.
2. **PV teardown after the policy is installed is a confirmed, previously
   reported K3s/local-path behavior.** A fresh helper pod receives different
   MCS categories from the volume directory. Deletion repeatedly fails while
   enforcing and succeeds on the next retry while permissive. The kernel
   records the category mismatch. The same root cause was confirmed upstream
   in `k3s-io/k3s#10130`; its original default fix was later reverted from the
   provisioner.

The first item belongs in qa-infra. The second should be reported to K3s with
the prior issues and revert linked, because the current K3s manifest still
ships the affected default helper configuration.

## Environment

- OS: SUSE Linux Enterprise Server 16.0
- Kernel: `6.12.0-160000.5-default` (`amd64`)
- SELinux: targeted policy, enforcing, MLS enabled
- Initial K3s: `v1.36.2+k3s1`
- Upgraded K3s: `v1.36.3+k3s1`
- containerd: `2.3.2-k3s2`
- Topology: three server nodes plus one worker
- K3s config: `selinux: true`
- Jenkins test: manual upgrade with infrastructure preserved
- Internal build: [mower k3s_manual_upgrade_qainfra #7](https://mower.jenkins.qa.rancher.space/job/distros_qa/job/k3s-tests/job/k3s_manual_upgrade_qainfra/7/)

Relevant test arguments:

```text
-destroy false -tags=upgrademanual \
  -installVersionOrCommit v1.36.3+k3s1 -channel latest
```

Final result:

```text
Ran 24 of 24 Specs in 786.284 seconds
FAIL! -- 23 Passed | 1 Failed | 0 Pending | 0 Skipped

[FAIL] Verifies Local Path Provisioner storage pre-upgrade
```

The manual K3s upgrade itself passed, all four nodes reached
`v1.36.3+k3s1`, and the local-path functional check after the upgrade passed.

## Evidence A: initial create failure without k3s-selinux

Before the manual upgrade, the node had no `k3s-selinux` RPM and no loaded
`k3s` SELinux module. The storage parent was labeled `var_lib_t`.

The local-path helper failed with:

```text
mkdir: can't create directory
'/var/lib/rancher/k3s/storage/pvc-844e81b8-034a-490f-a50c-d21c7ae934c9_local-path-storage_local-path-pvc':
Permission denied
```

The corresponding enforcing AVCs were:

```text
avc: denied { write } for pid=6995 comm="mkdir" name="storage"
dev="nvme0n1p3" ino=44486
scontext=system_u:system_r:container_t:s0:c354,c613
tcontext=system_u:object_r:var_lib_t:s0 tclass=dir permissive=0

avc: denied { write } for pid=7612 comm="mkdir" name="storage"
scontext=system_u:system_r:container_t:s0:c212,c440
tcontext=system_u:object_r:var_lib_t:s0 tclass=dir permissive=0

avc: denied { write } for pid=8573 comm="mkdir" name="storage"
scontext=system_u:system_r:container_t:s0:c30,c173
tcontext=system_u:object_r:var_lib_t:s0 tclass=dir permissive=0
```

The PVC stayed Pending and the provisioner reported:

```text
failed to provision volume with StorageClass "local-path":
failed to create volume ...: create process timeout after 120 seconds
```

The qa-infra role explains why the policy was absent. Its install task is
guarded by:

```yaml
when:
  - k3s_install_selinux_policy | default(true) | bool
  - ansible_facts.os_family | default('') == "RedHat"
```

This is not evidence that SLES 16 lacks a K3s SELinux package. It means the
qa-infra direct-binary path does not install the SUSE package.

## Evidence B: official installer adds a working SLES package

During the manual upgrade, `get.k3s.io` configured the Rancher K3s repository
and installed the following package at `17:41:00 UTC`:

```text
k3s-selinux-1.6-1.slemicro.noarch
Source RPM: k3s-selinux-1.6-1.slemicro.src.rpm
Build date: 2024-09-16
Repository alias: rancher-k3s-common-latest
```

After installation:

```text
$ semodule -l | grep '^k3s'
k3s

$ ls -Zd /var/lib/rancher/k3s/storage
system_u:object_r:container_file_t:s0 /var/lib/rancher/k3s/storage
```

A separate PVC/pod probe on another enforcing node became Bound/Ready in
approximately seven seconds. This demonstrates that immediate creation works
once the package is installed and the path is relabeled; it does not support a
required 10-15 minute first-boot delay.

The post-upgrade DTF local-path check also passed:

```text
PASSED! Test: Verifies Local Path Provisioner storage after upgrade
```

## Evidence C: delete helper fails because MCS categories differ

The packaged K3s local-path helper template has no `securityContext` or
explicit SELinux options:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: helper-pod
spec:
  containers:
    - name: helper-pod
      image: rancher/mirrored-library-busybox:1.37.0
      imagePullPolicy: IfNotPresent
```

The scripts are:

```sh
# setup
set -eu
mkdir -m 0777 -p "${VOL_DIR}"
chmod 700 "${VOL_DIR}/.."

# teardown
set -eu
rm -rf "${VOL_DIR}"
```

For the controlled probe, the volume directory was labeled:

```text
system_u:object_r:container_file_t:s0:c432,c816
```

While enforcing, the delete helper exited with status 1 and the PV remained
`Released`. The controller repeatedly reported:

```text
VolumeFailedDelete: failed to delete volume ...:
create process timeout after 120 seconds
```

SELinux was then changed to permissive temporarily. On the next retry, the
kernel logged the operations that enforcing had blocked/suppressed:

```text
avc: denied { write } for pid=15966 comm="rm"
name="pvc-434e5d46-6b45-4f45-ae6e-4a1f76f4705c_selinux-probe_probe-pvc"
scontext=system_u:system_r:container_t:s0:c471,c511
tcontext=system_u:object_r:container_file_t:s0:c432,c816
tclass=dir permissive=1

avc: denied { remove_name } for pid=15966 comm="rm" name="probe"
scontext=system_u:system_r:container_t:s0:c471,c511
tcontext=system_u:object_r:container_file_t:s0:c432,c816
tclass=dir permissive=1

avc: denied { unlink } for pid=15966 comm="rm" name="probe"
scontext=system_u:system_r:container_t:s0:c471,c511
tcontext=system_u:object_r:container_file_t:s0:c432,c816
tclass=file permissive=1
```

The PV and directory disappeared on that retry. SELinux was immediately
restored to enforcing. The differing category pairs are the key observation:

```text
delete helper: c471,c511
volume target: c432,c816
```

This confirms an MCS mismatch. Current upstream history shows that the intended
configuration point is the local-path helper pod template; see the upstream
source and history analysis below.

The original test PV remains a second example:

```text
persistentvolume/pvc-844e81b8-034a-490f-a50c-d21c7ae934c9
STATUS=Released

helper-pod-delete-pvc-844e81b8-034a-490f-a50c-d21c7ae934c9
STATUS=Error

VolumeFailedDelete ... create process timeout after 120 seconds
```

## Validation against current upstream sources

Validation was performed on 2026-08-12 against K3s `main` commit
[`866d743e56ecec4a7a9860a22f9930afa39ff722`](https://github.com/k3s-io/k3s/commit/866d743e56ecec4a7a9860a22f9930afa39ff722).

### K3s still ships the affected helper configuration

The current K3s
[`manifests/local-storage.yaml`](https://github.com/k3s-io/k3s/blob/866d743e56ecec4a7a9860a22f9930afa39ff722/manifests/local-storage.yaml#L67-L133)
ships:

```yaml
image: rancher/local-path-provisioner:v0.0.36

teardown: |-
  #!/bin/sh
  set -eu
  rm -rf "${VOL_DIR}"

helperPod.yaml: |-
  apiVersion: v1
  kind: Pod
  metadata:
    name: helper-pod
  spec:
    containers:
    - name: helper-pod
      image: rancher/mirrored-library-busybox:1.37.0
      imagePullPolicy: IfNotPresent
```

This matches the live ConfigMap collected from the reproduced cluster. There
is no pod/container `securityContext` or `seLinuxOptions` in the K3s default.

The K3s
[`updatecli` policy](https://github.com/k3s-io/k3s/blob/866d743e56ecec4a7a9860a22f9930afa39ff722/updatecli/updatecli.d/local-path-provisioner.yaml)
follows the latest release of `rancher/local-path-provisioner`; this is not an
old manifest accidentally pinning a pre-fix release.

### What local-path-provisioner v0.0.36 does

In
[`provisioner.go`](https://github.com/rancher/local-path-provisioner/blob/v0.0.36/provisioner.go#L509-L769),
both create and delete call `createHelperPod`. That function:

1. mounts the volume's parent directory into a newly created helper pod using
   a `hostPath` volume;
2. deep-copies the configured helper template;
3. assigns a unique create/delete helper name;
4. schedules it onto the volume's node;
5. runs `/script/setup` or `/script/teardown`; and
6. waits up to the default 120 seconds for `PodSucceeded`.

There is no code that reads the volume directory's SELinux range or reuses it
for the later delete helper. Kubernetes therefore assigns each helper a fresh
MCS pair. This is consistent with the collected source/target category pairs.

### The exact MCS bug was previously confirmed upstream

[`k3s-io/k3s#10130`](https://github.com/k3s-io/k3s/issues/10130)
reported the same symptoms:

- the PV remains `Released`;
- `helper-pod-delete-*` fails;
- permissive mode allows deletion; and
- the delete helper has different MCS categories from the target directory.

The upstream diagnosis explicitly identified the MCS mismatch. The original
fix,
[`rancher/local-path-provisioner#402`](https://github.com/rancher/local-path-provisioner/pull/402),
forced the helper container to use the full category range:

```go
helperPod.Spec.Containers[0].SecurityContext = &v1.SecurityContext{
    SELinuxOptions: &v1.SELinuxOptions{
        Level: "s0-s0:c0.c1023",
    },
}
```

That fix shipped in local-path-provisioner `v0.0.27`, but was reverted by
[`#421`](https://github.com/rancher/local-path-provisioner/pull/421). The
reason for the revert was sound: hard-coding the value in Go overwrote a
user-provided helper template. The maintainers' selected configuration point
was `helperPod.yaml`, not removal of the MCS mitigation itself.

The behavior was reported again in:

- [`rancher/local-path-provisioner#460`](https://github.com/rancher/local-path-provisioner/issues/460),
  where the ConfigMap workaround was confirmed after restarting the
  provisioner;
- [`rancher/local-path-provisioner#484`](https://github.com/rancher/local-path-provisioner/issues/484),
  with the same additional range labels and failed cleanup; it was closed by
  the stale bot, not by a code fix; and
- [`k3s-io/k3s-selinux#52`](https://github.com/k3s-io/k3s-selinux/issues/52),
  which remains open and describes the same cross-MCS access class.

Consequently, `v0.0.36` does not contain an automatic replacement for the
reverted fix, and K3s does not supply the mitigation in its helper template.

### The supported helper-template mitigation

The `v0.0.36` template validator allows SELinux options while rejecting unsafe
settings such as `privileged: true`, added capabilities, or explicit privilege
escalation. See
[`util.go`](https://github.com/rancher/local-path-provisioner/blob/v0.0.36/util.go#L47-L120).

This means the relevant mitigation can be expressed without
`--allow-unsafe-helper-pod-template`:

```yaml
helperPod.yaml: |-
  apiVersion: v1
  kind: Pod
  metadata:
    name: helper-pod
  spec:
    containers:
    - name: helper-pod
      image: rancher/mirrored-library-busybox:1.37.0
      imagePullPolicy: IfNotPresent
      securityContext:
        seLinuxOptions:
          level: s0-s0:c0.c1023
```

The current K3s deployment does not pass `CONFIG_MOUNT_PATH`, so the
provisioner does not hot-reload the helper template. After changing the
ConfigMap on an existing cluster, restart the deployment:

```sh
kubectl -n kube-system rollout restart deployment/local-path-provisioner
kubectl -n kube-system rollout status deployment/local-path-provisioner
```

**Mitigation validated on the preserved cluster (2026-08-12, SELinux
enforcing throughout):**

1. The original PV (`pvc-844e81b8`), stuck `Released` with `VolumeFailedDelete`
   retries for 37+ minutes, was deleted **within 15 seconds** of patching the
   ConfigMap and restarting the provisioner — the next retry's delete helper
   ran with `level: s0-s0:c0.c1023` and succeeded.
2. A fresh full-lifecycle probe: PVC Bound + pod Running in 20 s; after
   namespace deletion the PV object was gone in under 10 s, with
   `getenforce` reporting `Enforcing` before and after.

The permanent K3s-side proposal is to ship the SELinux level in the packaged
default helper template, rather than hard-code it in the provisioner.

### k3s-selinux master has no MCS-specific fix

The current `k3s-io/k3s-selinux` master is
[`20ad0cde0c94ba13d2d257cc27a932cb8d2bac12`](https://github.com/k3s-io/k3s-selinux/commit/20ad0cde0c94ba13d2d257cc27a932cb8d2bac12).

The SLE Micro policy labels the K3s storage path as `container_file_t`:

```text
/var/lib/rancher/k3s/storage(/.*)?
    gen_context(system_u:object_r:container_file_t,s0)
```

See
[`policy/slemicro/k3s.fc`](https://github.com/k3s-io/k3s-selinux/blob/20ad0cde0c94ba13d2d257cc27a932cb8d2bac12/policy/slemicro/k3s.fc#L25)
and
[`k3s.te`](https://github.com/k3s-io/k3s-selinux/blob/20ad0cde0c94ba13d2d257cc27a932cb8d2bac12/policy/slemicro/k3s.te#L35).

The SLE Micro `k3s.fc` and `k3s.te` blob SHAs are identical between the 1.6
source line used by the reproduced node and current master:

```text
k3s.fc  780d324a767beea1f3e279149919b70310cfdcaa
k3s.te  a242321fb2733815da7132f77126e878b5db371e
```

Therefore, there is no newer MCS-specific policy correction on master that
would make this reproduction obsolete.

### The official K3s installer already handles SLES 16 separately

Current
[`install.sh`](https://github.com/k3s-io/k3s/blob/866d743e56ecec4a7a9860a22f9930afa39ff722/install.sh#L584-L668)
routes SLES 16 to the `slemicro` RPM repository and installs
`k3s-selinux` before starting K3s. This was fixed by
[`k3s-io/k3s#14472`](https://github.com/k3s-io/k3s/pull/14472), tracked in
[`#14491`](https://github.com/k3s-io/k3s/issues/14491).

This reinforces the separation of concerns:

- the official K3s path handles SLES 16 and installs the policy;
- qa-infra's direct-binary role skips policy installation because its task is
  restricted to the Red Hat OS family; and
- even with a correctly installed policy, the default local-path delete helper
  remains vulnerable to the cross-MCS teardown problem.

### Current K3s tests do not detect the leak

The K3s E2E local-path test creates a PVC, writes data, recreates the workload,
and verifies that the data persists. It does not delete the PVC or wait for PV
cleanup. See
[`validatecluster_test.go`](https://github.com/k3s-io/k3s/blob/866d743e56ecec4a7a9860a22f9930afa39ff722/tests/e2e/validatecluster/validatecluster_test.go#L199-L252).

The local-storage integration test does issue `kubectl delete pvc`, but only
checks that the API command returns the word `deleted`. It does not wait for:

- the PV object to disappear;
- the delete helper to succeed;
- `VolumeFailedDelete` to remain absent; or
- the backing directory to be removed.

See
[`localstorage_int_test.go`](https://github.com/k3s-io/k3s/blob/866d743e56ecec4a7a9860a22f9930afa39ff722/tests/integration/localstorage/localstorage_int_test.go#L90-L95).

The integration test also does not establish a real SLES 16 enforcing/MCS
environment. This explains how both K3s and DTF tests can report green while
leaving an asynchronous PV cleanup failure behind.

## Important interpretation of older green builds

A green DTF local-path test proves that the pod reached Running and data could
be written/read. It does **not** prove asynchronous PV teardown succeeded.
`TestLocalPathProvisionerStorage` only verifies that `kubectl delete` accepted
the manifest deletion; it does not wait for the PV or backing directory to be
removed.

Therefore, a legacy build such as baler `k3s_manual_upgrade #85` can establish
that `k3s-selinux-1.6-1.sle` allowed volume creation, but its green status alone
cannot disprove the MCS delete problem.

The current qainfra run resolved a different variant and channel:

```text
legacy #85:  k3s-selinux-1.6-1.sle       (stable repository, per log review)
qainfra #7:  k3s-selinux-1.6-1.slemicro  (latest repository)
```

Do not claim that a recently repaired `1.7-x` package was used in qainfra #7;
the preserved node proves it used the 2024-built `1.6-1.slemicro` package.

## Minimal reproduction

Prerequisites:

- SLES 16 node(s)
- K3s installed with `selinux: true`
- `k3s-selinux` installed and `semodule -l` showing `k3s`
- SELinux enforcing

Apply:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: selinux-probe
  labels:
    pod-security.kubernetes.io/enforce: privileged
    pod-security.kubernetes.io/audit: privileged
    pod-security.kubernetes.io/warn: privileged
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: probe-pvc
  namespace: selinux-probe
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: local-path
  resources:
    requests:
      storage: 16Mi
---
apiVersion: v1
kind: Pod
metadata:
  name: probe
  namespace: selinux-probe
spec:
  containers:
    - name: probe
      image: rancher/mirrored-library-busybox:1.37.0
      command: ["sh", "-c", "echo probe >/data/probe; sleep 3600"]
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: probe-pvc
```

Then run:

```sh
kubectl apply -f selinux-probe.yaml
kubectl -n selinux-probe wait --for=condition=Ready pod/probe --timeout=120s
kubectl -n selinux-probe get pod,pvc -o wide

PV_NAME=$(kubectl -n selinux-probe get pvc probe-pvc \
  -o jsonpath='{.spec.volumeName}')
VOLUME_PATH=$(kubectl get pv "$PV_NAME" \
  -o jsonpath='{.spec.local.path}')
echo "$PV_NAME $VOLUME_PATH"

kubectl delete namespace selinux-probe
kubectl get pv "$PV_NAME" -w
```

Expected: the PV and backing directory are deleted.

Observed: the PV stays `Released`, a `helper-pod-delete-*` pod exits with status
1, and `VolumeFailedDelete` repeats every approximately 120 seconds.

## Diagnostic commands used

Use placeholders; never paste a private key or Jenkins credential into an
issue.

```sh
export SSH_KEY_FILE='<path-to-private-key>'
export NODE='<node-address>'
export MASTER='<server-address>'
```

OS, SELinux and package state:

```sh
ssh -i "$SSH_KEY_FILE" "ec2-user@$NODE" 'sudo sh -c '\''
  cat /etc/os-release
  uname -a
  sestatus
  getenforce
  semodule -l | grep -E "^k3s([[:space:]]|$)" || true
  rpm -q k3s-selinux container-selinux selinux-policy selinux-policy-targeted
  rpm -qi k3s-selinux || true
  zypper --non-interactive lr
'\'''
```

SELinux contexts and AVCs:

```sh
ssh -i "$SSH_KEY_FILE" "ec2-user@$NODE" 'sudo sh -c '\''
  ls -Zd /var/lib/rancher/k3s/storage
  ls -Zd /var/lib/rancher/k3s/storage/* 2>/dev/null || true
  journalctl -k --since "30 minutes ago" --no-pager \
    | grep -E "avc:.*(mkdir|rm|storage|container_t)" || true
'\'''
```

Kubernetes state:

```sh
ssh -i "$SSH_KEY_FILE" "ec2-user@$MASTER" 'sudo sh -c '\''
  K=/usr/local/bin/k3s
  $K kubectl -n kube-system get configmap local-path-config -o yaml
  $K kubectl -n kube-system get pods -o wide | grep helper-pod || true
  $K kubectl get pv,pvc -A -o wide
  $K kubectl get events -A --sort-by=.lastTimestamp \
    | grep -E "FailedProvisioning|VolumeFailedDelete" || true
'\'''
```

Inspect a helper before it is replaced by the next retry:

```sh
HELPER=$(kubectl -n kube-system get pod -o name \
  | grep 'helper-pod-delete' | tail -n 1)
kubectl -n kube-system get "$HELPER" -o yaml
kubectl -n kube-system logs "$HELPER"
kubectl -n kube-system describe "$HELPER"
```

Diagnostic-only enforcing/permissive experiment:

```sh
# Record the initial mode and always restore it.
ssh -i "$SSH_KEY_FILE" "ec2-user@$NODE" 'getenforce'
ssh -i "$SSH_KEY_FILE" "ec2-user@$NODE" 'sudo setenforce 0'

# Wait for one local-path delete retry, then verify PV/directory removal.
kubectl get pv "$PV_NAME" -w

ssh -i "$SSH_KEY_FILE" "ec2-user@$NODE" \
  'sudo journalctl -k --since "10 minutes ago" --no-pager | grep "avc:.*rm"'
ssh -i "$SSH_KEY_FILE" "ec2-user@$NODE" 'sudo setenforce 1 && getenforce'
```

## Suggested upstream issue scope

Suggested title:

> SELinux enforcing: bundled local-path helper still cannot delete MCS-labeled PV directories

Frame the report as a **silent regression of a closed issue**, not a new
discovery — this is what kept every previous report from landing:

- `k3s-io/k3s#10130` was closed as fixed on 2024-06-06, validated against a
  build containing `#402`; the revert `#421` landed 2024-06-13 — seven days
  after closure — and nobody reopened the issue.
- The revert's objection was that hard-coding the level in Go overrode
  user-provided helper templates. That objection does not apply to shipping
  the level in K3s's OWN bundled `helperPod.yaml` — the requested fix.
- Later reports died without code changes: `#460` self-closed on the
  workaround the same day; `#484` was closed by the stale bot (2025-08-24);
  `k3s-selinux#52` has zero comments since 2023-11.

The report should reference the existing history instead of presenting this as
a newly discovered root cause:

- `k3s-io/k3s#10130`: original K3s report and maintainer confirmation of the
  MCS mismatch;
- `rancher/local-path-provisioner#402`: original full-range helper fix;
- `rancher/local-path-provisioner#421`: revert because the hard-coded value
  overrode user configuration;
- `rancher/local-path-provisioner#484`: recurrence closed by stale automation;
  and
- `k3s-io/k3s-selinux#52`: still-open related policy discussion.

Requested changes:

1. Add `securityContext.seLinuxOptions.level: s0-s0:c0.c1023` to the helper
   template bundled by K3s, or provide an equally effective cross-MCS solution
   that does not overwrite user configuration.
2. Extend the K3s local-storage test to wait until the PV object and backing
   directory are actually deleted under a real SELinux-enforcing environment.
3. Verify that create and delete helpers can operate on the same volume when
   Kubernetes assigns them different MCS pairs.

Questions worth leaving to maintainers:

1. Is the broad `s0-s0:c0.c1023` range still the preferred mitigation, or
   should the provisioner preserve/reuse the volume's specific MCS label?
2. Should K3s ship this in its default manifest now that local-path-provisioner
   intentionally no longer hard-codes it?
3. Is the absence of visible enforcing AVCs for some failed `rm` attempts
   caused by an expected `dontaudit` rule?

Keep the qa-infra RHEL-only package installation gap as a separate issue/PR.

## Cleanup and publication checklist

- The probe namespace, PV and backing directory were removed.
- Both diagnostic nodes were restored to SELinux `Enforcing`.
- The Jenkins cluster was intentionally preserved with `-destroy false`; clean
  its remaining AWS resources after evidence collection.
- Before posting publicly, remove internal Jenkins links, hostnames, IPs, AMI
  IDs, AWS account/network identifiers, repository entitlement URLs, and any
  credential names.
- Never attach the full Jenkins parameters JSON because it may expose secret
  parameters even when console masking is enabled.
