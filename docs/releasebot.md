# Release bot

Turns a release request posted in Slack ("@bot please test v1.37.1-rc1+k3s1 v1.37.1-rc2+rke2r1")
into the three things we do by hand today for every RC round:

1. **Release checks**: dispatch `release-checks.yaml` (GitHub Actions, this repo).
2. **Qase runs**: dispatch `qase-patch-validation-create.yaml`, which creates the patch-validation
   runs for both products (plan ids rke2=14, k3s=20).
3. **Jenkins jobs**: schedule existing child jobs for each requested product/RC, with bounded
   concurrency per controller. Batch jobs remain manual entry points and coverage references;
   the bot does not launch them. Smoke dependencies exist; retries and full coverage reconciliation are planned below.

The bot reacts to a Slack mention (Socket Mode). It does not poll the channel.

## Status

| Piece | State |
|---|---|
| Parsing RC tags from Slack text | Done (`internal/pkg/releasebot/parse.go`) |
| Plan: workflows + Qase RCs + job expansion | Done (`plan.go`) |
| GitHub client: tag check + `workflow_dispatch` | Done (`github.go`) |
| Jenkins client: crumb, `buildWithParameters`, queue → build, result | Done (`jenkins.go`) |
| Scheduler: per-controller `maxConcurrent`, priority order, `dependsOn` gates | Done (`scheduler.go`); smoke retry not yet |
| Batch-to-matrix coverage reconciliation, smoke retry | Planned (smoke dependencies themselves are done, see Scheduler) |
| Failure policy + verified LLM explanation | Design below; deterministic retry rules and bot integration with the triage skill are not implemented |
| Qase run id → `QASE_RUN_ID` of every job | Done (`qase.go`); title and `search` lookup checked read-only against the real API |
| Job matrix | Initial set (`config/releasebot/matrix.yaml`) |
| CLI with dry-run (`cmd/releasebot`) | Done |
| Unit tests | Done (`releasebot_test.go`: parsing, plan/order/prefix, Qase RC dedupe, Qase title/request-id matching/timeout, scheduler limits, transient and persistent query errors, lost trigger responses, canceled queue items, `dependsOn` release/block/validation, Jenkins trigger error classification over HTTP) |
| Slack app: bot token, Socket Mode, `app_mention` event | Done; the app-level token opens the socket and receives `hello` |
| Slack scopes `groups:read`, `groups:history`, `app_mentions:read` | **Waiting for workspace admin approval** |
| Socket Mode listener + confirmation step | Pending |
| Hosting (vSphere VM) | Pending |

Verified so far: `go build`, `go vet`, `go test -race` and `golangci-lint` are clean on the new
package, and a dry-run against real tags passes the GitHub tag check:

```
$ GITHUB_TOKEN=$(gh auth token) go run ./cmd/releasebot -message 'please test v1.37.1-rc2+rke2r1'
INFO Release plan (DRY-RUN), request rb-20260928T144012-152cc11f
INFO   k3s: - | rke2: v1.37.1-rc2+rke2r1 | rke2 LTS: -
INFO GitHub workflows (2):
INFO   rancher/distros-test-framework@main release-checks.yaml rke2_versions=v1.37.1-rc2+rke2r1
INFO   rancher/distros-test-framework@main qase-patch-validation-create.yaml rcs=v1.37.1-rc2 request_id=rb-20260928T144012-152cc11f
INFO Qase runs the workflow creates (ids fill {{QASE_RUN_ID}} once they exist):
INFO   RKE2 September 2026 Patch Validation for v1.37.1+rke2r1
INFO   K3S September 2026 Patch Validation for v1.37.1+k3s1
INFO Jenkins jobs (3):
INFO   P1 mower  distros_qa/rke2-tests/rke2_validate_cluster_qainfra     v1.37.1-rc2+rke2r1 prefix=rbr1371
INFO   P2 mower  distros_qa/rke2-tests/rke2_validate_cluster_rpm_qainfra v1.37.1-rc2+rke2r1 prefix=rbr1371p after rke2-validate-cluster
INFO   P2 mower  distros_qa/rke2-tests/rke2_conformance_qainfra          v1.37.1-rc2+rke2r1 prefix=rbr1371c after rke2-validate-cluster
```

