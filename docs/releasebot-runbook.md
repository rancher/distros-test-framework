# Release bot runbook (QA captain)

How a QA captain sets up their laptop and runs the release bot on it: a patch release (the RCs'
workflows, Qase runs and Jenkins jobs) and the month's version-bump issues. This page is the
step-by-step; [`releasebot.md`](releasebot.md) is the reference (how it works, every flag, the
design). When the two disagree, `releasebot.md` wins.

There are two ways to drive a run, and both use the same CLI on the same laptop:

- **A. In your terminal.** You run `rb` (the bot on your pinned release) and type its commands.
- **B. With the agent.** The `releasebot-captain-setup` skill (`rancher/distros-qa-skills`, plugin
  0.6.0 and later) sets the laptop up and runs the CLI for you with `:runrelease` and
  `:runbumps`. It triggers nothing until you say go to the exact plan it showed you.

The bot runs on the captain's laptop (v1). The VM and Slack hosting (v2) is parked; see
"Hosting" in `releasebot.md`.

## What you need

| What | From |
|---|---|
| GitHub token: Actions write on `rancher/distros-test-framework`; for bumps, `read:project` and commenting on and closing issues in `k3s-io/k3s` and `rancher/rke2` (closing needs your triage role there) | GitHub settings |
| Mower `user:apitoken` (Baler too, for `baler` requests) | Jenkins → your user → Security |
| Qase automation token (project `K3SRKE2`) | team vault |
| Your SUSE login (e.g. `fmoral`): every `HOSTNAME_PREFIX` starts with its first 4 letters | you |
| The team CLZ AWS account onboarded for Bedrock (only for `-triage-local`) | the CLZ owner |
| macOS or Linux, `git`, `go` (the version in `go.mod`), `python3`, Rancher Desktop | |
| Clones of `distros-test-framework` and `distros-qa-skills` | GitHub |

## 1. Set up the laptop (once per captain)

**With the agent (recommended).** In Claude Code with the `distros-qa-skills` plugin, ask "set up
the release bot". The skill walks through every step below, keeps the tokens out of the chat (you
type each one at a hidden prompt), and ends with `bin/check.sh` saying `READY` and a dry-run.

**By hand:**

1. **Rancher Desktop**, not Docker Desktop. Preferences → Container Engine → `dockerd (moby)`,
   quit Docker Desktop, then `docker context use rancher-desktop && docker info`.
2. **claude-sandbox** (for `-triage-local`): `bash skills/claude-sandbox/install.sh` from
   `distros-qa-skills`, then the Bedrock sign-in (the CLZ owner gives you the `configure sso` line
   and the `--id`):
   ```
   export CLAUDE_SANDBOX_PROVIDER=bedrock
   claude-sandbox bedrock configure sso --no-browser <the CLZ owner's line>   # first time only
   claude-sandbox bedrock setup --id <id>                                     # first time only
   claude-sandbox bedrock doctor                                              # every line ok
   ```
3. **The env file** `~/.config/releasebot/releasebot.env` (directory 0700, file 0600), with your
   own values:

   | Key | Value |
   |---|---|
   | `GITHUB_TOKEN` | the GitHub token |
   | `JENKINS_MOWER_AUTH` | `user:apitoken` for Mower |
   | `JENKINS_BALER_AUTH` | `user:apitoken` for Baler (only for `baler` requests) |
   | `QASE_AUTOMATION_TOKEN` | the Qase token |
   | `RELEASEBOT_SUSE_LOGIN` | your SUSE login; nothing is planned without it |
   | `CLAUDE_SANDBOX_PROVIDER` | `bedrock`, so `-triage-local` never falls back to Vertex |

   No Slack keys on a laptop (`SLACK_*`, `RELEASEBOT_CHANNELS`, `RELEASEBOT_ALLOWED_USERS`): the
   skill's `credentials.py` refuses a file that has them. On macOS the skill keeps the tokens in
   the Keychain and writes this file from it (`bin/sync-env.sh`).
