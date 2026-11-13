# Release artifact checks

Use `scripts/release_checks.sh` locally or dispatch **Run Release Checks**.
Install `curl`, `jq`, `skopeo`, GnuPG (`gpg`), `gpgv`, `gzip`, `xmllint`
(`libxml2-utils` on Ubuntu) and `sha256sum` or `shasum` first.
The script uses temporary directories and removes downloaded RPMs after verification.

## Choose the publication type

| Publication | Workflow input | Local command |
| --- | --- | --- |
| Public K3s/RKE2 | `k3s_versions` / `rke2_versions` | `sh scripts/release_checks.sh -v 'v1.37.2+rke2r1' -d` |
| Prime-only/LTS RKE2 | `rke2_lts_versions` | `sh scripts/release_checks.sh -l 'v1.32.13-rc1+rke2r3' -d` |

Inputs accept comma-separated versions. Do not put a Prime-only release in
`rke2_versions`: public Docker Hub/GitHub checks cannot find its Prime-only
artifacts. The release suffix alone does not determine whether a release is LTS.

## LTS coverage

- Match artifact links for the exact release in the Prime index. Fetch both
  architecture SHA256 manifests and require their files to be indexed, including
  Linux binaries, tarballs and compressed airgap images. HTML formatting and nearby
  versions do not count as artifacts.
- Check runtime, system-agent-installer and upgrade image tags in the Prime registry.
  Failed registry queries fail the check even if they produced partial output.
- Check `rke2-agent`, `rke2-common` and `rke2-server` of the exact requested version
  and architecture in each selected RPM repository. Verify repository signatures
  with the HTTPS-published Rancher key, primary metadata SHA256, then download each
  RPM and verify its size and SHA256. Also verify the common repository and its most
  recently built `rke2-selinux` artifact.

RPM repositories are discovered during each run from these known locations:

- RC: `rpm-testing.rancher.io/rke2/testing/<minor>/<target>/`.
- GA: `rpm.rancher.io/rke2/latest/<minor>/<target>/`, then the corresponding
  `stable` channel. GA checks never fall back to testing artifacts.

For each channel, check the `/prime/` layout first, then the ordinary repository.
Verify repository signatures and primary metadata, including the `metadata` root
in the `http://linux.duke.edu/metadata/common` namespace, before selecting a
repository containing the exact requested RPM version and architecture. A 200
response or a nearby version is insufficient. A repository containing only some
of the required RPMs fails validation; it cannot be bypassed by falling back to
another repository.
Common SELinux repositories use the selected channel and do not have `/prime`.

Only HTTP 404 or verified metadata without the requested version allows trying
the next location. Network errors, HTTP 403/5xx, invalid signatures and corrupt
metadata fail. If no candidate contains the version, report **NOT VERIFIED** and
exit nonzero, preserving the other artifact/registry results. Absence alone does
not establish whether RPM publication was expected or whether the release is
defective; confirm that with the publisher. The script cannot discover arbitrary
new locations that have not been added to its known candidates.

On 2026-11-13, the RC `v1.32.13-rc1+rke2r3` passed full RPM verification.
For GA `v1.32.13+rke2r2`, both `slemicro/x86_64/prime/` paths returned 404;
the ordinary `latest` and `stable` signed repositories contained `rke2r1`, not
the requested `rke2r2`. GA RPM coverage for that release remains unverified.

The default RPM target is **`slemicro/x86_64`**, not every supported platform.
Set workflow input `rke2_lts_rpm_targets`, or locally:

```sh
LTS_RPM_TARGETS=slemicro/x86_64,slemicro/aarch64 \
  sh scripts/release_checks.sh -l 'v1.32.13-rc1+rke2r3' -d
```

Supported target syntax is `slemicro`, `microos` or `centos/7` through `centos/10`,
followed by `/x86_64` or `/aarch64`. Select only targets expected for that release.
A missing repository, package, signature or checksum fails; older LTS releases
without Prime RPM publication no longer pass this expanded check.

This is publication validation. It does **not** install RPMs, solve dependencies,
exercise the cluster, download container layers or verify large tarball payloads.
RPM payload integrity is verified against signed repository metadata; the script
does not separately run `rpmkeys` against each package's embedded signature.

## Local checks

```sh
sh -n scripts/release_checks.sh
shellcheck scripts/release_checks.sh
```

Run the release command above to check published artifacts. All validation is in
the shell script; no Python helper or test file is required.
