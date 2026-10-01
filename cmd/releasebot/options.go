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
	stateFile     string
	validate      bool

	// Triage broker: the spool shared with the bot, and how the broker runs the skill.
	triageBroker       bool
	triageSpool        string
	triageWait         time.Duration
	triageTimeout      time.Duration
	triageQuickTimeout time.Duration
	triageCmd          string
	triageWorkspace    string
	triageSkillDir     string
	triageManifest     string
	triageModel        string
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
	flag.StringVar(&o.stateFile, "state-file", defaultStateFile(),
		"listen mode: where the block state and in-progress marker survive restarts")
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
	flag.DurationVar(&o.triageWait, "triage-wait", time.Hour, "listen mode: how long a failure waits for a verdict")
	flag.BoolVar(&o.triageBroker, "triage-broker", false, "serve triage requests from -triage-spool with claude-sandbox")
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
		"broker mode: model for quick triage (the rerun decision); full triage keeps the default model")
	flag.StringVar(&o.triageBuild, "triage-build", "",
		"triage one failed build now and print the verdict (as the broker would; needs the broker flags)")
	flag.BoolVar(&o.triageFull, "triage-full", false, "with -triage-build: full triage instead of quick")
}