4. **Triage workspace** `~/releasebot-triage` (only for `-triage-local`): the triage skill at the
   revision the release's manifest pins, and the Mower MCP config. The skill installs both
   (`bin/install-triage-skill.sh ~/releasebot/current/triage-skill.sha256` and `bin/credentials.py mcp`); the
   bot refuses a skill that differs from the manifest in any file.
5. **`rb`**, the command you run the bot with, from the DTF clone (any directory on your `PATH`):
   ```
   install -m 0755 ops/releasebot/rb ~/.local/bin/rb
   ln -sfn ~/suse/distros-qa-skills/skills/releasebot-captain-setup/bin/credentials.py ~/.config/releasebot/credentials.py
   ```
   It runs the pinned release (`~/releasebot/current`, step 2), and reads the env file only through
   the skill's `credentials.py run` (owner-only file, known keys, never run as shell, inherited
   credentials dropped). `rb` alone lists its commands; it expects the DTF clone in
   `~/suse/distros-test-framework` (`RELEASEBOT_DTF_DIR` for another place).

## 2. Pin the release (once per release commit)

Everything a run reads comes from one read-only folder built from the reviewed DTF commit, so a
checkout or an edit in the DTF tree never changes the plan mid-release:

```
rb pin <reviewed ref>
```

The ref is any name the DTF clone knows (`upstream/releasebot-qainfra-rc1` after a `git fetch`, a
SHA). It builds `~/releasebot/<sha7>/` (the binary, `matrix.yaml`, `bumps.yaml` and
`triage-skill.sha256` of that commit), validates the matrix, makes the files read-only and points
`~/releasebot/current` at it: every `rb` command runs that folder from then on. A folder that
exists already is only validated and made current again; one that fails half-way is removed. Never
edit a pinned folder; a new commit gets a new folder. The skill does the same ("new release
commit").

## 3. Check before release day

- With the skill: "check", or `bash bin/check.sh ~/releasebot/current` from its folder. It checks the tools, the
  Docker engine, the pinned folder, the env file (modes and keys), the triage skill and MCP file,
  a saved run, and each token live against GitHub, Mower, Baler, Qase and Bedrock. Fix every
  `FAIL` until it says `READY`.
- By hand, a dry-run with the release's real tags. It checks the tags on GitHub and each job and
  its parameters on Jenkins, and triggers nothing:
  ```
  rb plan v1.37.2-rc2+k3s1 v1.37.2-rc2+rke2r1
  ```
  The tokens load in `rb`'s own process, so your terminal never holds them. A warning "not
  checked on jenkins ... is not set" means a missing `JENKINS_<CONTROLLER>_AUTH`.

## 4. Release day

### A. In your terminal

```
CLAUDE_SANDBOX_PROVIDER=bedrock claude-sandbox bedrock login --no-browser      # the session lasts 8 hours
rb run v1.37.2-rc2+k3s1 v1.37.2-rc2+rke2r1
```

Add `baler` after the tags to send every job that has a Baler path in the matrix to Baler. `rb run`
keeps a Mac awake (`caffeinate -i`) and triages failures on the laptop (`-triage-local`).
A release runs from the matrix's `dtfRef`: `rb` warns (also before bumps) when the env file sets
`RELEASEBOT_DTF_REF`, as every job would run from that branch; `rb dtf-ref ""` clears it.

Type these in the same terminal while it runs:

| Command | Does |
|---|---|
| `status` | Where every RC and job is |
| `retry <job> [rc]` | Runs a failed job again |
| `skip <job> [rc]` | Counts a failed job as passed, so its RC's next phases go on |
| `triage <job> [rc]` | A full analysis of a failed build (reads only) |
| `stop` | Ends the plan for good: pending jobs never start, also after a resume |

- **`NEEDS HELP`** (with the terminal bell): a job failed and the triage did not rerun it. Its RC's
  later phases wait for your `retry` or `skip`; other RCs go on.
- **Ctrl-C** pauses: the run is saved in `cli-run.json` (exit 130), and builds already started keep
  running on Jenkins. `rb resume` carries it on without triggering anything twice. A crash or a
  reboot works the same way.
- **Bedrock session expired** (exit 77 for the broker; in a CLI run, failures say so and ask you):
  run `claude-sandbox bedrock login --no-browser`; the next failure is triaged again, no restart.
- **The end** prints a summary of every job and how the plan ended.

### B. With the agent

In Claude Code with the skill: `:runrelease <tags> [baler]`, for example
`:runrelease v1.37.2-rc2+k3s1 v1.37.2-rc2+rke2r1`.

1. It runs `check.sh` (must be `READY`) and checks that no run is live or saved. A saved run is
   carried on with `-resume`, never replaced.
2. It shows the dry-run plan: RCs, jobs per controller and phase, skipped jobs and warnings.
3. It asks "Start it? It triggers <N> jobs on <controllers>, the workflows and Qase runs". Only a
   clear yes to that plan starts it.
4. It starts the CLI in a tmux session; `tmux attach -t rb-release` lets you watch it (detach with
   `Ctrl-b d`; `Ctrl-C` there pauses).
5. Every ~15 minutes and when you ask, it reports status. On `NEEDS HELP` it tells you the job,
   the failure, the triage verdict and your choices, and sends `retry`, `skip` or `stop` only on
   your word. "Pause" saves the run (Ctrl-C).
6. At the end it reports what passed, failed or was skipped, and the Qase run ids.

## 5. Bump issues

The month's version-bump issues of the K3s (`k3s-io` project 6) and RKE2 (`rancher` project 31)
boards: what the bot validates and how is in "Bump issues" in `releasebot.md`. Bumps never report
to Qase; their results go on their own issues.

### A. In your terminal

1. **The plan** (reads GitHub only):
   ```
   rb bumps-plan
   ```
   It lists each job (product, branch, version, its issues and checks), every main issue with
   what happens to each backport (`validate` or `person`, and "not in To Test" for backports
   found outside the column), the bumps for a person with the reason, and the other To Test
   issues. `rb bumps-plan rke2` (or `k3s`) reads one board only.
2. **The run:** `rb bumps` (or `rb bumps rke2`). It prints the plan again and asks
   `Validate these <n> issues with <m> jobs? Type yes to start:`. Nothing starts without `yes`.
   During the run, the terminal commands and Ctrl-C work as in a release; `rb resume-bumps`
   carries a paused one on.
3. **The MDs:** at the end, `~/releasebot/bumps/<month>/<product>-bumps-release-<month>.md` (a
   later run of the month writes `-2`, `-3`), one section per main issue. Jobs that did not pass are listed as "Not in the MDs". Read the file:
   it is what goes on the public issues, redacted, and the bot refuses one that would show a
   Jenkins link or an infrastructure detail.
4. **Posting**, only after reading the MD:
   ```
   rb post ~/releasebot/bumps/2026-11/rke2-bumps-release-2026-11.md
   ```
   It lists each section, counts the backports validated on a commit build only (run them again on
   the RC), and asks `yes`. It comments the section on the main issue and "Validated
   here: <link>" on each backport. `close` after the file also closes the validated backports; the
   main issue closes only once no backport of it is open on GitHub. `<md>.posted.json` records
   each step, so a rerun after a failure never posts twice.

### B. With the agent

`:runbumps [<YYYY-MM>] [k3s|rke2]`, for example `:runbumps 2026-11 rke2`. Same flow as
`:runrelease`: check, the plan with every issue, your go, the tmux session `rb-bumps`. It answers
the bot's `Type yes` only if the issues and jobs match the plan you accepted, then follows the run
and reports the MD paths and "Not in the MDs". It posts only when you say "post" (or "post and
close"), after listing each section, and answers the post prompt only if it lists the same issues.

## 6. Handing a release over

One captain runs a release at a time. The current captain presses **Ctrl-C** (not `stop`) and
sends the saved run to the next one through an approved private channel:

| OS | Saved release run | Saved bump run |
|---|---|---|
| macOS | `~/Library/Application Support/releasebot/cli-run.json` | `.../bumps-run.json` and `bumps-run-plan.json` |
| Linux | `~/.config/releasebot/cli-run.json` | `.../bumps-run.json` and `bumps-run-plan.json` |

The next captain, set up for the same release SHA (`rb pin <sha>`), puts the file in the same place
and runs `rb resume` (`rb resume-bumps` for a bump run). Never run it on two laptops at once, and
never copy env files, Keychain items, MCP files or AWS volumes between captains.

## 7. When something goes wrong

| You see | Why | Do |
|---|---|---|
| `not running: "<request>" ... is saved in .../cli-run.json` | A run was interrupted and its builds may still run | Carry it on with `rb resume`, or remove the file once its builds ended |
| `not running: a bump run is saved in .../bumps-run.json` | Same, for bumps (a saved bump run also blocks a release run, and the reverse) | `rb resume-bumps`, or remove it once the builds ended |
| `not running: the last bump run ended without its MDs` | Reading the consoles or writing the MDs failed after the run | `rb resume-bumps` writes them again |
| `not running (stop the Slack bot or the other CLI run first)` | Another bot holds the lock | Only one run per laptop |
| `RELEASEBOT_SUSE_LOGIN="": set it to your SUSE login` | No login in the env file | Add `RELEASEBOT_SUSE_LOGIN=<login>`, or with the skill `python3 -I bin/credentials.py login <login>` |
| `not checked on jenkins ... is not set` | A controller's credential is missing | Add its `JENKINS_<CONTROLLER>_AUTH` |
| Failures say the model is not reachable | The Bedrock session expired | `claude-sandbox bedrock login --no-browser` |
| `bump MD would show a jenkins link or a private infrastructure detail` | Something unsafe reached the MD (also after your edits) | Remove it from the MD; never post it by hand |
| A K3s commit install fails: `requires jq`, `No provider of '+jq'`, or a missing token | The node lacked `jq`, `unzip` or the `GITHUB_TOKEN` credential | See "Known issues". `No provider of '+jq'` was seen once on SLES while other builds on the same AMI installed it: `retry` |

## 8. Known issues (2026-11)

- **K3s commit installs** need the `GITHUB_TOKEN` credential on Mower (configured), the DTF
  Jenkinsfile that binds it and rancher/qa-infra-automation#269. Until the bot PR
  (rancher/distros-test-framework#369) merges into `qa-infra-RC-1`, run K3s commit bumps between
  `rb dtf-ref releasebot-qainfra-rc1` and `rb dtf-ref ""`: it sets the branch of every run, releases
  included, and that branch can lag `qa-infra-RC-1`. `k3s-version-bump_qainfra` points at those branches meanwhile
  (rancherlabs/jenkins-job-builder#1354, revert after the merges).
- **RKE2 commit installs** on RHEL hung at `Start rke2-server`: a tarball install skipped the
  `rke2-selinux` policy. Fixed in rancher/qa-infra-automation#269; `rke2-version-bump_qainfra`
  points at it meanwhile (rancherlabs/jenkins-job-builder#1355, revert after the merge).

## Where things live

| What | Where |
|---|---|
| Pinned release | `~/releasebot/<sha7>/` (`releasebot`, `matrix.yaml`, `bumps.yaml`, `triage-skill.sha256`); `~/releasebot/current` is the one `rb` runs |
| `rb` | `~/.local/bin/rb`, from `ops/releasebot/rb`; it reads the env through `~/.config/releasebot/credentials.py` (a link to the skill's) |
| Tokens and settings | `~/.config/releasebot/releasebot.env` (0600) |
| Saved runs and lock | `~/Library/Application Support/releasebot/` (macOS), `~/.config/releasebot/` (Linux) |
| Bump MDs and post ledgers | `~/releasebot/bumps/<month>/` |
| Triage workspace | `~/releasebot-triage/` (`skill/`, `private/mcp.json`) |
| The skill | `distros-qa-skills/skills/releasebot-captain-setup` (`bin/check.sh`, `bin/credentials.py`, `bin/live-run.sh`) |