(Timestamps trimmed.) Output goes through `resources.LogLevel`: the plan, dispatches, job
progress and summary are `info`; warnings, query failures and non-SUCCESS results are `warn`;
fatal errors are `error`. With `LOG_LEVEL=debug`, each job also logs its full expanded
parameters (`params: BRANCH=... HOSTNAME_PREFIX=... INSTALL_VERSION=... QASE_RUN_ID=...`).

Nothing has been dispatched or triggered for real yet (`-dry-run=false` has not been run).

## How it works

### Parsing

`ParseRequest` finds `vX.Y.Z-rcN+k3sN` and `vX.Y.Z-rcN+rke2rN` anywhere in the text. It undoes
Slack escaping first, so tags inside links (`<url|tag>`), code spans and `%2B`-encoded URLs still
match. Tags after `lts=` go to `rke2_lts_versions` (prime registry). Duplicates are dropped.
A message with no tags is rejected.

### Workflows

- `release-checks.yaml` gets `k3s_versions`, `rke2_versions`, `rke2_lts_versions` (comma
  separated; empty inputs are omitted).
- `qase-patch-validation-create.yaml` gets `rcs`, one `vX.Y.Z-rcN` per patch version. The script
  creates the runs for both products from each entry, so k3s and rke2 of the same version collapse
  into one. When the products are on different RCs of the same version (k3s rc1, rke2 rc2), the
  higher RC is used and the plan prints a warning.
- Both run on `dtfRef` from the matrix (`main`; both workflow files exist on `main` and
  `qa-infra-RC-1`). GitHub returns 204 without a run id, so the bot reports "dispatched", not a link.

### Qase run ids

Every Jenkins job reports to the patch-validation run of its product and version, so it needs
that run's id in `QASE_RUN_ID` (for example `1016` from
`https://app.qase.io/run/K3SRKE2/dashboard/1016`). The workflow does not return ids (it only
prints them in the Action log), so the bot finds them in Qase by title plus a request id:

1. Every plan gets a unique request id (`rb-<UTC timestamp>-<8 hex>`, `NewRequestID`), sent to
   the Qase workflow as the `request_id` input. The script appends
   ` | Release bot request: <id>` to the description of every run it creates
   (`Version: v1.37.1-rc1 | Release bot request: rb-20260928T135419-17836701`).
2. The title is deterministic and mirrors `scripts/qase-patch-validation.sh`:
   `<RKE2|K3S> <Month> <Year> Patch Validation for vX.Y.Z+<rke2r1|k3s1>` (`QaseRunTitle`).
3. After dispatching, the bot polls `GET /v1/run/K3SRKE2?search=<title>` until, for every title,
   a run exists with that exact title **and** a description ending in its own request marker
   (`WaitQaseRuns` / `MatchQaseRun`, default timeout 15 min). Runs from any other dispatch
   (manual, or an earlier workflow that finishes late) have no marker or a different one and are
   never taken, even when they are newer.
4. `{{QASE_RUN_ID}}` in the job params is replaced by that id (`ApplyQaseRunIDs`), and the
   matrix sets `REPORT_TO_QASE=true`. If a job needs an id that was not found, the bot stops
   before triggering any job, so nothing reports to a wrong run.

With `-skip-workflows` (runs created earlier by hand) there is no request id, so the newest run
with the exact title is used.

**Deployment dependency:** the `request_id` input exists only in this tree's version of
`qase-patch-validation-create.yaml`. GitHub rejects a dispatch with an unknown input (HTTP 422),
so the bot's Qase dispatch fails until this change is on the branch `dtfRef` points to (`main`).
The failure happens before any Jenkins job is triggered. The input is optional, so manual
dispatches from the Actions UI keep working unchanged.

