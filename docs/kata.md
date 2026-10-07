# RKE2 Prime Kata P0 suite

`entrypoint/kata` is a normal infrastructure-owning Ginkgo suite. It uses
`SetupClusterInfra`, OpenTofu/Ansible, shared reporting and `-destroy`; it does
**not** take a pre-created cluster. `KUBE_CONFIG` is rejected before provisioning.
The suite covers the **Prime** CPU-only amd64/x86-64 scope (Intel or AMD, not ARM).
It does not assert that upstream Kata is technically restricted to Prime.
No NVIDIA non-confidential or confidential scenarios are included in P0.

The entrypoint calls `TestKata*` cases in `internal/pkg/testcase/kata.go`.
Helpers live in the existing `internal/pkg/testcase/support` package, grouped into
configuration/lifecycle, cluster setup, runtime identity, workloads,
isolation and performance files. The dedicated performance Dockerfile lives there too.
Host observations use Go parsing of standard Linux/CRI commands over SSH;
there is no additional runtime-language dependency. Initialization happens in
KATA-01, and fixture cleanup uses the suite's `AfterSuite` alongside shared
infrastructure teardown. Overall timeouts are set by the runner, not individual specs.

| Scenario | Automated check |
| --- | --- |
| KATA-01 | Install chart on one worker; keep ordinary worker and default runc unaffected |
| KATA-02 | Explicit runtime and alias; pod UID → CRI sandbox → containerd runtime → shim → KVM VM |
| KATA-03 | DNS and Service HTTP in both directions, colocated and cross-worker |
| KATA-04 | RuntimeClass selector-specific rejection; no sandbox on any node, no fallback |
| KATA-07 | RKE2 agent restart, existing workload recovery, new workload creation and traffic |
| KATA-14 | Guest boot/kernel identity, host-only resource markers, guest-confined mount |
| KATA-15 | Five cold + five warm starts per runtime; five idle CPU/PSS samples per runtime |

This is an isolation smoke, not penetration testing or a security certification.
Performance is characterization without an acceptance threshold. Missing or
failed observations fail the characterization case instead of silently passing.

## Prerequisites and provisioning

Follow [QA Infra configuration](qa-infra-integration.md) first. The selected
qa-infra ref must include `worker_nested_virtualization` in its AWS
`cluster_nodes` module. The dependency is developed on
`fmoral2/qa-infra-automation:feat-kata-nested-virtualization`; publish/review that
branch before using it remotely, then pin the reviewed ref. A ref predating this
input fails OpenTofu validation instead of silently ignoring nested virtualization.

Use one server and two worker-only nodes. Select a supported nested-virtualization
instance type, for example `c7i.2xlarge`, in `infrastructure/qainfra/vars.tfvars`.
The module checks AWS's advertised CPU capability. Both workers have KVM but only
one receives Kata: the other is the scheduling-negative and cross-node peer.
Use an approved amd64 AMI, explicit RPM or tar installation method, enough disk
space (for example 60 GiB), existing QA VPC/subnet/security groups, SSH key and a
unique resource prefix. Do not change an existing cluster to satisfy this suite.

Representative environment additions (replace approved versions and paths):

```bash
ENV_PRODUCT=rke2
PROVISIONER_MODULE=qainfra
QA_INFRA_PROVIDER=aws
PROVISIONER_TYPE=opentofu
QA_INFRA_WORKER_NESTED=true
ARCH=amd64
NO_OF_SERVER_NODES=1
NO_OF_WORKER_NODES=2
SERVER_FLAGS='prime: true\nselinux: true\nprofile: cis'
WORKER_FLAGS='prime: true\nselinux: true\nprofile: cis'
INSTALL_VERSION=<approved-rke2-version>
INSTALL_METHOD=tar
RESOURCE_NAME=<unique-prefix>
KATA_TEST_IMAGE=<approved-busybox-repository>@sha256:<amd64-manifest-digest>
KATA_PERF_IMAGE=<dedicated-fixture-repository>@sha256:<manifest-digest>
KATA_PERF_MANIFEST=/absolute/path/to/perf-manifest.json
KATA_PERF_CONFIG=/absolute/path/to/perf-config.json
KATA_EVIDENCE_ROOT=/persistent/private/kata-evidence
```

All three Prime/CIS/SELinux flags must be present for servers and workers at first
boot. For RC/staging builds, also set the approved staging registry explicitly
through `system-default-registry`. SELinux configuration is not proof of host
enforcement: the suite requires Enforcing on RPM-family hosts, and records the
actual mode on Ubuntu/Debian without claiming SELinux coverage there.
The suite does not bypass unsupported RPM/SELinux combinations.
The Kata chart's `selinux.enabled` follows actual enforcing mode on the selected
worker; it is not forced on an AppArmor-only host. RKE2's `selinux: true` remains
required in both cases.

