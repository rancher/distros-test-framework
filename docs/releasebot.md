# Release bot

Turns a release request posted in Slack ("@bot please test v1.37.1-rc1+k3s1 v1.37.1-rc2+rke2r1")
into the three things we do by hand today for every RC round:

1. **Release checks**: dispatch `release-checks.yaml` (GitHub Actions, this repo).
2. **Qase runs**: dispatch `qase-patch-validation-create.yaml`, which creates the patch-validation
   runs for both products (plan ids rke2=14, k3s=20).
3. **Jenkins jobs**: schedule existing child jobs for each requested product/RC, with bounded
   concurrency per controller. Batch jobs remain manual entry points and coverage references;
   the bot does not launch them. Jobs run in phases per RC; a failure is triaged and rerun once
   when a transient-infrastructure rule allows it, otherwise the thread is asked (`retry`/`skip`).
   A parity test against the batch parameters is still to do (see "Execution model").

The bot reacts to a Slack mention (Socket Mode, `-listen`). It does not poll the channel.

## Status

| Piece | State |
|---|---|
| Parsing RC tags from Slack text | Done (`internal/pkg/releasebot/parse.go`) |
| Plan: workflows + Qase RCs + job expansion | Done (`plan.go`) |
| GitHub client: tag check + `workflow_dispatch` | Done (`github.go`) |
| Jenkins client: crumb, `buildWithParameters`, queue → build, result | Done (`jenkins.go`) |
| Scheduler: shared per-controller `maxConcurrent`, phases per RC, `dependsOn` gates, reruns | Done (`scheduler.go`) |
| Parity test of per-job parameters against the batch Jenkinsfiles; OS-validation children | Not done (see "Execution model") |
| Failure triage + reruns | Done: every failure is triaged by the pinned skill through the broker (quick mode, reuse per failure signature); a rerun is granted by the versioned transient rules and can be vetoed by the verdict; at most one; `retry`/`skip`/`triage` in the thread |
| Qase run id → `QASE_RUN_ID` of every job | Done (`qase.go`); title and `search` lookup checked read-only against the real API |
| Job matrix | Five phases, smoke split, upgrade starts, jobs checked on Jenkins (`config/releasebot/matrix.yaml`) |
| CLI with dry-run (`cmd/releasebot`) | Done |
| Unit tests | Done (`releasebot_test.go`: parsing, plan/order/prefix, Qase RC dedupe, Qase title/request-id matching/timeout, scheduler limits, transient and persistent query errors, lost trigger responses, canceled queue items, `dependsOn` release/block/validation, Jenkins trigger error classification over HTTP) |
| Slack app: bot token, Socket Mode, `message.channels` event | Done; live-tested: mention and thread reply both arrive over the socket |
| Slack scopes `groups:read`, `groups:history`, `app_mentions:read` | **Waiting for workspace admin approval** |
| Resume after a restart; builds in unknown state watched instead of blocking new plans | Done (`runrecord.go`, `schedulerwatch.go`, `listener.go`) |
| Socket Mode listener (`-listen`): auto-start, `status`/`stop`, concurrent runs | Done (`internal/pkg/slack`, `listener.go`, `planner.go`); dry-run verified live in #distros-test-reports, not yet run with `-dry-run=false` |
| Hosting (vSphere VM) | Host prepared (`setup.sh prepare`, env file, hardened unit); first `deploy` waits for the reviewed commit |

Verified so far: `go build`, `go vet`, `go test -race`, `golangci-lint` and the `ops/` and
`scripts/` shell tests are clean (`make pre-commit`). A dry-run plan against real tags lists the
workflows, the Qase runs and the Jenkins jobs by phase, RC and controller. **The shipped matrix is
the full one** (five phases, about 35 jobs per RC: the last dry-run for 4 RCs of each product
listed 280 builds), so the first `-dry-run=false` run starts all of it; start with one RC and a low
`maxConcurrent`, or with a reduced matrix.

Output goes through `resources.LogLevel`: the plan, dispatches, job progress and summary are
`info`; warnings, query failures and non-SUCCESS results are `warn`; fatal errors are `error`.
With `LOG_LEVEL=debug`, each job also logs its full expanded parameters.

Nothing has been dispatched or triggered for real yet (`-dry-run=false` has not been run).

## How it works

### Code layout

`cmd/releasebot` contains flags, environment reads, mode selection and dependency wiring.
The application lives in `internal/pkg/releasebot`, split by responsibility within one package:

- `app.go` and `controllers.go`: orchestration, injected client factories and client reuse.
- `plan.go`, `planner.go`, `planoutput.go`: matrix validation, planning and presentation.
- `scheduler.go`, `schedulercommands.go`, `schedulerstatus.go`, `capacity.go`: execution and run controls.
- `triage.go`: the request/verdict contract and rerun gate.
- `triagebroker.go`: broker transport, spool, isolation checks, verdict reuse and the Claude runner.
- `failurelog.go`, `transient.go`, `redact.go`: failure extraction, fixed retry rules and secret redaction.

File names use lowercase joined words; test files retain Go's `_test.go` suffix.
An additional directory would be a separate package, not just a visual grouping. Keep these
files together while they share run and triage contracts; extract a package when it has a clear
API and one-way dependencies. Avoid generic `utils`, `types` or `responses` packages.
Contexts are passed explicitly to operations; clients are reused without a global singleton.