Coupling with the script to keep in mind:

- The script hardcodes the `rke2r1`/`k3s1` suffix in the title whatever the tag says; the bot
  does the same. If the script changes its title format, `QaseRunTitle` must change with it.
- Month and year come from the GitHub runner clock (UTC). A request dispatched right at midnight
  UTC at the end of a month could produce a title the bot does not expect; the lookup then times
  out, and no job is triggered.
- The script always creates runs for **both** products for every version in `rcs`, even when
  only one product was requested.
- The script POSTs a new milestone on every call, so a second dispatch in the same month adds a
  second milestone with the same name (Qase already has two each for July 2025, September 2025,
  February 2026, May 2026 and July 2026). Fixed in the script: it now searches for the milestone
  by exact title first and reuses it (lowest id when duplicates exist), creating one only when
  none exists. A failed search (non-200, `status` not true, or no `entities` array) stops the
  script before anything is created, instead of being read as "no milestone". The existing
  duplicates were left in Qase.

Checked read-only on 2026-09-28 with `QASE_AUTOMATION_TOKEN`: run 1016 is titled
`RKE2 September 2026 Patch Validation for v1.35.9+rke2r1` (description `Version: v1.35.9-rc1`,
plan 14), exactly what `QaseRunTitle` builds. The `search` query the bot sends returns only that
run (1016), and for `K3S September 2026 Patch Validation for v1.35.9+k3s1` only run 1020.

### Jenkins matrix

`config/releasebot/matrix.yaml` lists controllers (URL + `maxConcurrent`) and job templates.
Unknown keys are rejected when the matrix loads, so a typo such as `depends_on` fails instead of
silently dropping a dependency.
Each template runs once per RC of its product. In params, `{{VERSION}}` becomes the full tag and
`{{QASE_RUN_ID}}` becomes the Qase run id (see above), and `{{PREFIX}}` becomes a short `HOSTNAME_PREFIX`: `<prefixBase><k|r><version digits>`, for example
`rbr1371`. Add a suffix per job (`{{PREFIX}}p`) so two jobs of the same RC never share AWS
resource names. The prefix is short because qa-infra caps `dsf-<prefix>-<product>-<id>` at 24
characters.

Params not set in the matrix keep the job's own defaults, so TFVARS, `NODE_OS` and the hardened
server flags (SELinux, CIS profile on RKE2, protect-kernel-defaults on K3s) come from the job
definitions in JJB rather than being duplicated here. This holds for the initial six jobs; it
does not hold for every batch scenario (see "Per-scenario parameters" below).

### Execution model (agreed design)

The bot owns scheduling at the **child-job level**. Existing install, airgap, upgrade and OS
batches fan out with `parallel jobs`: counting a batch as one slot would hide its children from
the bot's concurrency limit. Keep those batches unchanged for manual use; do not delete them
or launch a batch alongside its children in a bot release plan.

The next matrix/scheduler changes should implement:

- **Coverage groups:** installation, airgap, upgrades, conformance and OS variants. Reconcile
  the matrix with the existing batch/JJB definitions using an explicit batch-scenario-to-child
  mapping, including any parameters the batch supplies. Reuse the existing jobs and test code;
  do not copy their implementation into the bot. Deduplicate overlapping scenarios and show
  exclusions or unsupported combinations in the dry-run.
- **Smoke dependencies per product/RC:** wait for the designated smoke checks to pass before
  scheduling their dependents. A failed smoke is eligible for at most one automatic retry only
  when the deterministic failure policy permits it (see below), not just because it is a smoke.
  Otherwise, or if the retry fails, its dependents stay blocked and the bot asks in the Slack
  thread whether to continue. An installation failure holds those dependents, not other
  products/RCs. An isolated failure in a later validation does not cancel unrelated scenarios.
  This needs explicit dependency state; sorting by priority is not a success gate.