The ordinary test namespace uses PSA `restricted`. Exceptions are limited to the
Kata installer namespace and a separate synthetic mount-probe namespace. The
probe adds only guest `SYS_ADMIN` with unconfined seccomp, not `privileged: true`,
host namespaces or host mounts. If another admission/LSM policy blocks it, the
case fails with the cause; the suite does not relax that policy automatically.

The test runner needs `kubectl`, OpenSSH and the normal qa-infra dependencies.
Workers need `bash`, `ip`, `lsblk`, `base64`, usable `/dev/kvm` and passwordless sudo for
QA. SSH uses a private, run-specific known-hosts file with accept-new, never a
global host-key-checking bypass. Kata chart is pinned to **4.2.0**.

## Performance fixture

Use a dedicated, single-layer linux/amd64 BusyBox image for each execution. It
must not be shared with system/functional workloads; the suite removes only this
exact image through CRI, after its pods/sandboxes/VMs are gone. It never prunes
containerd globally or drops the host page cache. Example fixture preparation:

```bash
docker buildx build --platform linux/amd64 --provenance=false --sbom=false \
  --build-arg BASE="$KATA_TEST_IMAGE" --build-arg FIXTURE_ID="$UNIQUE_FIXTURE_ID" \
  -f internal/pkg/testcase/support/kata-perf.Dockerfile \
  -t "$DEDICATED_PERF_TAG" --push internal/pkg/testcase/support
skopeo inspect --raw "docker://$DEDICATED_PERF_TAG" > perf-manifest.json
skopeo inspect --config "docker://$DEDICATED_PERF_TAG" > perf-config.json
skopeo manifest-digest perf-manifest.json
```

Set `KATA_PERF_IMAGE` to that repository plus the printed digest and provide both
JSON paths. Choose a fresh nonempty `UNIQUE_FIXTURE_ID` before building; this is
fixture preparation and does not require a running cluster.
These files must be accessible **inside** the test runner if using a
container; export variables or include them in its ignored `config/.env`.
Both images must be pullable by the new cluster; this initial suite does not
inject imagePullSecrets. Preflight verifies manifest/config digests, one layer,
linux/amd64 and a distinct performance image before creating infrastructure.

Cold means the fixture's manifest, config, layer and overlayfs chain are absent;
warm means all are present before creating the pod. Actual workload snapshotter
must be overlayfs. Guest kernel, pause image, host page cache, registry caches
and installer images are not made cold. Cache-query/GC failures stop the case.
Startup measures immediately before `kubectl create` to observed Ready (one-second
polling, including client/API overhead). Runtime order alternates by round. Each
warm pod also settles 30 seconds and receives twelve five-second host-side
CPU/PSS observations including its correlated VM/shim/helper processes. Summary
reports median/worst startup, per-pod median idle cost and Kata-minus-runc medians;
raw observations preserve the full distribution.

## Run and cleanup

```bash
make test-kata DESTROY=true
# Equivalent:
go test -timeout=180m -v -count=1 ./entrypoint/kata/... -destroy true --ginkgo.timeout=175m
```

The existing runner accepts `TEST_DIR=kata`. A Jenkins job still needs its own
configuration/parameters and timeout (at least 180 minutes); no job is created by
this change. Qase uses the shared suite reporter and existing configured case/run
IDs, not invented IDs for KATA scenario labels.

Evidence is retained in a private per-run directory under `KATA_EVIDENCE_ROOT`
(default: local temporary directory); mount a persistent root for containers and
copy/archive it before removing the runner. Raw evidence includes node identities
and internal addresses: redact it before public publication. No keys, tokens,
kubeconfigs or full RKE2 configuration are copied into the evidence bundle.

Owned workload/probe namespaces are removed even after a scenario failure. With
`-destroy=true`, the shared AfterSuite destroys this run's infrastructure; the
existing qa-infra interrupted-build cleanup remains the backstop. With
`-destroy=false`, the cluster, Kata HelmChart, installer namespace and node label
are deliberately retained for investigation. P0 does **not** perform the KATA-08
uninstall scenario or claim cleanup-hook coverage.

## Offline verification

```bash
go test -mod=readonly -race -count=1 ./internal/pkg/testcase/... ./internal/provisioning/qainfra
go test -mod=readonly -c ./entrypoint/kata -o /tmp/dtf-kata.test
golangci-lint run --tests ./internal/pkg/testcase/... ./internal/provisioning/qainfra ./entrypoint/kata
```

Compilation is not end-to-end acceptance. Live provisioning, all seven scenarios,
four representative runs per the PR checklist, OS/install-method coverage and
cleanup must still be validated before calling this suite production-ready.
Kata helper unit-test files are deferred for now; the commands above compile the
helpers and run the existing package tests, not the seven infrastructure scenarios.
