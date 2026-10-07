# RKE2 Prime Kata suite

`entrypoint/kata` is a normal infrastructure-owning Ginkgo suite. It uses
`SetupClusterInfra`, OpenTofu/Ansible, shared reporting and `-destroy`; it does
**not** take a pre-created cluster. `KUBE_CONFIG` is rejected before provisioning.
The suite covers the **Prime** CPU-only amd64/x86-64 scope (Intel or AMD, not ARM).
It does not assert that upstream Kata is technically restricted to Prime.
No NVIDIA non-confidential or confidential scenarios are included in this suite.

The entrypoint calls `TestKata*` cases in `internal/pkg/testcase/kata.go`.
Helpers live in the existing `internal/pkg/testcase/support` package, grouped into
configuration/lifecycle, cluster setup, runtime identity, workloads,
isolation and performance files. The optional manual-run fixture Dockerfile lives there too.
Host observations use Go parsing of standard Linux/CRI commands over SSH;
there is no additional runtime-language dependency. Initialization happens in
KATA-01, and fixture cleanup uses the suite's `AfterSuite` alongside shared
infrastructure teardown. The cases run in an `Ordered` container because they
share fixtures; `--randomize-all` does not reorder those cases. Overall timeouts
are set by the runner, not individual specs. Kata command execution does not log
each failed polling attempt: stderr is preserved in the returned error, and a
timeout reports the last observation through the failing test.

| Scenario | Automated check |
| --- | --- |
| KATA-01 | Install chart on one worker; keep ordinary worker and default runc unaffected |
| KATA-02 | Explicit runtime and alias; pod UID → CRI sandbox → containerd runtime → shim → KVM VM |
| KATA-03 | DNS and Service HTTP in both directions, colocated and cross-worker |
| KATA-04 | RuntimeClass selector-specific rejection; no sandbox on any node, no fallback |
| KATA-07 | RKE2 agent restart, existing workload recovery, new workload creation and traffic |
| KATA-14 | Guest boot/kernel identity, host-only resource markers, guest-confined mount |
| KATA-15 | Three cold + three warm starts per runtime; three idle CPU/PSS samples per runtime |

This is an isolation smoke, not penetration testing or a security certification.
Performance is characterization without an acceptance threshold. Missing or
failed observations fail the characterization case instead of silently passing.

## Prerequisites and provisioning