- **Bounded child execution:** after smoke, schedule functional tests, airgap, upgrades and
  conformance as capacity allows. Let long-running conformance start early after its smoke
  passes. Count accepted queue items as well as running builds; an unknown state must not
  release capacity. These limits cover the bot's jobs, not manual/cron builds, and must be
  sized with controller capacity and shared AWS account/region quotas in mind.
- **Per-scenario parameters:** pass exactly what the corresponding batch passes to that child,
  plus the requested versions/channels, refs, a unique resource prefix and the product/version's
  Qase run ID. Child defaults are not always the batch scenario:
  - Airgap keeps child defaults on purpose: `Jenkinsfile_batch_airgap_test` does not pass
    `TEST_ARGS`, so each child keeps its own `-tags`, `-tarballType` and registry flags.
  - Upgrade builds `TEST_ARGS` per child: `-tags=upgradesuc -sucUpgradeVersion <target>` or
    `-tags=upgrademanual -installVersionOrCommit <target>` (`Jenkinsfile_batch_upgrade_test`),
    with `INSTALL_VERSION` (start) and `UPGRADE_VERSION` (target) passed separately.
  - OS validation also sets the OS per child (`BATCH_OS_NODE_OS`, `NODE_OS_ARM` for ARM), the
    upgrade channel, and requires matching `INSTALL_VERSIONS`/`UPGRADE_VERSIONS` lists
    (`Jenkinsfile_batch_os_validation`).

  Firing those children with their own defaults would run a different scenario (for example an
  upgrade with no target version). Build each group's parameters in one place in the bot,
  following the batch Jenkinsfile logic, and add a test that compares them with what the batch
  would pass, so the two cannot drift apart silently. Confirm Qase case mappings before enabling
  reporting.
- **Upgrade starting version:** the RC is the target; the starting version needs a rule agreed
  with the team (latest GA of the same minor, or of the previous minor). The bot resolves it
  from GitHub Releases and shows it in the dry-run.

Direct child scheduling exists for the initial six jobs, and so do the minimal smoke dependency
gates (`dependsOn`, see Scheduler). The coverage mapping, deduplication, exclusion report and the
smoke retry above are planned, not implemented yet.

### Scheduler

Jobs are sorted by priority, then newest version first. Among the jobs whose dependencies are
met, the scheduler triggers as many as each controller's `maxConcurrent` allows, polls queue items until they become builds, polls builds until
they finish, and starts the next pending job as slots free up. A failure to trigger one job is
recorded and does not stop the others. On Ctrl-C/SIGTERM the remaining jobs are reported as
canceled; builds already running on Jenkins are not aborted.

Dependencies (`dependsOn` in the matrix):

- A matrix job lists other job **names of the same product**; each expanded job depends on those
  jobs **for the same RC** (e.g. `rke2-conformance` for `v1.37.1-rc2+rke2r1` waits for
  `rke2-validate-cluster` for `v1.37.1-rc2+rke2r1`). The dry-run prints `after <names>`.
- A dependent starts only after its dependencies ended with `SUCCESS`. Failure, abort,
  cancellation, trigger error or unknown state blocks it, and transitively its own dependents;
  jobs of other versions and products carry on.
- A job waiting for dependencies takes no capacity, so the smoke of another RC can use the slot.
  Priority only chooses among jobs whose dependencies are met.
- Unknown names, cross-product references, duplicate names and cycles are rejected when the
  matrix loads. The scheduler checks again before anything is triggered (unknown references,
  cycles, and duplicate product/version/name identities in the plan it is given), so a plan built
  outside `LoadMatrix` gets the same protection.
- The shipped matrix makes RPM validation and conformance depend on the plain validate-cluster
  smoke of the same product. No retry yet: a flaky smoke blocks its dependents for that RC.

