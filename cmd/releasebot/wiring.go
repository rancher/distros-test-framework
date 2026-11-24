package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// newApp builds the application from the flags and the environment (tokens, controller credentials).
func newApp(o *options) *releasebot.App {
	return releasebot.New(&releasebot.Config{
		MatrixPath:    o.matrixPath,
		DTFRef:        os.Getenv(releasebot.DTFRefEnv),
		SkipTagCheck:  o.skipTagCheck,
		SkipWorkflows: o.skipWorkflows,
		SkipJobs:      o.skipJobs,
		Poll:          o.poll,
		QaseTimeout:   o.qaseTimeout,
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		JenkinsAuth:   jenkinsAuth,
		ControllerURL: controllerURL,
		Log:           resources.LogLevel,
	})
}

// jenkinsAuth reads JENKINS_<CONTROLLER>_AUTH as "user:apitoken".
func jenkinsAuth(controller string) (user, token string, ok bool) {
	return strings.Cut(os.Getenv("JENKINS_"+strings.ToUpper(controller)+"_AUTH"), ":")
}

// controllerURL reads JENKINS_<CONTROLLER>_URL; empty keeps the matrix URL.
func controllerURL(controller string) string {
	return os.Getenv("JENKINS_" + strings.ToUpper(controller) + "_URL")
}

// run is the one-shot CLI mode: plan the request in -message or -message-file, then execute it
// unless -dry-run.
func run(o *options) error {
	if o.messageFile != "" {
		raw, err := os.ReadFile(o.messageFile)
		if err != nil {
			return err
		}
		o.message = string(raw)
	}

	if !o.dryRun {
		release, claimErr := claimExecution(o.stateFile)
		if claimErr != nil {
			return claimErr
		}
		defer release()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := newApp(o)
	p, err := app.Prepare(ctx, releasebot.ParseRequest(o.message))
	if err != nil {
		return err
	}
	mode := "EXECUTE"
	if o.dryRun {
		mode = "DRY-RUN"
	}
	app.PrintPlan(p.Plan, mode)
	if o.dryRun {
		return nil
	}

	return app.Execute(ctx, p, nil)
}

// claimExecution takes the -listen lock and refuses while the Slack bot has saved runs: their builds
// may still run, so this run would exceed the controllers' capacity and test the same RCs twice.
func claimExecution(stateFile string) (release func(), err error) {
	release, lockErr := releasebot.AcquireInstanceLock(stateFile + ".lock")
	if lockErr != nil {
		return nil, fmt.Errorf("not running (stop the Slack bot or the other CLI run first): %w", lockErr)
	}

	saved := &releasebot.Listener{StatePath: stateFile}
	if restoreErr := saved.Restore(); restoreErr != nil {
		release()
		return nil, fmt.Errorf("read the Slack bot's saved runs: %w", restoreErr)
	}

	if n := saved.SavedRuns(); n > 0 {
		release()
		return nil, fmt.Errorf("not running: the Slack bot left %d unfinished runs in %s; start releasebot.service "+
			"so it resumes them, and run this once they end", n, stateFile)
	}

	return release, nil
}

// validateMatrix loads the matrix the way a request would, so a deploy can reject a bad candidate.
func validateMatrix(o *options) error {
	m, err := releasebot.LoadMatrixWithRef(o.matrixPath, os.Getenv(releasebot.DTFRefEnv))
	if err != nil {
		return err
	}
	resources.LogLevel("info", "matrix %s ok: %d jobs, %d controllers", o.matrixPath, len(m.Jobs), len(m.Controller))

	return nil
}
