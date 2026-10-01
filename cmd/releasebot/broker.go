package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// claudeTriage is how the broker (and -triage-build) runs the skill.
func claudeTriage(o *options) (*releasebot.ClaudeTriage, error) {
	if o.triageManifest == "" {
		return nil, errors.New("triage needs -triage-manifest")
	}
	if o.triageSkillDir == "" {
		o.triageSkillDir = filepath.Join(o.triageWorkspace, "skill")
	}
	if err := releasebot.VerifySkill(o.triageSkillDir, o.triageManifest); err != nil {
		return nil, err
	}
	if o.triageMCP == "" {
		o.triageMCP = filepath.Join(o.triageWorkspace, "private", "mcp.json")
	}
	if err := releasebot.VerifyPrivate(o.triageMCP); err != nil {
		return nil, err
	}

	return &releasebot.ClaudeTriage{
		Command: o.triageCmd, Workspace: o.triageWorkspace, SkillDir: o.triageSkillDir,
		Manifest: o.triageManifest, Timeout: o.triageTimeout, QuickTimeout: o.triageQuickTimeout, QuickModel: o.triageModel,
		MCPConfig: o.triageMCP,
	}, nil
}

// runTriageBuild triages one build directly (no spool) and prints the verdict as JSON.
func runTriageBuild(o *options) error {
	ct, err := claudeTriage(o)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	req := &releasebot.TriageRequest{Mode: releasebot.TriageQuick, BuildURL: o.triageBuild, Attempt: 1}
	if o.triageFull {
		req.Mode = releasebot.TriageFull
	}

	j, err := newApp(o).JenkinsForBuild(o.triageBuild)
	if err != nil {
		return err
	}

	log, err := j.ConsoleText(ctx, o.triageBuild)
	if err != nil {
		return err
	}

	fail := releasebot.AnalyzeFailure(log)
	req.Excerpt = fail.Excerpt
	resources.LogLevel("info", "signature %q, excerpt %d chars, test failed %v", fail.Signature,
		len(req.Excerpt), fail.TestFailed)

	v, err := ct.Triage(ctx, req)
	if err != nil {
		return err
	}

	// What the bot would do: the verdict can only veto; a transient rule has to grant the rerun.
	d := releasebot.GateRerun(v.Decision(), &fail)
	rule := ""
	if r := releasebot.MatchTransient(&fail); r != nil {
		rule = r.ID
	}

	// The result is data (stdout, pipeable to jq); progress stays in the log.
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(map[string]any{
		"verdict":  v,
		"decision": map[string]any{"rerun": d.Rerun, "summary": d.Summary, "rule": rule},
	})
}

// runBroker answers the bot's triage requests one at a time until SIGINT/SIGTERM.
func runBroker(o *options) error {
	if o.triageSpool == "" {
		return errors.New("broker mode needs -triage-spool")
	}
	ct, err := claudeTriage(o)
	if err != nil {
		return err
	}

	release, err := releasebot.AcquireBrokerLock(o.triageSpool)
	if err != nil {
		return err
	}
	defer release()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	resources.LogLevel("info", "triage broker serving %s (skill %s)", o.triageSpool, o.triageSkillDir)
	b := &releasebot.Broker{Spool: o.triageSpool, Triage: ct.Triage, Poll: 5 * time.Second, Logf: resources.LogLevel}

	return b.Serve(ctx)
}