Triggers whose outcome is unknown keep their slot. A trigger error frees the slot only when the
rejection is certain: an HTTP error status from Jenkins, or a request that never left (DNS or
connection failure). A lost response after the POST was sent (EOF, reset, timeout), a
502/503/504 from the proxy in front of Jenkins, a redirect (the trigger never follows redirects,
since the first hop may already have queued the build; point the controller URL at the final
address), or a 201 without a queue URL may mean Jenkins queued a build: the job is reported with "check for a build before re-running" and holds its slot
for the rest of the run.

Query errors never free a slot early, so a flaky controller cannot push it over `maxConcurrent`:

- A failed queue or build query (503, timeout, 403, 404...) is reported through `Notify` with a
  counter and retried on the next poll; the job keeps its slot. A successful query resets the
  counter.
- After `MaxPollErrors` (10) consecutive failures the job is reported as "state unknown" with
  the last error and its build or queue URL to check by hand. It keeps holding its slot for the
  rest of the run, since the build may still be running.
- A job in unknown state takes only its own slot; the controller keeps using the rest. Only when
  such jobs fill the controller's whole `maxConcurrent` is it marked unreachable and its
  pending jobs reported as not triggered, instead of waiting forever.
- A queue item Jenkins canceled never ran, so its slot is released at once.

### Failure triage and retry policy (planned bot integration)

This section is the self-contained contract; it does not require access to a
private skills repository. Keep scheduling decisions separate from the failure explanation:

- **Deterministic policy decides whether a rerun is eligible.** Inputs are the confirmed
  Jenkins terminal result, failed stage, scoped log signals, scenario/ref and retry count.
  Implement explicit, versioned rule IDs with positive and negative fixture tests; no LLM
  bucket, confidence or verifier verdict grants a retry, changes capacity or releases a
  dependency. Default to no automatic retry if evidence is missing or rules conflict.
- **The LLM explains the failure for the Slack thread.** Gather build metadata, the earliest
  causal log evidence and comparable N-1/N-2/flaky history. Search INFRA/JOB first as a cheap
  prior, but require causal evidence for the verdict. Use a fresh verifier for
  PRODUCT, confidence below 80%/UNKNOWN, or an explanation used to justify an issue or rerun.
  The verifier gets only the claim, quotes and source locators/access scope, without inherited
  conversation or the analyst's reasoning (a non-fork Agent in Claude Code, or an equivalent
  context-isolated spawn in Codex), using the same model as the analyst. Select and
  check the effective model explicitly; do not rely on Explore's default. It independently
  checks earlier causes, N-1/N-2, flaky history and the log emitter.
- **Restrict verifier tools in every handoff.** Allow only `getBuild`, `getBuildLog`,
  `searchBuildLog`, `getTestResults`, `getFlakyFailures`, `getBuildChangeSets` and `getJob`
  on the relevant controller/jobs, plus local evidence/instruction reads. Forbid
  `triggerBuild`, `replayBuild`, `rebuildBuild`, `updateBuild`, all other mutations,
  shell writes or Jenkins/AWS calls, credential access, external posts and further
  delegation. Apply an exact tool allowlist where supported; otherwise label it
  read-only by instruction, not a sandbox guarantee. Explore denying Edit/Write does
  not establish that Bash or MCP tools are read-only. Missing access or model parity
  means INCONCLUSIVE; never bypass the restrictions to collect evidence.
- **Report the verification result.** CONFIRMED retains only evidence-backed confidence;
  REFUTED retracts the claim and uses a supported alternative or UNKNOWN; INCONCLUSIVE
  lowers confidence below 80% (or n/a for UNKNOWN) and names the missing check. If a verifier
  is unavailable, disclose INCONCLUSIVE instead of treating self-review as independent.
  High-confidence non-PRODUCT informational triage can say NOT REQUIRED. Reports are concise
  and in English, with evidence, verdict, one next action and owner. Posting still requires
  the bot's authorized workflow; the skill itself neither posts nor reruns anything.