The structure follows [Go's module layout guidance](https://go.dev/doc/modules/layout) and
[package naming guidance](https://go.dev/blog/package-names). Client factories follow the same
dependency-injection approach as [GitHub CLI's Factory](https://github.com/cli/cli/blob/trunk/pkg/cmdutil/factory.go).

### When a job fails, step by step

The whole path of a failed build in a running plan; the sections linked give the details.

1. **The build ends with anything but SUCCESS** (FAILURE, UNSTABLE, ABORTED). The scheduler sees
   it on its next poll and frees the Jenkins slot. From now on the later phases of that RC wait;
   other RCs and the other product carry on.
2. **Can it still be rerun automatically?** No AI involved (`rerunDenied` in
   `schedulercommands.go`). A job already rerun once, or an ABORTED build, goes straight to step 6:
   triage could not change anything. A build whose state is unknown (trigger response lost, status
   unreadable 10 times in a row) has no result yet: it keeps its Jenkins slot, its dependents wait,
   and it is checked every 5 min. When it is seen finishing, its result goes through these steps like
   any other (a failure is triaged); one Jenkins has not answered for in 4 h is given up.
3. **Quick triage** (see [Triage broker](#triage-broker)). Without a running broker
   (`releasebot-triage@<user>`) or with `-triage-spool` unset, this is skipped and the job goes to
   step 6 ("automatic triage unavailable"). Otherwise:
   - the bot downloads the console log (last 8 MiB) and extracts the failing part (secrets
     redacted), a signature, and whether a Ginkgo spec failed;
   - a failure already triaged in the last 24 h reuses that verdict (`triagebroker.go`): an
     infrastructure failure by signature across jobs, RCs and products (one outage, one triage), a
     test failure by job and signature;
   - otherwise the request goes through the spool to the broker, which runs `claude-sandbox` in
     quick mode (Sonnet, the pinned skill, read-only Jenkins tools, prompt on stdin, 5 min limit)
     and returns a verdict `{action, bucket, confidence, summary, evidence}`. The bot waits up to
     `-triage-wait` (1 h). Measured: about $0.13–0.37 and 30 s per failure.
4. **Two keys for an automatic rerun** (see [Failure triage and retry
   policy](#failure-triage-and-retry-policy)): the verdict must agree (`rerun`, bucket exactly
   `INFRA`, confidence at least 80, cited evidence) **and** a deterministic transient rule must
   match the build's main error (EC2 capacity, AWS throttling or 5xx, a lost Jenkins agent, the
   Docker Hub pull limit), with no spec failed. The rule grants; the verdict can only veto.
5. **Automatic rerun.** The job goes back to the queue as attempt 2 with a new hostname prefix,
   and the thread gets "rerunning … (triage: …)". If attempt 2 fails too, step 2 sends it to step 6.
6. **"Needs help" in the Slack thread**, mentioning who started the run, with the job, its result,
   the build link and the triage summary. The job and its RC's later phases wait for a reply:
   - `retry <job> [rc]` runs it again;
   - `skip <job> [rc]` counts it as passed (SKIPPED) and lets the next phase start;
   - `triage <job> [rc]` runs the full triage (default model, with the independent verifier;
     about 7 min and $3) and posts it; the job keeps waiting for `retry` or `skip`;
   - `stop` starts no new job and ends the waiting ones as failed.
7. **End of the run.** The summary lists every job. Failures (SKIPPED is not one) make the run end
   with an error. A run waits for its watched builds before it reports its end; nothing it leaves
   blocks other plans. A bot restart resumes the run in its thread (see [Listen mode](#listen-mode)).

### Parsing

`ParseRequest` finds `vX.Y.Z-rcN+k3sN` and `vX.Y.Z-rcN+rke2rN` anywhere in the text. It undoes
Slack escaping first, so tags inside links (`<url|tag>`), code spans and `%2B`-encoded URLs still
match. Duplicates are dropped.
A message with no tags is rejected.

### Workflows

- `release-checks.yaml` gets `k3s_versions` and `rke2_versions` (comma separated; empty inputs
  are omitted). Its `rke2_lts_versions` input is for manual runs; the bot does not set it.
- `qase-patch-validation-create.yaml` gets `rcs`, one `vX.Y.Z-rcN` per patch version. The script
  creates the runs for both products from each entry, so k3s and rke2 of the same version collapse
  into one. When the products are on different RCs of the same version (k3s rc1, rke2 rc2), the
  higher RC is used and the plan prints a warning.
- Both run on `dtfRef` from the matrix (`qa-infra-RC-1`): GitHub runs the workflow file and the
  checked-out code of that branch. The workflow must also exist on the default branch for the
  dispatch to find it (both do). GitHub returns 204 without a run id, so the bot reports
  "dispatched", not a link.

### Qase run ids

Every Jenkins job reports to the patch-validation run of its product and version, so it needs
that run's id in `QASE_RUN_ID` (for example `1016` from
`https://app.qase.io/run/K3SRKE2/dashboard/1016`). The workflow does not return ids (it only
prints them in the Action log), so the bot finds them in Qase by title plus a request id:

1. Every plan gets a unique request id (`rb-<UTC timestamp>-<8 hex>`, `newRequestID`), sent to
   the Qase workflow as the `request_id` input. The script appends
   ` | Release bot request: <id>` to the description of every run it creates
   (`Version: v1.37.1-rc1 | Release bot request: rb-20260928T135419-17836701`).
2. The title is deterministic and mirrors `scripts/qase-patch-validation.sh`:
   `<RKE2|K3S> <Month> <Year> Patch Validation for vX.Y.Z+<rke2r1|k3s1>` (`qaseRunTitle`).
3. After dispatching, the bot polls `GET /v1/run/K3SRKE2?search=<title>` until, for every title,
   a run exists with that exact title **and** a description ending in its own request marker
   (`waitQaseRuns` / `matchQaseRun`, default timeout 15 min). Runs from any other dispatch
   (manual, or an earlier workflow that finishes late) have no marker or a different one and are
   never taken, even when they are newer.
4. `{{QASE_RUN_ID}}` in the job params is replaced by that id (`applyQaseRunIDs`), and the
   matrix sets `REPORT_TO_QASE=true`. If a job needs an id that was not found, the bot stops
   before triggering any job, so nothing reports to a wrong run.

With `-skip-workflows` (runs created earlier by hand) there is no request id, so the newest run
with the exact title is used.

**Deployment dependency:** the dispatch passes `request_id`, an input that only
`qase-patch-validation-create.yaml` on `qa-infra-RC-1` has (with the matching script change). A
branch without it rejects the dispatch (HTTP 422) before any Jenkins job is triggered.

Coupling with the script to keep in mind:

- The script hardcodes the `rke2r1`/`k3s1` suffix in the title whatever the tag says; the bot
  does the same. If the script changes its title format, `qaseRunTitle` must change with it.
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
plan 14), exactly what `qaseRunTitle` builds. The `search` query the bot sends returns only that
run (1016), and for `K3S September 2026 Patch Validation for v1.35.9+k3s1` only run 1020.

### Jenkins matrix

`config/releasebot/matrix.yaml` lists controllers (URL + `maxConcurrent`) and job templates.
Unknown keys are rejected when the matrix loads, so a typo such as `depends_on` fails instead of
silently dropping a dependency.
Each template runs once per RC of its product. In params:

- `{{VERSION}}` becomes the full tag, and `{{QASE_RUN_ID}}` the Qase run id (see above).
- `{{PREFIX}}` becomes `HOSTNAME_PREFIX`: `<prefixBase><k|r><major><minor><code>`, for example
  `rbr137vc` for `rke2-validate-cluster` on v1.37. qa-infra caps `dsf-<prefix>-<product>-<id>` at
  24 characters, so the prefix has at most 9 (rke2) or 10 (k3s); `loadMatrix` rejects a template
  that could exceed it. The 5-character id is unique per build, so reruns keep the same prefix.
- `{{UPGRADE_FROM}}` becomes the latest GA release of the RC's minor that is older than the RC
  (from the GitHub tags; for a `.0` RC, the previous minor's latest GA, and the plan says so).
  Upgrade jobs set `INSTALL_VERSION: "{{UPGRADE_FROM}}"` and `UPGRADE_VERSION: "{{VERSION}}"`;
  their default `TEST_ARGS` already read `${UPGRADE_VERSION}`.

**Phases.** Every job has a `phase` (1 smoke, 2 install, 3 airgap, 4 upgrade, 5 Rancher). Per
product and RC, a job waits for every job of the previous phase that runs for that RC; other RCs
and products carry on (a failure holds only its own RC). **Split:** jobs with the same `split`
group share the product's RCs, newest first in matrix order: with 4 RCs, `validate-cluster`
(tarball) runs on the 2 newest and `validate-cluster-rpm` on the 2 oldest. `dependsOn` still
works for extra dependencies inside the same product and RC.

**Checked on Jenkins before running, failing closed.** The plan reads each job's parameter list
(`JENKINS_<CONTROLLER>_AUTH`), because Jenkins silently ignores parameters a job does not define:
an upgrade job without `UPGRADE_VERSION` would otherwise test its default versions.

- Jobs marked `optional: true` may be missing on the controller (dual-stack, IPv6-only and the
  Windows jobs have no `_qainfra` job yet): they are listed as skipped, the next phase does not
  wait for them, and they run on their own once the job exists.
- Any other job that is missing, lacks a parameter the bot sets, or whose upgrade start version is
  unknown is listed under "Cannot run", and **the plan is not started** (the dry-run still shows
  it). A required job never silently drops out and lets a later phase start without it.
- If jobs could not be checked at all (no credentials, a timeout, a 5xx), the dry-run warns, but a
  real run is refused.
- Parameters in `optionalParams` (the Qase ones) are dropped with a warning instead of refusing:
  a job that lacks them runs without reporting to Qase.

The expanded plan's dependency graph (phases included) is validated before anything is
dispatched, and `loadMatrix` rejects a `dependsOn` on a job of a later phase. Job `code`s are
required (1 or 2 characters, unique per product), so two jobs never share a hostname prefix.
`rke2-validate-cluster` sets `INSTALL_METHOD: tar`: on SLES 16 the installer would otherwise pick
rpm, and the tarball half of the smoke split would not test the tarball.

Params not set in the matrix keep the job's own defaults, so TFVARS, `NODE_OS` and the hardened
server flags (SELinux, CIS profile on RKE2, protect-kernel-defaults on K3s) come from the job
definitions in JJB rather than being duplicated here. Airgap jobs keep their own `TEST_ARGS`,
as the batch does.

### Execution model

The bot schedules **child jobs**, never the batch parents (`parallel jobs`), so every build counts
against its controller's `maxConcurrent`. Keep the batches for manual use; do not run a batch next
to its children in a bot plan. Implemented (see "Jenkins matrix", "Scheduler" and "Failure triage
and retry policy"):

- phases per product and RC, with the smoke split, and a failure holding only its own RC;
- capacity shared by all runs; queue items and builds in unknown state keep their slot;
- one automatic rerun at most, only when a transient-infrastructure rule matches; everything else
  asks in the thread (`retry`, `skip`, `triage`);
- upgrade start = latest GA of the RC's minor (previous minor for `.0`), shown in the plan.

Not done yet:

- **Parity with the batches.** Each job keeps its own defaults except the params the matrix sets.
  That matches the batches for airgap (`Jenkinsfile_batch_airgap_test` passes no `TEST_ARGS`) and
  for upgrades (their default `TEST_ARGS` read `UPGRADE_VERSION`). A test comparing the bot's
  params with what each batch Jenkinsfile passes, so the two cannot drift, is still to write.
- **OS validation** (`Jenkinsfile_batch_os_validation`: per-child OS, ARM OS, upgrade channel,
  paired version lists) is not in the matrix.

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
  outside `loadMatrix` gets the same protection.
- The shipped matrix gates each phase on the previous one per product and RC. A failed smoke is
  triaged like any job: rerun once when a transient rule allows it, otherwise its RC's later
  phases wait for `retry` or `skip` in the thread.

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

### Failure triage and retry policy

This section is the self-contained contract; it does not require access to a
private skills repository.

**As implemented (decided 2026-09-29, and written into the pinned skill's "Release-bot
boundary" at 0.3.0): two keys, and the deterministic one grants.**

- **The rule grants.** A versioned allowlist of transient infrastructure signatures
  (`internal/pkg/releasebot/transient.go`, each rule with an ID shown in the thread) must match
  the build's *primary* failure: the first error block with its indented continuation lines, in
  the stage that failed first, never cleanup, post actions or the log's end. Lines the log marks
  as not an error (`ignoring`, `failed=0`) do not count; Jenkins' own trailers (`Finished:`,
  "script returned exit code", `Also:`, stack frames) are skipped, but past one only a lost agent
  is promoted, and an exception with its own message is a cause. When a Ginkgo spec failed,
  provisioning had succeeded and no rule applies. The rules: EC2 insufficient capacity, AWS throttling, AWS 5xx, a lost
  Jenkins agent, the Docker Hub pull limit. Ansible `UNREACHABLE` is deliberately not one of
  them (it also covers bad keys, inventory and security groups).
- **The verdict can only veto.** The quick triage verdict counts as agreement only as
  `action: rerun` with bucket exactly `INFRA`, confidence of at least 80 and at least one
  non-blank evidence line. Any other
  answer, or a missing, late, malformed or failed verdict, blocks the rerun; it never grants one.

At most **one** automatic rerun per job, RC and request, and never for an aborted build, a
build or trigger in unknown state, or a controller marked unreachable. No verdict changes capacity or releases a
dependency on its own: a rerun has to pass, or a person replies `skip`. Everything else is a
person's decision in the thread (`retry`/`skip`).

The points below were written for a stricter design (deterministic rules only) and remain the
guidance for the skill's analysis and for extending the rules:

- **Deterministic rules can narrow the rerun further.** Inputs are the confirmed Jenkins
  terminal result, failed stage, scoped log signals, scenario/ref and retry count, with
  explicit, versioned rule IDs and positive and negative fixture tests. Default to no automatic
  retry if evidence is missing or rules conflict.
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
scenario/product/RC/request, counting it against `maxConcurrent`. The rerun keeps its
`HOSTNAME_PREFIX`: qa-infra names resources after each build's own `QA_INFRA_RUN_ID`, so the
rerun gets new resource names, and leaked resources of the first attempt stay tellable apart
by that id. Unknown trigger/build
state, manual abort, build timeout, test assertion failure or unclassified failure must not auto-retry.
Persistent credential/quota/configuration errors require remediation, not blind retries.
These safety denials take precedence over any flaky-scenario allowlist match.
Dependents remain blocked until an eligible retry succeeds; other products/RCs continue.

The policy can run independently of LLM availability; a missing verifier does not turn into
permission to retry and does not veto an already eligible deterministic decision. The thread
should distinguish the policy's rule ID/evidence/attempt from the LLM's diagnosis/verdict.
Include a Bot action field only when a policy decision was supplied; omit it in manual triage.
Human-requested reruns remain a separate authorized action, not an LLM policy override.

### Triage broker

The bot user has no Claude login and no Jenkins read credential. A separate broker does the
triage: `releasebot -triage-broker`, run by `releasebot-triage@<user>.service` as the user who
owns the claude-sandbox and its Vertex login (`fmoral` on the bot VM).

- **Cost.** Every failure gets a *quick* triage: the bot reads the console log itself, extracts
  the failing part and a signature (failed tests and reasons, IDs and numbers normalized), and
  sends the excerpt; the broker runs the skill on Sonnet without the independent verifier. A
  failure already triaged within 24 h reuses the first verdict instead of asking again: an
  infrastructure failure (no spec failed) by signature alone, so one outage that fails many jobs,
  RCs or both products is triaged once; a test failure by job name and signature (across RCs of
  that job); the reuse cache lives in
  memory and starts empty after a restart. The signature normalizes only volatile values (ids,
  addresses, times, durations): HTTP codes, exit statuses and expected values stay, and a log
  with only Jenkins' generic trailers gets no signature (never reused). A failure that can never
  be rerun is not triaged at all, and one person's `triage` runs at most one full analysis per
  job at a time. Measured on two real builds, quick mode gave the same decision as the full
  triage for $0.13 to $0.37 and about 30 s, instead of about $3 and 7 min; 40 distinct failures
  would be about $5 to $15, less with reuse. Both real builds were `ask` cases: the rerun path
  is covered by tests, not yet by a real transient failure. The *full* triage (default model,
  verifier, deeper explanation) runs only when someone replies `triage <job>`.
- **Exchange.** Bot and broker share `/var/lib/releasebot-triage` through the
  `releasebot-triage` group: the bot writes `in/<id>.json` (job, RC, result, build URL,
  attempt), the broker moves it to `work/`, triages it and writes `out/<id>.json`; every file is
  written under a temporary name and renamed. The broker holds `.broker.lock`; when no broker
  holds it the bot asks a person at once instead of waiting. The bot waits up to
  `-triage-wait` (1 h, requests queue: one triage at a time) and the broker gives each triage
  `-triage-timeout` (20 min).
- **The call.** In its own workspace (`~<user>/releasebot-triage`, with its own sandbox state
  and the jenkins-mower MCP), the broker runs `claude-sandbox -p` with `--json-schema` (the
  verdict must be `{action: rerun|ask, bucket: INFRA|JOB|TEST-CODE|PRODUCT|UNKNOWN, confidence,
  summary, evidence}`), the pinned skill
  through `--plugin-dir`, `--no-session-persistence` and `--permission-mode dontAsk`, which
  refuses tools that are not listed: `Skill`, `Read`/`Glob`/`Grep` scoped to
  `//workspace/skill/**`, `Agent` in full mode (the verifier), and the read-only jenkins-mower
  tools (`getBuild`, `getBuildLog`, `searchBuildLog`, `getTestResults`, `getFlakyFailures`,
  `getBuildChangeSets`, `getJob`). This is not a closed file boundary: Claude Code lets the
  model read its working directory without approval whatever the allow list says, so the
  workspace holds only the pinned skill and `private/`, and the real credential boundary is the
  explicit deny below. The MCP server comes only from
  `--strict-mcp-config --mcp-config /workspace/private/mcp.json` (owner-only,
  `-triage-mcp-config`), and `Read(//workspace/private/**)` is denied (deny rules win and cover
  Grep and Glob):
  verified live, a read of the file returns nothing, a recursive Grep or Glob of the workspace
  never lists it, and reads of the sandbox's Google login or state outside the workspace are
  refused. Shell, edits, web access and the Jenkins tools that trigger, replay, rebuild or
  update builds are also denied explicitly. The Jenkins credential in `mcp.json` must be a
  **read-only** account: the deny list stops the model from calling the mutating tools, but a
  token that cannot trigger builds is what makes a prompt-injected log harmless. Until that
  account exists, the MCP config holds a personal token (see "Pending").
- **Robustness.** Verdicts are checked against the full contract on both sides of the spool
  (required fields present and typed, bounds, no extra fields, nothing after the object);
  anything else asks. The spool root is not group-writable (`setup.sh prepare`: root 0750,
  `in/`/`out/` root-owned 2770, `work/` the broker's own 0700, all in the spool's group), so
  neither side, broker included, can swap a directory for a symlink. Before taking
  `.broker.lock` the broker checks that exact layout (modes, root as owner, group, no links) and
  the lock itself (a single-link regular 0640 file it owns, in the spool's group so the bot can
  probe it, opened
  without following links, never re-moded), and refuses to start otherwise. `prepare` removes
  links and a foreign or hard-linked lock left by an older layout instead of keeping them, and
  refuses entries that are not directories. Spool files are created
  exclusively under random names and read without following symlinks. A canceled
  or timed-out triage sends SIGTERM to the wrapper's whole process group (the docker client
  passes it to the container; the wrapper's deferred trap then runs), then stops the container
  by name (`claude-sandbox-<pid>`) and kills the group; verified live, nothing is left running.
  Console logs are streamed to their real end (last 8 MiB); a log over 512 MiB is refused
  rather than cut short. A broker that crashes mid-triage leaves the request in `work/`;
  the next broker puts it back in `in/`; a broker stopped mid-triage leaves the request there
  on purpose (no verdict), so the next one answers it while the bot still waits. When the bot stops waiting (`stop`, shutdown or
  `-triage-wait`) for a request the broker already claimed, it leaves `in/<id>.cancel` and the
  broker cancels that triage (and its container) instead of letting it run to its timeout. The
  prompt, with the log excerpt (redacted like the summaries), goes to `claude-sandbox` on stdin,
  never on its command line, where `ps` would show it. Quick triage has its own limit
  (`-triage-quick-timeout`, 5 min) so a queue of them fits the bot's `-triage-wait`. The broker
  unit runs with `UMask=0077` and the kernel, clock, hostname and SUID restrictions; the sandbox
  was checked working under them (`systemd-run` with the same properties). `setup.sh deploy` restarts a running broker with the
  bot (and again on rollback), so both always run the same release. Error texts are scrubbed of
  credential-looking strings like summaries. The prompt tells the model to treat the log as
  data; a manipulated log cannot produce a rerun unless it also carries a transient-rule
  signature, and then at most one; summaries are scrubbed of anything that looks like a
  credential before they are posted.
- **Pinned skill.** The skill runs from `~<user>/releasebot-triage/skill`, a copy of
  `.claude-plugin/plugin.json` and `skills/jenkins-failure-triage` at the reviewed revision
  (`68e530f`, plugin 0.3.0). `ops/releasebot/triage-skill.sha256` lists every file's SHA-256;
  the broker refuses to start, and every triage answers `ask`, if the directory differs in any
  way (changed, extra, missing or symlinked file). `releasebot -triage-build <url>` (with the
  broker flags, `-triage-full` for the full mode) triages one build by hand and prints the
  verdict with its cost. Updating the skill = review the new revision, copy it
  there (`git archive <rev> .claude-plugin/plugin.json skills/jenkins-failure-triage`), and
  deploy a DTF commit with the new manifest.

## Running the CLI

Code layout: `cmd/releasebot` only reads flags and the environment, builds the clients and picks
the mode (`main.go`, `options.go`, `wiring.go`, `listen.go`, `broker.go`). Everything else is in
`internal/pkg/releasebot`: the use cases (`app.go`: plan a request and execute it; `planner.go`
for Slack; `planoutput.go`) and the domain (matrix, plan, scheduler, listener, triage);
`internal/pkg/slack` is the Slack client.

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
| `-listen` | `false` | Serve requests from Slack instead of `-message` (see below) |
| `-state-file` | `<user config dir>/releasebot/state.json` | Listen mode: every run in progress (stage and jobs), saved at each change and resumed after a restart |
| `-validate` | `false` | Load and check `-matrix`, then exit (`setup.sh deploy` runs it on the candidate) |
| `-triage-spool` | | Listen mode: ask the triage broker through this spool (empty: every failure asks a person); broker mode: the spool to serve |
| `-triage-wait` | `1h` | Listen mode: how long a failure waits for a verdict |
| `-triage-broker` | `false` | Serve triage requests from `-triage-spool` with claude-sandbox |
| `-triage-timeout` / `-triage-quick-timeout` | `20m` / `5m` | Broker mode: limit for one full / quick triage |
| `-triage-cmd` | `claude-sandbox` | Broker mode: the sandbox command |
| `-triage-workspace` | `~/releasebot-triage` | Broker mode: workspace the sandbox mounts |
| `-triage-skill-dir` / `-triage-manifest` | `<workspace>/skill` / | The pinned skill and the manifest it must match |
| `-triage-mcp-config` | `<workspace>/private/mcp.json` | The jenkins-mower MCP config (0600, in a 0700 `private/`) |
| `-triage-quick-model` | `sonnet` | Model for quick triage |
| `-triage-build <url>` / `-triage-full` | | Triage one build now (no spool) and print the verdict and the decision as JSON |

Environment:

| Variable | Used for |
|---|---|
| `GITHUB_TOKEN` | Tag check (optional, avoids rate limits) and `workflow_dispatch` (needs Actions write on this repo) |
| `QASE_AUTOMATION_TOKEN` | Finding the new Qase run ids (required when a job uses `{{QASE_RUN_ID}}`) |
| `JENKINS_<CONTROLLER>_AUTH` | `user:apitoken` per controller in the matrix, e.g. `JENKINS_MOWER_AUTH` |
| `SLACK_BOT_TOKEN` | Listen mode: bot token (`xoxb`, needs `channels:history` and `chat:write`) |
| `SLACK_APP_TOKEN` | Listen mode: app-level token (`xapp`, `connections:write`) for Socket Mode |
| `RELEASEBOT_CHANNELS` | Listen mode: comma-separated channel ids to serve, e.g. `C07Q8H55F6Z` (#distros-test-reports); the older `RELEASEBOT_CHANNEL` still works |
| `RELEASEBOT_OPEN_CHANNELS` | Listen mode: channels (also served) where anyone may start and steer runs |
| `RELEASEBOT_ALLOWED_USERS` | Listen mode: comma-separated Slack user ids who may run plans in the other channels |
| `RELEASEBOT_DRY_RUN` | systemd unit only: passed as `-dry-run`; `true` (default) or `false` in `/etc/releasebot/releasebot.env` |
| `RELEASEBOT_DTF_REF` | Overrides the matrix's `dtfRef` (the workflow ref and every job's `BRANCH`, `{{DTF_REF}}`); set it to `main` once `qa-infra-RC-1` merges |

### Listen mode

`go run ./cmd/releasebot -listen` keeps a Socket Mode connection open and serves the channels in
`RELEASEBOT_CHANNELS`. `-dry-run` (the default) also applies here: every request only gets the
plan. Start with `-listen -dry-run=false` to run them.

1. Someone posts `@distros-test-reports v1.37.1-rc2+rke2r1 v1.37.1-rc2+k3s1` (any text with RC tags).
2. The bot builds the plan (matrix re-read, tags and jobs checked) and replies **in that thread**.
   If the sender may run plans there (listed in `RELEASEBOT_ALLOWED_USERS`, or any user in a
   channel of `RELEASEBOT_OPEN_CHANNELS`), the reply says "Started by ...": **no confirmation**,
   phase 1 starts at once and every RC moves through the phases on its own. Anyone else gets the
   plan only.
3. Progress (dispatches, Qase run ids, triggered, started and finished jobs, summary) is posted in
   the same thread, grouped into one message every 10 s. In the thread:
   - `status`: where each RC is (phase, running, waiting, passed, failed); anyone may ask.
   - `stop`: no new job starts (checked before every trigger); builds already running are followed
     to their result, and jobs waiting for help end as failed.
   - `retry <job> [rc]` / `skip <job> [rc]`: answer a "Needs help" message (below). The RC is only
     needed when several waiting jobs share the name.
   - `triage <job> [rc]`: run the full triage (with the independent verifier) of a waiting job and
     post it; the job keeps waiting for `retry` or `skip`.

   **When a job fails.** A build that ends with anything but SUCCESS goes to triage, holding no
   Jenkins slot; later phases of its RC wait meanwhile (other RCs carry on). Triage may ask for
   one automatic rerun, which also needs the build's log to match a transient-infrastructure rule
   (see "Failure triage and retry policy"). A failed rerun or an aborted build can never be rerun
   automatically, so those skip triage altogether. Triage is cheap by design (see "Triage
   broker"): the bot reads the build log, and a failure already triaged reuses that verdict (an
   infrastructure failure across jobs, one outage failing many jobs; a test failure within its job).
   Otherwise, and for a rejected trigger or a queue item canceled on Jenkins, the bot posts
   "Needs help" in the thread, mentioning who started the run, with the build URL and the triage
   summary. `retry` runs the job again; `skip` counts it as passed (SKIPPED in the summary) and
   lets the next phase start. Until then the run keeps waiting. Builds in unknown state are not
   retried or skipped: they may still be running, so they are watched (step 5).
   Without a running broker (or with `-triage-spool` unset), triage is skipped and every
   failure asks at once.
4. Several runs can go at once (a new RC mid-release does not wait for the others); they share
   each controller's `maxConcurrent`. RC tags that another run is already validating are left out
   with a pointer to that run's thread, and the plan (workflows, Qase runs, jobs) is rebuilt for
   the remaining tags; only when every tag is already running is nothing started.
5. **Builds in unknown state** (trigger response lost, or status unreadable 10 times in a row)
   may still be running, so they keep their Jenkins slot and have no result yet: their dependents
   wait. The run checks them every 5 minutes. When one is seen finishing, its slot is freed, the
   thread is told, and its result is handled like any other (a success lets the next phase run, a
   failure is triaged). A build Jenkins still reports as running is never given up; one Jenkins
   has not answered for in 4 hours (or a lost trigger with no URL to check) is: its slot is freed
   and it fails its dependents. `status` counts them as "in unknown state (watched)". Nothing
   blocks other plans: a run that fails or stops part-way says so in its thread (asking again may
   dispatch workflows or create Qase runs again, so check first).
6. **Restarts.** Every run is saved in `-state-file` before it starts and at each change: its
   stage (dispatching, dispatched, jobs) and each job's state (pending, triggering, queued,
   running with its URLs, unknown, triaging, waiting for help, done). When the bot starts again
   (a deploy, a crash, even SIGKILL), it resumes each saved run in its own thread: queued and
   running builds are followed from their URLs and take their slots back, triage cut by the
   restart is redone, jobs waiting for help are posted again, finished jobs are not run again,
   and later phases carry on. Resumed runs first take back the slots of their builds still on
   Jenkins, all of them, before any triggers something new (they wait for each other at most 2
   minutes; the bot starts serving Slack once they are ready). A job that was queued when the bot
   stopped is found by its queue id among the job's builds if Jenkins has forgotten the queue item
   meanwhile. A trigger the restart cut is treated as a build in unknown state (it may exist),
   never triggered again. A job is only triggered after the state saying so is written: while the
   state cannot be saved (full disk), nothing is triggered and the thread says why. `stop` is saved
   at once, so a stopped run stays stopped after a restart. A run saved while its workflows were
   being dispatched cannot tell what started: its thread is told and it is not resumed. A run
   whose resume cannot be set up (missing Jenkins credentials, an unreadable matrix, no Qase runs)
   stays saved: its builds on Jenkins keep their slots, its RC tags stay taken, and its thread says
   why; the next start resumes it once the cause is fixed. On
   shutdown a run stays saved and says so in its thread; one that ended by itself at that moment is
   over. A bot started with `-dry-run` (the default) resumes nothing. The state file has a format
   version; a bot refuses a newer one instead of misreading it.
   Each post has its own 2-minute timeout, so progress and the outcome are posted even for long
   runs and on shutdown.

The bot ignores channels it does not serve, bot messages, edits and thread replies other than
`status`, `stop`, `retry`, `skip` and `triage`. A reply sent with "Also send to channel"
(`thread_broadcast`) counts.

Reliability details:

- The socket reader only acknowledges envelopes and queues message events (bounded queue, 256);
  one worker handles them in order, so slow work never delays acknowledgements. Building a plan
  (Jenkins and GitHub checks) runs beside that worker, in 2 planning workers with room for 4 more
  requests and within 3 minutes each, so `stop`, `status`, `retry` and `skip` are never queued
  behind one; a mention beyond that is answered "busy" at once instead of piling up. A controller or GitHub that
  fails a check is not asked again in that plan: an unreachable Jenkins costs one timeout, and the
  plan is refused as unchecked.
- The connect and websocket handshake have a timeout (30 s) and honor shutdown. The connection is
  dropped and re-opened after 3 minutes without any incoming bytes (pings included), so a
  half-open connection cannot leave the bot deaf. Slack's `disconnect` refresh reconnects
  immediately; errors reconnect with backoff up to 1 minute.
- Permanent Slack errors (`invalid_auth`, `token_revoked`, wrong token type, missing scope) stop
  the bot with an error instead of retrying forever, so the service manager sees the failure.
- Posts are spaced at least 1 s apart. HTTP 429 responses are retried after the full
  `Retry-After` (up to 3 times, within a 3-minute budget per call); a wait beyond the budget fails
  the call at once instead of retrying early, following Slack's rate-limit contract. The client
  remembers each method's Retry-After deadline, so later calls wait for it too, even after a call gave up.
- The Socket Mode URL carries a connection ticket, so it is redacted from connection errors
  before they are logged.

**Why plain message events:** the approved scopes are `channels:history` and `chat:write`;
`app_mentions:read` and `reactions:read` are waiting for the workspace admins. So the bot
subscribes to `message.channels` and recognizes the mention by `<@bot-id>` in the text, and the
thread commands are plain replies. Both were verified live on #distros-test-reports. When the
scopes are approved this can move to `app_mention`.

## Pending

1. **Slack scopes approval.** `groups:read`/`groups:history` (private channel) and
   `app_mentions:read` are waiting for the workspace admin. `groups:write` was also requested
   and is not needed; it can be dropped.
2. **First real execution.** The dry-run listener was verified live on #distros-test-reports
   (2026-09-29: mention -> plan in the thread in ~1 s). The shipped matrix is the full one: start
   `-listen -dry-run=false` with one RC and a low `maxConcurrent` (or a reduced matrix), then widen.
3. **Matrix.** The batch parity test and OS validation (see "Execution model"). Decide
   `maxConcurrent` per controller with the team, accounting for manual/cron load and shared AWS
   quotas. Baler can be added as a second controller; do not add batch parent jobs to the matrix.
   Flip `dtfRef` to `main` (or set `RELEASEBOT_DTF_REF`) when `qa-infra-RC-1` merges: until then
   every dispatch and job runs from that branch, and once it is deleted every run fails.
4. **Failure policy: decisions still open.** Implementation follows the
   [failure-triage and retry contract](#failure-triage-and-retry-policy).
   Still to agree:

   - **Rule allowlist:** approve scenario/ref/stage/signature rules and rule IDs, including
     eligible flaky scenarios; reuse the shared Jenkinsfile's deterministic signals where available.
   - **Qase on reruns:** keep the same run, but decide whether the retry replaces the failed
     result or both attempts stay recorded.
   - **Thread format and controls:** agree the compact progress/final summary, build links,
     retry counts and permitted user commands/reactions for a manual rerun.

5. **Qase end to end.** The title lookup is verified against existing runs. Still to confirm on
   a real round: the lookup picks up runs right after the workflow creates them, and
   `QASE_RUN_ID` + `REPORT_TO_QASE=true` make the qainfra jobs report into them. Confirm the per-job `QASE_TEST_CASE_ID` defaults are right for each job.
6. **Rerun poller on the shared Slack client.** The reporter (`internal/pkg/qase`) and the bot use
   `internal/pkg/slack`; `cmd/rerunpoller` still has its own `postToSlack` and history reads.
7. **Tokens for the bot.** A dedicated GitHub token (fine-grained, Actions write on this repo
   only) and a Jenkins service account instead of personal tokens. The triage MCP config needs a
   **read-only** Jenkins account: today the tool deny list is the only barrier between the model
   (which reads build logs anyone's build can print) and a token that can trigger builds.
8. **Hosting**, see below.

## Hosting

The bot runs as a long-lived systemd service on a vSphere VM (dyn-3-241). Socket Mode keeps an
outbound WebSocket to Slack open, so Slack pushes each mention within about a second; there is
no polling and the VM needs no inbound port or public IP. It needs outbound HTTPS to `slack.com`,
`api.github.com`, `api.qase.io` and the Jenkins controllers.

Files in `ops/releasebot/`:

| File | Purpose |
|---|---|
| `releasebot.service` | Hardened unit: runs `-listen` as user `releasebot`, dry-run unless the env file sets `RELEASEBOT_DRY_RUN=false` |
| `releasebot-triage@.service` | The triage broker, run as the user who owns the claude-sandbox (`releasebot-triage@fmoral`) |
| `triage-skill.sha256` | Manifest of the pinned triage skill; `deploy` ships it with each release |
| `releasebot.env.example` | Template for `/etc/releasebot/releasebot.env` |
| `setup.sh` | `prepare [triage-user]` (users, directories, spool, units, env file) and `deploy <binary> <matrix> <sha>` |
| `setup_test.sh` | Tests `deploy` with stubbed `go` and `systemctl` (and `prepare`, as root) |

How the host is set up:

- **Dedicated account.** `releasebot` is a system user with no login shell, no home, no extra
  groups (no sudo, no docker) and no SSH keys; it cannot read other users' secrets.
- **Least credentials.** Dry-run needs only `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` (tag checks on
  the public repos work without a GitHub token). Add `GITHUB_TOKEN` (Actions write on this repo
  only), `QASE_AUTOMATION_TOKEN` (read runs) and `JENKINS_*_AUTH` (a service account) only when
  enabling real runs.
- **Secrets.** `/etc/releasebot/releasebot.env` is `root:root 0600`; systemd reads it before
  switching to `releasebot`, which cannot read the file itself. `prepare` refuses a link and closes
  an existing file to `root:root 0600` (keeping its content); `deploy` refuses to run otherwise. `LoadCredential=` would be
  stronger but needs the bot to read credential files; not done yet.
- **Reviewed binary, read-only config.** `setup.sh deploy` checks the VCS stamp Go embeds in the
  binary (`go version -m`): it must be exactly the given commit, built from a clean tree. The
  binary and its matrix are staged, the candidate validates the matrix (`-validate`, as the bot
  user), and only then are they moved to `/usr/local/lib/releasebot/<sha>-<hash>/` (the hash
  covers binary and matrix) and `current` is switched, all root-owned. Existing releases are never
  overwritten: an identical redeploy reuses its directory, and a matrix change on the same commit
  gets a new one. `deploy` also installs both units shipped next to `setup.sh`, runs
  `daemon-reload`, enables and restarts the bot and restarts the brokers that were running; if a
  restart fails or the bot or a broker does not stay up (for example a skill that no longer matches
  the new manifest), `current` and the units go back to the previous version, and `deploy` reports
  whether that one is running; a failed first deploy leaves the units stopped and disabled. The execution mode lives in the env
  file (`RELEASEBOT_DRY_RUN`), which `deploy` never touches, so it survives deploys. Updates are a
  new deploy of a reviewed commit, never `git pull` of a moving branch on the host. The bot writes
  only `/var/lib/releasebot`.
- **Hardened unit.** `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, `ProtectHome`,
  `UMask=0077`, `StateDirectory`, kernel/namespace/address-family restrictions and a system-call
  filter. `systemd-analyze security` rates it 1.7 (OK) on systemd 254; the binary was checked
  under the same restrictions with `systemd-run`.
- **One instance.** `-listen` takes an exclusive `flock` on `<state-file>.lock` and refuses to
  start if another bot holds it. Slack would split events between two connected instances, so
  development uses a separate Slack app, never the production tokens.
- **Failures.** The bot exits on any Slack error other than the documented transient ones
  (`ratelimited`, `internal_error`, `service_unavailable`, ...), and reconnects only for those and
  network errors. `Restart=on-failure` every 30 s, up to 5 times in 10 minutes; after that the unit
  stays failed (for example with a revoked token) instead of looping. Monitoring that does not
  depend on the bot's own Slack token is still to be added.

Preparing a new host (once; `deploy` refuses to run until this is done):

```
$ scp ops/releasebot/{setup.sh,releasebot.service,releasebot-triage@.service,releasebot.env.example} \
    <host>:/root/releasebot-deploy/
# on the host, as root (fmoral: the user who owns the claude-sandbox and its Vertex login):
$ bash /root/releasebot-deploy/setup.sh prepare fmoral  # users, dirs, triage group and spool, units, env
$ vi /etc/releasebot/releasebot.env                     # Slack tokens, channel, allowlist (root:root 0600)
# once, as fmoral: the triage workspace (its own sandbox state) with the jenkins-mower MCP and the
# pinned skill (git archive <rev> .claude-plugin/plugin.json skills/jenkins-failure-triage)
$ mkdir -p ~/releasebot-triage/skill && tar -xf skill-<rev>.tar -C ~/releasebot-triage/skill
$ install -d -m 0700 ~/releasebot-triage/private     # the MCP config with the Jenkins credential
$ (umask 077; printf '{"mcpServers":{"jenkins-mower":{"type":"http","url":"https://mower.jenkins.qa.rancher.space/mcp-server/mcp","headers":{"Authorization":"Basic %s"}}}}' \
    "$(printf %s "$USER_COLON_TOKEN" | base64 -w0)" > ~/releasebot-triage/private/mcp.json)
# after the first deploy, as root:
$ systemctl enable --now releasebot-triage@fmoral
```

Deploying a commit:

```
$ git worktree add /tmp/releasebot-<sha> <sha>          # clean checkout of the reviewed commit
$ cd /tmp/releasebot-<sha> && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o releasebot ./cmd/releasebot
$ scp releasebot config/releasebot/matrix.yaml \
    ops/releasebot/{setup.sh,releasebot.service,releasebot-triage@.service,triage-skill.sha256} \
    <host>:/root/releasebot-deploy/
# on the host, as root:
$ bash /root/releasebot-deploy/setup.sh deploy /root/releasebot-deploy/releasebot \
    /root/releasebot-deploy/matrix.yaml <sha>
```

Logs: `journalctl -u releasebot -f`; status: `systemctl status releasebot`; deployed commit and
release: `/usr/local/lib/releasebot/current-commit` and `current-release`. Old releases stay for
rollback; remove them by hand (`current` must not point at them).
