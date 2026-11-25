package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// newApp builds the application from the flags and the environment (tokens, controller credentials).
func newApp(o *options) *releasebot.App {
	return releasebot.New(&releasebot.Config{
		MatrixPath:    o.matrixPath,
		DTFRef:        os.Getenv(releasebot.DTFRefEnv),
		SUSELogin:     os.Getenv(releasebot.SUSELoginEnv),
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

// run is the CLI mode: plan the request in -message or -message-file, then execute it unless
// -dry-run, or carry on a saved run (-resume). An execution is driven from the terminal like a Slack
// thread: progress is saved at each change, and status, stop, retry, skip and triage are typed in.
func run(o *options) error {
	if o.messageFile != "" {
		raw, err := os.ReadFile(o.messageFile)
		if err != nil {
			return err
		}
		o.message = string(raw)
	}

	if o.dryRun {
		if o.resume {
			return errors.New("-resume carries a run on: it needs -dry-run=false")
		}

		return planOnly(o)
	}

	release, claimErr := claimExecution(o.stateFile, cliStatePath(o.stateFile))
	if claimErr != nil {
		return claimErr
	}
	defer release()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return executeInTerminal(ctx, o, newApp(o))
}

// executeInTerminal carries the saved run on (-resume) or starts the request, refusing a new one
// while a run is saved: its builds may still be running.
func executeInTerminal(ctx context.Context, o *options, app *releasebot.App) error {
	failures, triageErr := localTriage(ctx, o, app)
	if triageErr != nil {
		return triageErr
	}

	terminal := &releasebot.TerminalRun{StatePath: cliStatePath(o.stateFile), Out: os.Stdout, User: os.Getenv("USER")}
	if isTerminal(os.Stdin) {
		terminal.In = os.Stdin
	}
	saved, savedErr := terminal.Saved()
	if savedErr != nil {
		return savedErr
	}

	switch {
	case o.resume && saved == nil:
		return fmt.Errorf("-resume: no saved run in %s", terminal.StatePath)
	case o.resume:
		return terminal.Resume(ctx, app, saved, failures)
	case saved != nil:
		return fmt.Errorf("not running: %q (started %s UTC) is saved in %s and its builds may still run; carry "+
			"it on with -resume, or remove the file once they have ended", saved.Request,
			saved.Started.Format("2006-01-02 15:04"), terminal.StatePath)
	}

	p, err := app.Prepare(ctx, releasebot.ParseRequest(o.message))
	if err != nil {
		return err
	}
	app.PrintPlan(p.Plan, "EXECUTE")

	return terminal.Execute(ctx, app, p, o.message, failures)
}

// planOnly prints the plan, triggering nothing.
func planOnly(o *options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := newApp(o)
	p, err := app.Prepare(ctx, releasebot.ParseRequest(o.message))
	if err != nil {
		return err
	}
	app.PrintPlan(p.Plan, "DRY-RUN")

	return nil
}

// localTriage is the -triage-local triage, or nil (every failure then asks the person at the terminal).
func localTriage(ctx context.Context, o *options, app *releasebot.App) (*releasebot.FailureTriage, error) {
	if !o.triageLocal {
		return nil, nil
	}

	ct, err := claudeTriage(o)
	if err != nil {
		return nil, fmt.Errorf("-triage-local: %w", err)
	}
	logIdentity(ctx, o.triageCmd)

	return app.LocalFailureTriage(ct.Triage, o.triageWait), nil
}

// cliStatePath keeps the CLI's run next to the listener's state, under the same lock.
func cliStatePath(stateFile string) string {
	return filepath.Join(filepath.Dir(stateFile), "cli-run.json")
}

// bumpsStatePath keeps the bump run there too.
func bumpsStatePath(stateFile string) string {
	return filepath.Join(filepath.Dir(stateFile), "bumps-run.json")
}

// refuseSavedTerminalRuns refuses while a saved CLI or bump run may still have builds; own is the state
// the caller resumes itself, "" refuses both.
func refuseSavedTerminalRuns(stateFile, own string) error {
	for _, mode := range []struct{ path, name, resume string }{
		{cliStatePath(stateFile), "CLI run", "releasebot -resume -dry-run=false"},
		{bumpsStatePath(stateFile), "bump run", "releasebot -bumps -resume -dry-run=false"},
	} {
		if mode.path == own {
			continue
		}

		saved, savedErr := (&releasebot.TerminalRun{StatePath: mode.path}).Saved()
		if savedErr != nil {
			return savedErr
		}
		if saved != nil {
			return fmt.Errorf("the %s %q (started %s UTC) is saved in %s and its builds may still run; carry it "+
				"on with %s, or remove the file once they ended", mode.name, saved.Request,
				saved.Started.Format("2006-01-02 15:04"), mode.path, mode.resume)
		}
	}

	return nil
}

// isTerminal: commands are read only from a person at a terminal (not /dev/null, also a character
// device); elsewhere a failed job just fails.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// claimExecution takes the -listen lock and refuses while the Slack bot, or a CLI or bump run other than
// own, has saved runs: their builds may still run and would exceed the controllers' capacity.
func claimExecution(stateFile, own string) (release func(), err error) {
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

	if refuseErr := refuseSavedTerminalRuns(stateFile, own); refuseErr != nil {
		release()
		return nil, fmt.Errorf("not running: %w", refuseErr)
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