- **Bound verification cost.** Default to at most three verifier passes per triage request/batch,
  unless explicitly overridden. Group shared causes first and prioritize PRODUCT and
  action-driving findings before uncertain informational ones. Required checks beyond the
  budget are INCONCLUSIVE (verification budget exhausted), with lower confidence and a
  deferred check, not silently accepted. Failed passes count against the same budget.

Maintainer reference (private; access is not assumed):
[reviewed triage baseline, `e48b26b`](https://github.com/fmoral2/distros-qa-skills/blob/e48b26bc30c912ca5d723081070a4d5509714508/skills/jenkins-failure-triage/SKILL.md),
[tool/model follow-up, `f124b36`](https://github.com/fmoral2/distros-qa-skills/blob/f124b366853c7fcaee7d955ea3525e4cb6d17051/skills/jenkins-failure-triage/SKILL.md),
local checkout `../distros-qa-skills/skills/jenkins-failure-triage/SKILL.md`.

The skill's `references/signals.md` is a search catalog, not an executable retry allowlist.
For example, `connection refused` can follow a product crash, and a cluster failing to reach
Ready does not prove an infra failure. A retry rule must distinguish those from a confirmed
transient provisioning failure by stage and causal evidence. Historical flakiness alone is
not eligibility: require a reviewed, narrowly scoped scenario/signature allowlist.

Before any automatic retry: confirm the previous build has finished, the rule is enabled,
the retry budget remains, and resource reuse/cleanup is safe. Limit to one retry per logical
scenario/product/RC/request, counting it against `maxConcurrent`; use a fresh short resource
prefix without forgetting leaked resources from the first attempt. Unknown trigger/build
state, manual abort, build timeout, test assertion failure or unclassified failure must not auto-retry.
Persistent credential/quota/configuration errors require remediation, not blind retries.
These safety denials take precedence over any flaky-scenario allowlist match.
Dependents remain blocked until an eligible retry succeeds; other products/RCs continue.

The policy can run independently of LLM availability; a missing verifier does not turn into
permission to retry and does not veto an already eligible deterministic decision. The thread
should distinguish the policy's rule ID/evidence/attempt from the LLM's diagnosis/verdict.
Include a Bot action field only when a policy decision was supplied; omit it in manual triage.
Human-requested reruns remain a separate authorized action, not an LLM policy override.

This update defines the contract only: the CLI still has no automatic retry, Slack triage
invocation or verifier integration. The reviewed baseline is `e48b26b`; the integration
candidate including the tool/model follow-up is `f124b366853c7fcaee7d955ea3525e4cb6d17051`.
Review and pin that exact revision before integration, not a moving branch. Updating
the source checkout does not update an installed plugin: publish the approved revision,
refresh/reinstall the plugin, verify its triage files match that revision, and start a new
session before relying on it. Do not claim that a source-only edit is active in the cache.

## Running the CLI

```
$ go run ./cmd/releasebot -message 'v1.37.1-rc1+k3s1 v1.37.1-rc2+rke2r1'            # dry-run (default)
$ go run ./cmd/releasebot -message-file req.txt -skip-jobs -dry-run=false            # workflows only
$ go run ./cmd/releasebot -message-file req.txt -dry-run=false                       # everything
```

| Flag | Default | Meaning |
|---|---|---|
| `-message` / `-message-file` | | Request text |
| `-matrix` | `config/releasebot/matrix.yaml` | Job matrix |
| `-dry-run` | `true` | Print the plan only |
| `-skip-tag-check` | `false` | Do not check tags on GitHub |
| `-skip-workflows` / `-skip-jobs` | `false` | Run only part of the plan |
| `-poll` | `60s` | Jenkins polling interval |
| `-qase-timeout` | `15m` | How long to wait for the Qase runs to appear |

Environment:

| Variable | Used for |
|---|---|
| `GITHUB_TOKEN` | Tag check (optional, avoids rate limits) and `workflow_dispatch` (needs Actions write on this repo) |
| `QASE_AUTOMATION_TOKEN` | Finding the new Qase run ids (required when a job uses `{{QASE_RUN_ID}}`) |
| `JENKINS_<CONTROLLER>_AUTH` | `user:apitoken` per controller in the matrix, e.g. `JENKINS_MOWER_AUTH` |

## Pending

1. **Slack scopes approval.** `groups:read`/`groups:history` (private channel) and
   `app_mentions:read` are waiting for the workspace admin. `groups:write` was also requested
   and is not needed; it can be dropped.
2. **Socket Mode listener** (`cmd/releasebot` `-listen` mode): on `app_mention`, build the plan
   and reply in the thread with the dry-run output; only execute after a ✅ reaction from an
   allowed user. Progress (triggered / started / finished, with build links) goes to the same
   thread, and a summary at the end.
3. **Matrix.** The current list (validate cluster, validate cluster RPM, conformance for each
   product) is a starting set. Implement the agreed child-job execution model above: map batch
   coverage and the parameters each batch passes (with a parity test against the batch logic),
   deduplicate scenarios, expose exclusions in the dry-run, add the bounded policy-based
   smoke retry, and agree the upgrade starting-version rule. Decide `maxConcurrent` per controller with the team, accounting
   for manual/cron load and shared AWS quotas. Baler can be added as a second controller; do
   not add batch parent jobs to the bot matrix.
4. **Failure policy: decisions still open.** Implementation follows the
   [failure-triage and retry contract](#failure-triage-and-retry-policy-planned-bot-integration).
   Agree these before enabling automatic retries:

   - **Rule allowlist:** approve scenario/ref/stage/signature rules and rule IDs, including
     eligible flaky scenarios; reuse the shared Jenkinsfile's deterministic signals where available.
   - **Qase on reruns:** keep the same run, but decide whether the retry replaces the failed
     result or both attempts stay recorded.
   - **Thread format and controls:** agree the compact progress/final summary, build links,
     retry counts and permitted user commands/reactions for a manual rerun.

5. **Merge the workflow change to `main`** before running the bot for real (see "Deployment
   dependency" above), or point `dtfRef` to a branch that has it.
6. **Qase end to end.** The title lookup is verified against existing runs. Still to confirm on
   a real round: the lookup picks up runs right after the workflow creates them, and
   `QASE_RUN_ID` + `REPORT_TO_QASE=true` make the qainfra jobs report into them. Confirm the per-job `QASE_TEST_CASE_ID` defaults are right for each job.
7. **LTS.** `lts=` tags are only passed to release checks; no Jenkins jobs are expanded for them.
8. **Idempotency.** A repeated mention for the same RCs triggers everything again. Keep a small
   state file of (RC set → thread) and ask before re-running.
9. **Tokens for the bot.** A dedicated GitHub token (fine-grained, Actions write on this repo
   only) and a Jenkins service account instead of personal tokens.
10. **Hosting**, see below.

## Hosting

The plan is a small VM on vSphere. Socket Mode opens an outbound WebSocket to Slack, so the VM
needs no inbound port, public IP or ingress. It needs outbound HTTPS to `slack.com`,
`api.github.com`, `api.qase.io` and the Jenkins controllers (which may require the VPN or an internal network).

- Run the binary as a systemd service with `Restart=always`, under a dedicated user.
- Secrets in an `EnvironmentFile` owned by that user, mode `0600` (`SLACK_BOT_TOKEN`,
  `SLACK_APP_TOKEN`, `GITHUB_TOKEN`, `QASE_AUTOMATION_TOKEN`, `JENKINS_*_AUTH`); never in the repo or the matrix file.
- Only one instance may be connected: with two, Slack delivers each event to one of them at
  random.
- Build with `GOOS=linux GOARCH=amd64 go build -o releasebot ./cmd/releasebot` and ship the
  matrix file next to it.
