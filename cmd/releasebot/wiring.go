package main

import (
	"context"
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
		Log:           resources.LogLevel,
	})
}

// jenkinsAuth reads JENKINS_<CONTROLLER>_AUTH as "user:apitoken".
func jenkinsAuth(controller string) (user, token string, ok bool) {
	return strings.Cut(os.Getenv("JENKINS_"+strings.ToUpper(controller)+"_AUTH"), ":")
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

// validateMatrix loads the matrix the way a request would, so a deploy can reject a bad candidate.
func validateMatrix(o *options) error {
	m, err := releasebot.LoadMatrixWithRef(o.matrixPath, os.Getenv(releasebot.DTFRefEnv))
	if err != nil {
		return err
	}
	resources.LogLevel("info", "matrix %s ok: %d jobs, %d controllers", o.matrixPath, len(m.Jobs), len(m.Controller))

	return nil
}
