package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// RunningBuilds are the URLs of the builds running on the controller now (each once, wherever it runs).
func (j *jenkins) RunningBuilds(ctx context.Context) ([]string, error) {
	var c struct {
		Computer []struct {
			Executors       []executor `json:"executors"`
			OneOffExecutors []executor `json:"oneOffExecutors"`
		} `json:"computer"`
	}
	u := j.BaseURL + "/computer/api/json?depth=2&tree=computer%5Bexecutors%5BcurrentExecutable%5Burl%5D%5D," +
		"oneOffExecutors%5BcurrentExecutable%5Burl%5D%5D%5D"
	if getErr := j.getJSON(ctx, u, &c); getErr != nil {
		return nil, getErr
	}

	seen := map[string]bool{}
	var builds []string
	for _, node := range c.Computer {
		for _, e := range append(node.Executors, node.OneOffExecutors...) {
			if e.CurrentExecutable == nil || e.CurrentExecutable.URL == "" {
				continue
			}
			if b := strings.TrimRight(e.CurrentExecutable.URL, "/") + "/"; !seen[b] {
				seen[b] = true
				builds = append(builds, b)
			}
		}
	}

	return builds, nil
}

type executor struct {
	CurrentExecutable *struct {
		URL string `json:"url"`
	} `json:"currentExecutable"`
}

// BusyLimit is the bump mode's Scheduler.LimitFor: while builds other than ours run on the controller,
// at most busy of ours (the team's rule: 4 with other runs active, the configured 6 without). Ours
// are told by their job path (ourJobs, "/job/a/job/b/"): a build of ours still queued holds a slot
// without running, so the slot count alone would hide another build. The count is cached for ttl;
// when it cannot be read the busy limit applies.
func BusyLimit(
	running func(context.Context) ([]string, error), ourJobs []string, busy int, ttl time.Duration,
) func(string, int, int) int {
	var (
		mu     sync.Mutex
		at     time.Time
		others = -1
	)

	return func(_ string, configured, ours int) int {
		mu.Lock()
		defer mu.Unlock()

		if others < 0 || time.Since(at) > ttl {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			builds, countErr := running(ctx)
			cancel()

			at = time.Now()
			others = 1 // unknown counts as busy
			if countErr == nil {
				others = otherBuilds(builds, ourJobs, ours)
			}
		}
		if others > 0 {
			return min(configured, busy)
		}

		return configured
	}
}

// otherBuilds counts the running builds that are not ours: those of other jobs, plus any of our jobs'
// builds beyond the ours we hold (someone else running the same job).
func otherBuilds(builds, ourJobs []string, ours int) int {
	mine, others := 0, 0
	for _, b := range builds {
		if slices.ContainsFunc(ourJobs, func(job string) bool { return strings.Contains(b, job) }) {
			mine++
		} else {
			others++
		}
	}

	return others + max(mine-ours, 0)
}

// BumpRun is the plan and the jobs of a bump run, saved next to its progress so a resume (and the MDs
// written once it ends) know which issue each check belongs to. Final is the progress the run ended
// with, kept until its MDs are written so a report that failed can be written again.
type BumpRun struct {
	Plan  *BumpPlan    `json:"plan"`
	Jobs  []JenkinsJob `json:"jobs"`
	Final *Progress    `json:"final,omitempty"`
}

// BumpJobs turns the plan's targets into the version-bump jobs (one per product and branch).
func BumpJobs(plan *BumpPlan, cfg *BumpConfig, dtfRef string) ([]JenkinsJob, error) {
	jobs := make([]JenkinsJob, 0, len(plan.Targets))
	for _, t := range plan.Targets {
		j, jobErr := t.Job(cfg, cfg.Controller, dtfRef)
		if jobErr != nil {
			return nil, fmt.Errorf("%s %s: %w", t.Product, t.Branch, jobErr)
		}
		jobs = append(jobs, j)
	}

	return jobs, nil
}

// SaveBumpRun and LoadBumpRun keep the BumpRun beside the run's state (owner-only).
func SaveBumpRun(path string, run *BumpRun) error {
	raw, marshalErr := json.MarshalIndent(run, "", " ")
	if marshalErr != nil {
		return marshalErr
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		return mkErr
	}

	return writeDurable(path, raw)
}

func LoadBumpRun(path string) (*BumpRun, error) {
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}

	var run BumpRun
	if decodeErr := json.Unmarshal(raw, &run); decodeErr != nil {
		return nil, fmt.Errorf("read the bump run %s: %w", path, decodeErr)
	}

	return &run, nil
}

// PrepareJobs makes a plan of just these jobs (no workflows, no Qase): the bump run executes it like a
// release run, with its state, resume and terminal commands.
func (a *App) PrepareJobs(jobs []JenkinsJob) (*prepared, error) {
	matrix, matrixErr := a.loadMatrix()
	if matrixErr != nil {
		return nil, matrixErr
	}
	if !a.cfg.SkipWorkflows {
		return nil, errors.New("a jobs-only plan needs SkipWorkflows")
	}

	now := time.Now()
	plan := &Plan{RequestID: newRequestID(now), Jobs: jobs, SkipWorkflows: true}

	return &prepared{Plan: plan, Matrix: matrix, GitHub: a.cfg.NewGitHub(a.cfg.GitHubToken)}, nil
}

