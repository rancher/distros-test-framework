package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
)

// runBumps is the bump mode: plan the boards' bump issues (dry-run), or, once the captain accepts the
// list at the terminal, run one version-bump job per product and branch and write the MDs. Posting
// is a separate, explicit step (-bumps-post).
func runBumps(o *options) error {
	if o.bumpsPost != "" {
		return postBumps(o)
	}

	cfg, cfgErr := releasebot.LoadBumpConfig(o.bumpsConfig)
	if cfgErr != nil {
		return cfgErr
	}
	matrix, matrixErr := releasebot.LoadMatrixWithRef(o.matrixPath, os.Getenv(releasebot.DTFRefEnv))
	if matrixErr != nil {
		return matrixErr
	}
	if _, ok := matrix.Controller[cfg.Controller]; !ok {
		return fmt.Errorf("bump controller %q is not in the matrix %s", cfg.Controller, o.matrixPath)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if o.dryRun {
		if o.resume {
			return errors.New("-resume carries a run on: it needs -dry-run=false")
		}
		app := newApp(o)
		plan, planErr := app.PlanBumps(ctx, cfg, o.month)
		if planErr != nil {
			return planErr
		}
		app.PrintBumpPlan(plan, cfg, matrix.DTFRef, "DRY-RUN")

		return nil
	}

	o.skipWorkflows = true // bumps dispatch no workflow and report to no Qase run
	release, claimErr := claimExecution(o.stateFile)
	if claimErr != nil {
		return claimErr
	}
	defer release()

	return executeBumps(ctx, o, cfg, matrix.DTFRef)
}

func executeBumps(ctx context.Context, o *options, cfg *releasebot.BumpConfig, dtfRef string) error {
	app := newApp(o)
	runFile := filepath.Join(filepath.Dir(o.stateFile), "bumps-run-plan.json")
	terminal, in, terminalErr := bumpTerminal(o, app, cfg)
	if terminalErr != nil {
		return terminalErr
	}
	failures, triageErr := localTriage(ctx, o, app)
	if triageErr != nil {
		return triageErr
	}

	saved, savedErr := terminal.Saved()
	if savedErr != nil {
		return savedErr
	}
	if o.resume {
		return resumeBumps(ctx, o, app, terminal, saved, runFile, failures)
	}
	if saved != nil {
		return fmt.Errorf("not running: a bump run is saved in %s and its builds may still run; carry it on "+
			"with -resume, or remove the file once they have ended", terminal.StatePath)
	}

	plan, planErr := app.PlanBumps(ctx, cfg, o.month)
	if planErr != nil {
		return planErr
	}
	app.PrintBumpPlan(plan, cfg, dtfRef, "EXECUTE")
	if len(plan.Targets) == 0 {
		return errors.New("no bump to validate")
	}
	if acceptErr := acceptBumps(plan, in, terminal.In != nil); acceptErr != nil {
		return acceptErr
	}

	jobs, jobsErr := releasebot.BumpJobs(plan, cfg, dtfRef)
	if jobsErr != nil {
		return jobsErr
	}
	run := &releasebot.BumpRun{Plan: plan, Jobs: jobs}
	if saveErr := releasebot.SaveBumpRun(runFile, run); saveErr != nil {
		return saveErr
	}
	p, prepareErr := app.PrepareJobs(jobs)
	if prepareErr != nil {
		return prepareErr
	}

	runErr := terminal.Execute(ctx, app, p, "bumps "+o.month, failures)

	return reportBumps(ctx, o, app, run, terminal, runErr)
}

// bumpTerminal is the terminal run of the bump jobs, saved beside the state file and limited by the
// controller's other builds, with the captain's input (terminal.In is nil when not at a terminal).
func bumpTerminal(
	o *options, app *releasebot.App, cfg *releasebot.BumpConfig,
) (*releasebot.TerminalRun, *bufio.Reader, error) {
	terminal := &releasebot.TerminalRun{StatePath: filepath.Join(filepath.Dir(o.stateFile), "bumps-run.json"),
		Out: os.Stdout, User: os.Getenv("USER")}
	in := bufio.NewReader(os.Stdin)
	if isTerminal(os.Stdin) {
		terminal.In = in
	}

	limit, limitErr := app.ControllerBusyLimit(cfg.Controller, 4)
	if limitErr != nil {
		return nil, nil, limitErr
	}
	terminal.LimitFor = limit

	return terminal, in, nil
}

// resumeBumps carries on the saved bump run with the jobs saved in runFile, then reports it.
func resumeBumps(
	ctx context.Context, o *options, app *releasebot.App, terminal *releasebot.TerminalRun,
	saved *releasebot.SavedTerminalRun, runFile string, failures *releasebot.FailureTriage,
) error {
	if saved == nil {
		return fmt.Errorf("-resume: no saved bump run in %s", terminal.StatePath)
	}

	run, loadErr := releasebot.LoadBumpRun(runFile)
	if loadErr != nil {
		return loadErr
	}
	runErr := terminal.Resume(ctx, app, saved, failures)

	return reportBumps(ctx, o, app, run, terminal, runErr)
}

// acceptBumps shows the issues the run validates and waits for the captain's "yes": nothing starts
// without a person at the terminal.
func acceptBumps(plan *releasebot.BumpPlan, in *bufio.Reader, atTerminal bool) error {
	if !atTerminal {
		return errors.New("-bumps -dry-run=false needs a person at a terminal to accept the bump issues")
	}

	fmt.Println("\nBump issues to validate:")
	issues := 0
	for _, g := range plan.Groups {
		var branches []string
		for _, bp := range g.Backports {
			for _, t := range plan.Targets {
				if t.Product == g.Product && t.Branch == bp.Branch && hasIssue(t, bp.Number) {
					branches = append(branches, fmt.Sprintf("#%d %s", bp.Number, bp.Branch))
					issues++
				}
			}
		}
		if len(branches) > 0 {
			fmt.Printf("  %s#%d %s: %s\n", g.Main.Repo, g.Main.Number, g.Main.Title, strings.Join(branches, ", "))
		}
	}

	fmt.Printf("Validate these %d issues with %d jobs? Type yes to start: ", issues, len(plan.Targets))
	answer, readErr := in.ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if strings.TrimSpace(answer) != "yes" {
		return errors.New("not started: the bump issues were not accepted")
	}

	return nil
}

func hasIssue(t *releasebot.BumpTarget, issue int) bool {
	for _, c := range t.Checks {
		if c.Issue == issue {
			return true
		}
	}

	return false
}

// reportBumps writes the MDs of the passed jobs once the run ended (an interrupted run reports when
// resumed) and tells the captain where they are and how to post them.
func reportBumps(
	ctx context.Context, o *options, app *releasebot.App, run *releasebot.BumpRun, terminal *releasebot.TerminalRun,
	runErr error,
) error {
	if errors.Is(runErr, releasebot.ErrInterrupted) {
		return runErr
	}

	progress := terminal.Last()
	results, notPassed, resultsErr := app.BumpResults(ctx, run, &progress)
	if resultsErr != nil {
		return errors.Join(runErr, resultsErr)
	}
	dir := o.bumpsDir
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "releasebot", "bumps", run.Plan.Month)
	}
	files, writeErr := releasebot.WriteBumpMDs(dir, run.Plan, results)
	if writeErr != nil {
		return errors.Join(runErr, writeErr)
	}

	for _, n := range notPassed {
		fmt.Printf("Not in the MDs (the job did not pass): %s\n", n)
	}
	for _, f := range files {
		fmt.Printf("MD written: %s\nRead it, then post it with: releasebot -bumps-post %s [-bumps-close]\n", f, f)
	}
	if len(files) == 0 {
		fmt.Println("No MD written: no bump job passed.")
	}

	return runErr
}

// postBumps posts a bump MD the captain read, after a last "yes" at the terminal.
func postBumps(o *options) error {
	raw, readErr := os.ReadFile(o.bumpsPost)
	if readErr != nil {
		return readErr
	}
	sections, parseErr := releasebot.ParseBumpMD(string(raw))
	if parseErr != nil {
		return parseErr
	}
	if !isTerminal(os.Stdin) {
		return errors.New("-bumps-post needs a person at a terminal to confirm")
	}

	what := "post"
	if o.bumpsClose {
		what = "post and close"
	}
	for _, s := range sections {
		fmt.Printf("  %s#%d (backports %v)\n", s.Repo, s.Issue, s.Backports)
	}
	fmt.Printf("%s these %d issues and their backports? Type yes: ", what, len(sections))
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(answer) != "yes" {
		return errors.New("nothing posted")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return newApp(o).PostBumpMD(ctx, o.bumpsPost, o.bumpsClose)
}
