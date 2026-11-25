package main

import (
	"flag"
	"os"
	"path/filepath"
	"time"
)

type options struct {
	message       string
	messageFile   string
	matrixPath    string
	dryRun        bool
	skipTagCheck  bool
	skipWorkflows bool
	skipJobs      bool
	poll          time.Duration
	qaseTimeout   time.Duration
	listen        bool
	resume        bool
	bumps         bool
	bumpsConfig   string
	bumpsDir      string
	bumpsPost     string
	bumpsClose    bool
	month         string
	stateFile     string
	validate      bool

	// Triage broker: the spool shared with the bot, and how the broker runs the skill.
	triageBroker       bool
	triageLocal        bool
	triageSpool        string
	triageWait         time.Duration
	triageTimeout      time.Duration
	triageQuickTimeout time.Duration
	triageCmd          string
	triageWorkspace    string
	triageSkillDir     string
	triageManifest     string
	triageModel        string
	triageFullModel    string
	triageMCP          string
	triageBuild        string
	triageFull         bool
}

// parseFlags reads the command line; the triage flags are in triageFlags.
func parseFlags() *options {
	var o options
	flag.StringVar(&o.message, "message", "", "release request text (as posted in Slack)")
	flag.StringVar(&o.messageFile, "message-file", "", "read the release request text from a file")
	flag.StringVar(&o.matrixPath, "matrix", "config/releasebot/matrix.yaml", "job matrix file")
	flag.BoolVar(&o.dryRun, "dry-run", true,
		"print the plan without dispatching or triggering anything (in -listen mode: requests only get the plan)")
	flag.BoolVar(&o.skipTagCheck, "skip-tag-check", false, "do not verify that the RC tags exist on GitHub")
	flag.BoolVar(&o.skipWorkflows, "skip-workflows", false, "do not dispatch the GitHub workflows")
	flag.BoolVar(&o.skipJobs, "skip-jobs", false, "do not trigger the Jenkins jobs")
	flag.DurationVar(&o.poll, "poll", 60*time.Second, "Jenkins polling interval")
	flag.DurationVar(&o.qaseTimeout, "qase-timeout", 15*time.Minute, "how long to wait for the Qase runs to appear")
	flag.BoolVar(&o.listen, "listen", false, "serve release requests from Slack (Socket Mode) instead of -message")
	flag.BoolVar(&o.bumps, "bumps", false,
		"plan (and with -dry-run=false run) the month's version-bump issues of the K3s and RKE2 boards")
	flag.StringVar(&o.bumpsConfig, "bumps-config", "config/releasebot/bumps.yaml", "bump mode: jobs and default images")
	flag.StringVar(&o.bumpsDir, "bumps-dir", "", "bump mode: where the MDs go (default ~/releasebot/bumps/<month>)")
	flag.StringVar(&o.bumpsPost, "bumps-post", "",
		"bump mode: post this MD (each section on its issue, a link on its backports), after a yes")
	flag.BoolVar(&o.bumpsClose, "bumps-close", false, "with -bumps-post: also close the issues")
	flag.StringVar(&o.month, "month", time.Now().Format("2006-01"), "bump mode: the release month the MDs are named after")
	flag.BoolVar(&o.resume, "resume", false,
		"carry on the run a CLI execution saved (interrupted, killed or rebooted) instead of -message; with -dry-run=false")
	flag.StringVar(&o.stateFile, "state-file", defaultStateFile(),
		"where runs in progress survive restarts (the CLI keeps its run in cli-run.json next to it)")
	flag.BoolVar(&o.validate, "validate", false, "load and check the -matrix file, then exit (used before deploying)")
	triageFlags(&o)
	flag.Parse()

	return &o
}

func triageFlags(o *options) {
	home, _ := os.UserHomeDir()
	flag.StringVar(&o.triageSpool, "triage-spool", "",
		"listen mode: ask the triage broker through this spool directory (empty: every failure asks a person); "+
			"broker mode: the spool to serve")
	flag.DurationVar(&o.triageWait, "triage-wait", time.Hour,
		"how long a failure waits for a verdict (listen mode) or for the triage slot (-triage-local)")
	flag.BoolVar(&o.triageBroker, "triage-broker", false, "serve triage requests from -triage-spool with claude-sandbox")
	flag.BoolVar(&o.triageLocal, "triage-local", false,
		"CLI execution: triage failed builds in this process with claude-sandbox (needs the broker flags below)")
	flag.DurationVar(&o.triageTimeout, "triage-timeout", 20*time.Minute, "broker mode: limit for one full triage")
	flag.DurationVar(&o.triageQuickTimeout, "triage-quick-timeout", 5*time.Minute,
		"broker mode: limit for one quick triage (12 fit in the default -triage-wait)")
	flag.StringVar(&o.triageCmd, "triage-cmd", "claude-sandbox", "broker mode: the claude-sandbox command")
	flag.StringVar(&o.triageWorkspace, "triage-workspace", filepath.Join(home, "releasebot-triage"),
		"broker mode: workspace the sandbox mounts (holds the pinned skill)")
	flag.StringVar(&o.triageSkillDir, "triage-skill-dir", "",
		"broker mode: pinned jenkins-failure-triage skill (default <workspace>/skill)")
	flag.StringVar(&o.triageManifest, "triage-manifest", "",
		"broker mode: sha256 manifest the skill must match (ops/releasebot/triage-skill.sha256)")
	flag.StringVar(&o.triageMCP, "triage-mcp-config", "",
		"broker mode: jenkins-mower MCP config with its credential, owner-only (default <workspace>/private/mcp.json)")
	flag.StringVar(&o.triageModel, "triage-quick-model", "sonnet",
		"broker mode: model for quick triage (the rerun decision)")
	flag.StringVar(&o.triageFullModel, "triage-full-model", "opus",
		"broker mode: model for full triage (empty keeps the sandbox's default)")
	flag.StringVar(&o.triageBuild, "triage-build", "",
		"triage one failed build now and print the verdict (as the broker would; needs the broker flags)")
	flag.BoolVar(&o.triageFull, "triage-full", false, "with -triage-build: full triage instead of quick")
}