// ControllerBusyLimit is BusyLimit on the named controller for our jobs (their paths), with the
// app's credentials.
func (a *App) ControllerBusyLimit(controller string, jobs []string, busy int) (func(string, int, int) int, error) {
	matrix, matrixErr := a.loadMatrix()
	if matrixErr != nil {
		return nil, matrixErr
	}

	clients := a.controllerClients(matrix)
	j := clients[controller]
	if j == nil {
		return nil, fmt.Errorf("no credentials for controller %s (JENKINS_%s_AUTH)", controller, strings.ToUpper(controller))
	}

	ourJobs := make([]string, 0, len(jobs))
	for _, path := range jobs {
		ourJobs = append(ourJobs, strings.TrimPrefix(j.jobURL(path), j.BaseURL)+"/")
	}

	return BusyLimit(j.RunningBuilds, ourJobs, busy, 2*time.Minute), nil
}

// BuildParams are the parameters a build ran with.
func (j *jenkins) BuildParams(ctx context.Context, buildURL string) (map[string]string, error) {
	var b struct {
		Actions []struct {
			Parameters []struct {
				Name  string `json:"name"`
				Value any    `json:"value"`
			} `json:"parameters"`
		} `json:"actions"`
	}
	paramsURL := strings.TrimRight(buildURL, "/") + "/api/json?tree=actions%5Bparameters%5Bname,value%5D%5D"
	if getErr := j.getJSON(ctx, paramsURL, &b); getErr != nil {
		return nil, getErr
	}

	params := map[string]string{}
	for _, a := range b.Actions {
		for _, p := range a.Parameters {
			params[p.Name] = fmt.Sprint(p.Value)
		}
	}

	return params, nil
}

// BumpResults reads the consoles of the run's passed jobs, by product and then branch, and names the
// jobs that did not pass (their bumps stay out of the MDs).
func (a *App) BumpResults(ctx context.Context, run *BumpRun, progress *Progress) (
	results map[string]map[string]*BranchResult, notPassed []string, err error,
) {
	targets := map[string]*BumpTarget{}
	for _, t := range run.Plan.Targets {
		targets[t.Product+"-bumps-"+t.Minor()] = t
	}

	results = map[string]map[string]*BranchResult{}
	for i := range progress.Jobs {
		rec := &progress.Jobs[i]
		t := targets[rec.Job.Name]
		if t == nil {
			continue
		}
		if rec.Result != resultSuccess || rec.BuildURL == "" {
			state := strings.ToLower(rec.State + " " + rec.Result)
			notPassed = append(notPassed, fmt.Sprintf("%s %s (%s)", t.Product, t.Branch, state))
			continue
		}

		j, jenkinsErr := a.JenkinsForBuild(rec.BuildURL)
		if jenkinsErr != nil {
			return nil, nil, jenkinsErr
		}
		console, consoleErr := j.ConsoleText(ctx, rec.BuildURL)
		if consoleErr != nil {
			return nil, nil, fmt.Errorf("console of %s %s: %w", t.Product, t.Branch, consoleErr)
		}
		params, paramsErr := j.BuildParams(ctx, rec.BuildURL)
		if paramsErr != nil {
			return nil, nil, fmt.Errorf("parameters of %s %s: %w", t.Product, t.Branch, paramsErr)
		}

		version, checks := ParseBumpConsole(console)
		if missing := noOutput(t.Checks, checks); missing != "" {
			notPassed = append(notPassed, fmt.Sprintf("%s %s (passed, but its console shows no output for %s)",
				t.Product, t.Branch, missing))
			continue
		}

		if results[t.Product] == nil {
			results[t.Product] = map[string]*BranchResult{}
		}
		results[t.Product][t.Branch] = &BranchResult{
			Target: t, Version: version, NodeOS: params["NODE_OS"], Arch: params["ARCH"], Results: checks,
		}
	}

	return results, notPassed, nil
}

// noOutput is the first check whose output the console does not show (a command with no output is no
// evidence), or "".
func noOutput(checks []BumpCheck, results []BumpResult) string {
	for _, c := range checks {
		if len(resultFor(results, c.Expected).Output) == 0 {
			return c.What
		}
	}

	return ""
}

// WriteBumpMDs writes <dir>/<product>-bumps-release-<month>.md for each product with a validated bump
// and returns the files written.
func WriteBumpMDs(dir string, plan *BumpPlan, results map[string]map[string]*BranchResult) ([]string, error) {
	var files []string
	for _, product := range []string{"k3s", "rke2"} {
		md, renderErr := RenderBumpMD(product, plan.Month, plan.Groups, results[product])
		if renderErr != nil {
			return files, fmt.Errorf("%s MD: %w", product, renderErr)
		}
		if md == "" {
			continue
		}

		if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
			return files, mkErr
		}
		file := filepath.Join(dir, fmt.Sprintf("%s-bumps-release-%s.md", product, plan.Month))
		if writeErr := writeDurable(file, []byte(md)); writeErr != nil {
			return files, writeErr
		}
		files = append(files, file)
	}

	return files, nil
}
