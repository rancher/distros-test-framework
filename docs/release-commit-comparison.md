# GA-to-RC commit comparison

`scripts/compare_release_commits.sh` lists the commits between a preceding published
GA and each supplied K3s/RKE2 RC, like GitHub's `compare/<GA>...<RC>` view. It does
not validate linked issues, test coverage, or parity between main and backports.
A successful comparison means collection succeeded, not that the changes are approved.

## Existing release-checks workflow

Run `.github/workflows/release-checks.yaml` with its existing `k3s_versions`,
`rke2_versions` and/or `rke2_lts_versions` inputs. The **Compare GA to RC commits**
step collects the RC tags from all three inputs. GA-only runs retain their existing
artifact checks and skip commit comparison.

Each RC normally uses the highest published, non-draft, non-prerelease GA below it
in the same major/minor and product. Patch numbers and product revisions are sorted
numerically: `v1.37.10-rc2+rke2r1` uses `v1.37.9+rke2r1`, not `v1.37.2+rke2r1`;
`v1.37.9-rc1+k3s2` can use `v1.37.9+k3s1`. This is version ordering, not publication
date ordering. The selected baseline is printed in the report.

Override specific baselines with the optional `commit_baselines` JSON input:

```json
{"v1.26.14-rc1+rke2r1":"v1.26.13+rke2r1"}
```

A new-minor `.0` RC with no earlier GA in its minor requires an explicit baseline.
The script does not silently substitute a different minor. Overrides must be real
published GA releases of the same product, and their keys must name supplied RCs.

The step appends its Markdown report to the Actions summary and uploads
`report.json` and `summary.md` as the `release-commit-comparison` artifact, including
on failure. Temporary authentication files are never uploaded. Failures from this
step participate in the workflow's final failure check.

## Local use

Requires Bash, curl and jq. `GH_TOKEN` (or `GITHUB_TOKEN`) is optional for public
repositories, but recommended to avoid GitHub's unauthenticated rate limit.

```bash
bash scripts/compare_release_commits.sh \
  --versions 'v1.26.14-rc1+rke2r1' \
  --baselines '{"v1.26.14-rc1+rke2r1":"v1.26.13+rke2r1"}' \
  --output-dir tmp/release-commits
```

`--versions` accepts comma-separated full RC tags, including mixed products.
`--request-id` optionally records a caller ID in the JSON report. The default output
directory is ignored by Git. Rerunning replaces the two reports in that directory.

Tags are resolved to commit SHAs before comparison. The report includes both the
human-readable tag comparison and an immutable SHA comparison, plus every commit's
SHA, subject and link. GA discovery lists matching tags only for the target minor
and confirms that candidates are published releases. Comparison responses are paginated;
incomplete, duplicate or inconsistent comparison pages are rejected. Failure for
one RC does not stop collection for the others.

## Results

| Exit | Meaning |
| --- | --- |
| `0` | All comparisons were collected completely, and each RC is ahead of its GA. |
| `1` | Attention required: an RC is identical to, behind, or diverged from its baseline. |
| `2` | Input, baseline or collection error; the comparison is not complete. |

An identical tag may be an intentional rebuild; a diverged branch may be deliberate.
These are attention signals for a person, not automatic product-bug findings.
No commit mismatch policy or issue-status gate is applied by this first version.

## Tests

```bash
bash -n scripts/compare_release_commits.sh scripts/compare_release_commits_test.sh
shellcheck scripts/compare_release_commits.sh scripts/compare_release_commits_test.sh
bash scripts/compare_release_commits_test.sh
```

The tests replace only curl's HTTP boundary and exercise the actual script with
paginated responses, mixed products, explicit baselines and collection failures.
They also run under `make unit-tests` without network access or real credentials.