Follow [QA Infra configuration](qa-infra-integration.md) first. The selected
qa-infra ref must include `worker_nested_virtualization` in its AWS
`cluster_nodes` module and the role-aware RKE2 `server_flags`/`worker_flags`
resolver. Select a published, reviewed tag/branch in `QA_INFRA_REF` and record
its resolved SHA in the execution evidence. The nested-virtualization dependency
is tracked in [qa-infra PR #250](https://github.com/rancher/qa-infra-automation/pull/250).
The example and JJB job default to `main`; confirm that it contains these changes
before running. If they are not merged yet, keep the job disabled or explicitly
select a published, reviewed ref that includes both dependencies.
A ref predating the module input fails OpenTofu validation instead of silently
ignoring nested virtualization. Do not use an older playbook that lacks role-aware
flag resolution: DTF no longer forwards server flags as global additional config.

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
QA_INFRA_REPO=rancher/qa-infra-automation
QA_INFRA_REF=main
PROVISIONER_TYPE=opentofu
ARCH=amd64
NO_OF_SERVER_NODES=1
NO_OF_WORKER_NODES=2
SERVER_FLAGS='prime: true\nselinux: true\nprofile: cis'
WORKER_FLAGS='selinux: true\nprofile: cis'
INSTALL_VERSION=<approved-rke2-version>
INSTALL_CHANNEL=<approved-channel>
INSTALL_METHOD=tar
RESOURCE_NAME=<unique-prefix>
KATA_EVIDENCE_ROOT=/persistent/private/kata-evidence
```

`prime: true` is server-only; agents inherit registry selection during bootstrap.
Both roles require `selinux: true` and `profile: cis` from first boot. The suite
rejects `prime` in worker flags instead of treating an ignored flag as evidence.
Prime evidence comes from the server configuration; effective registry evidence
comes from running kube-proxy images on every node and API-server/etcd images on
the server. This initial suite requires kube-proxy (no proxy-free CNI mode).
Future Track B/C coverage with Cilium kube-proxy replacement needs a different
approved system-image oracle before that mode can run; do not skip the check.

The suite owns its fixed defaults; there are no Kata image, manifest, config,
build-type or expected-registry job parameters. It enables worker nested
virtualization itself before provisioning.

The existing version, install channel and server configuration select the
security baseline. Testing/RC/commit builds require
`system-default-registry: stgregistry.suse.com` on the server. A GA release
on stable/latest without that staging override expects `registry.rancher.com`
through Prime's automatic selection. An explicit staging override selects the
staging baseline even for a release-shaped tag. Confirm promotion before choosing
the GA channel; a version string alone is not evidence of promotion. Workers
inherit the registry; a conflicting explicit override fails preflight.

Host enforcement is fixed by the approved homogeneous `NODE_OS` row:
`rhel10`/`sles16` require SELinux Enforcing and CRI SELinux;
`ubuntu`/`sles15` select the approved AppArmor-only configuration and require
the containerd runtime-default profile `cri-containerd.apparmor.d` loaded in
`enforce` mode on every node. A runc witness must also report that exact profile
in `enforce` mode through `/proc/1/attr/current`; module enablement and
`disableApparmor=false` alone are insufficient. Other OS labels fail preflight
until their row is reviewed. The latter rows are not SELinux-enforcement passes; do not select them
to bypass an unexpected loss of enforcement. Both roles still require
`selinux: true` and `profile: cis` in RKE2. Prime, kernel-protection settings,
runtime enforcement and system-image registries are checked before/after install
and after restart against the saved baseline.

The AppArmor baseline is captured after the initial runc fixtures start and
before Kata is installed: containerd can load its default profile lazily. The
existing runc workloads are witnesses on the workers; one additional restricted
sleep pod is bound to the server solely for this check. Their manifests request
`runtime/default` using the AppArmor annotation supported by the current Go API
dependency. Each check correlates the witness UID/node and CRI runtime identity,
records the required profile/mode and process confinement, and rejects a missing,
`complain` or `unconfined` result. It never loads a custom policy or changes profile
modes to obtain a pass. Unrelated OS profiles and guest-side Kata AppArmor are
not claimed as validated. The [aa-status documentation](https://www.apparmor.net/man/3.1/aa-status/)
distinguishes module enablement from enforcing profiles; the
[containerd implementation](https://github.com/containerd/containerd/blob/v2.3.4/internal/cri/sputil/apparmor_linux.go)
defines runtime-default handling.

## Fixed fixture images

`support/kataconfig.go` pins two single-platform linux/amd64 BusyBox 1.37.0
manifests: glibc for functional checks and musl exclusively for performance.
Kata and runc use the **same** performance image. Its manifest/config/layer
digests and overlayfs chain are fixed in the test and were checked against the
registry metadata. Updating fixtures is a reviewed code change, not a Jenkins
parameter change. Nodes must be able to pull these public images.

The automated suite no longer requires building/pushing a fresh image or
supplying raw JSON for each run. It owns a fresh cluster and reserves the fixed
performance image for characterization. Image aliases, other CRI users, retained
content or a shared snapshot prevent a cold claim; the suite fails rather than
deleting shared blobs or weakening its cache oracle. The custom-image recipe in
the manual runbook remains separate from these automated fixed defaults.

Cold means the fixture's manifest, config, layer and overlayfs chain are absent;
warm means all are present before creating the pod. Actual workload snapshotter
must be overlayfs. Guest kernel, pause image, host page cache, registry caches
and installer images are not made cold. Cache-query/GC failures stop the case.
Startup uses the pod's API-recorded `creationTimestamp` and the `Ready=True`
condition's `lastTransitionTime`, saved with the pod UID in the raw evidence.
Both runtimes execute the same long-lived sleep command, without an HTTP server,
readiness/liveness probes or other periodic work. `Ready` measures container
startup, not HTTP application readiness; functional HTTP/DNS probes remain in
the non-performance scenarios. Prewarming uses this same idle fixture.
Client command and polling latency are excluded. These timestamps have
one-second resolution and require synchronized API-server/worker clocks; missing
timestamps or a negative duration fail the sample. A zero-second observation is
valid at this resolution, not a claim of instantaneous startup. Kubelet status
reporting still contributes to the measurement. These results must not be compared
directly to earlier HTTP-probed or client-observed startup baselines. Runtime order
alternates by round. Each warm pod also settles 30 seconds and receives six
five-second host-side CPU/PSS observations including its correlated VM/shim/helper
processes. PID/start-time, sandbox and container identity checks remain strict; a genuine process replacement
or missing observation is not silently skipped to obtain a measurement. Summary
reports median/worst startup, per-pod median idle cost and Kata-minus-runc medians;
raw observations preserve the full distribution.

## Run and cleanup

```bash
make test-kata DESTROY=true
# Equivalent:
go test -timeout=180m -v -count=1 ./entrypoint/kata/... -destroy true --ginkgo.timeout=175m
```

The existing runner accepts `TEST_DIR=kata`; Jenkins selects
`TEST_DIRECTORY=kata`. Mower's JJB definition is `rke2_kata_qainfra`.
Keep it disabled until the suite and Jenkinsfile are published at both its SCM
and runtime checkout refs. Use a timeout of at least 180 minutes. Qase uses
assigned case/run IDs, not invented IDs for KATA labels.

There is no Kata image/JSON argument forwarding in the Jenkinsfile. It only sets
the internal evidence output path for this suite to
`<agent workspace root>/qainfra-state/<job>/kata-evidence/<run_id>`.
This sits outside both the checkout and disposable infrastructure state, so
successful destroy and container removal preserve it. Retention and redacted
export are operator-managed; raw evidence is not publicly archived.

The job console log is the run's record: each check logs what it verified
(`Kata: …` lines), including security baselines, runtime identities, traffic
results and the performance summary. The per-run directory under
`KATA_EVIDENCE_ROOT` holds raw working files only; on Jenkins it lives in the
agent workspace and is not archived. Raw files include node identities and
internal addresses: redact them before public publication. No keys, tokens,
kubeconfigs or full RKE2 configuration are copied into the evidence bundle.

Owned workload/probe namespaces are removed even after a scenario failure. With
`-destroy=true`, the shared AfterSuite destroys this run's infrastructure; the
existing qa-infra interrupted-build cleanup remains the backstop. With
`-destroy=false`, the cluster, Kata HelmChart, installer namespace and node label
are deliberately retained for investigation. The suite does **not** perform the KATA-08
uninstall scenario or claim cleanup-hook coverage.

## Offline verification

```bash
go test -mod=readonly -race -count=1 ./internal/resources ./internal/pkg/testcase/... ./internal/provisioning/qainfra
go test -mod=readonly -c ./entrypoint/kata -o /tmp/dtf-kata.test
golangci-lint run --tests ./internal/pkg/testcase/... ./internal/provisioning/qainfra ./entrypoint/kata
```

Compilation is not end-to-end acceptance. Live provisioning, all seven scenarios,
four representative runs per the PR checklist, OS/install-method coverage and
cleanup must still be validated before calling this suite production-ready.
Kata helper unit-test files are deferred for now; the commands above compile the
helpers and run the existing package tests, not the seven infrastructure scenarios.
