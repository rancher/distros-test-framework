package releasebot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type LogFunc func(level, format string, args ...any)

type Config struct {
	MatrixPath    string
	DTFRef        string
	SkipTagCheck  bool
	SkipWorkflows bool
	SkipJobs      bool
	Poll          time.Duration
	QaseTimeout   time.Duration
	GitHubToken   string
	JenkinsAuth   func(controller string) (user, token string, ok bool)
	NewQase       func() (*Qase, error)
	NewGitHub     func(token string) *GitHub
	NewJenkins    func(baseURL, user, token string) *Jenkins
	Log           LogFunc
}

type App struct {
	cfg               Config
	controllerMu      sync.Mutex
	controllers       map[string]controllerClient
	controllersLoaded bool
}

func New(cfg *Config) *App {
	a := &App{cfg: *cfg}
	if a.cfg.Log == nil {
		a.cfg.Log = func(string, string, ...any) {}
	}
	if a.cfg.NewQase == nil {
		a.cfg.NewQase = newQase
	}
	if a.cfg.NewGitHub == nil {
		a.cfg.NewGitHub = newGitHub
	}
	if a.cfg.NewJenkins == nil {
		a.cfg.NewJenkins = NewJenkins
	}
	if a.cfg.JenkinsAuth == nil {
		a.cfg.JenkinsAuth = func(string) (string, string, bool) { return "", "", false }
	}

	return a
}

// Hooks are one run's controls; listen mode sets them per run, the one-shot CLI leaves them empty.
type Hooks struct {
	Notify    LogFunc
	Stop      <-chan struct{}
	SetStatus func(func() string)
	Capacity  *Capacity
	Owner     string
	Commands  <-chan Command
	Help      func(string)
	Triage    Triager
	Deep      Triager
}

type Prepared struct {
	Plan        *Plan
	Matrix      *Matrix
	GitHub      *GitHub
	controllers map[string]*Jenkins
}

func (a *App) Prepare(ctx context.Context, req Request) (*Prepared, error) {
	matrix, err := LoadMatrixWithRef(a.cfg.MatrixPath, a.cfg.DTFRef)
	if err != nil {
		return nil, err
	}

	gh := a.cfg.NewGitHub(a.cfg.GitHubToken)
	controllers := a.controllerClients(matrix)
	now := time.Now()
	plan, err := buildPlan(ctx, req, matrix, now, newRequestID(now),
		&planResolver{gh: gh, jenkins: controllers})
	if err != nil {
		return nil, err
	}

	if !a.cfg.SkipTagCheck {
		if tagErr := checkTags(ctx, gh, plan.Request); tagErr != nil {
			return nil, tagErr
		}
	}

	return &Prepared{Plan: plan, Matrix: matrix, GitHub: gh, controllers: controllers}, nil
}

// Execute dispatches the workflows, waits for the Qase runs and runs the jobs. Once anything may
// have started, an error wraps ErrPartialRun.
func (a *App) Execute(ctx context.Context, p *Prepared, h *Hooks) (err error) {
	if h == nil {
		h = &Hooks{}
	}
	if h.Notify == nil {
		h.Notify = a.cfg.Log
	}
	qase, builders, err := a.preflight(p)
	if err != nil {
		return err
	}

	started := false
	defer func() {
		if err != nil && started && !errors.Is(err, ErrPartialRun) {
			err = fmt.Errorf("%w: %w", ErrPartialRun, err)
		}
	}()

	if !a.cfg.SkipWorkflows {
		if started, err = dispatchWorkflows(ctx, h.Notify, p.GitHub, p.Plan.Workflows); err != nil {
			return err
		}
	}

	if a.cfg.SkipJobs || len(p.Plan.Jobs) == 0 {
		return nil
	}

	if qase != nil {
		// Runs created by this dispatch carry the request id; with SkipWorkflows (runs created by
		// hand) there is none, and the newest run with each title is used.
		requestID := p.Plan.RequestID
		if a.cfg.SkipWorkflows {
			requestID = ""
		}
		if qErr := a.fillQaseRunIDs(ctx, h.Notify, qase, p.Plan, requestID); qErr != nil {
			return qErr
		}
	}

	started = true

	return a.runJobs(ctx, h, p.Matrix, builders, p.Plan.Jobs)
}

func (a *App) fillQaseRunIDs(
	ctx context.Context,
	notify LogFunc,
	qase *Qase,
	plan *Plan,
	requestID string,
) error {
	notify("info", "waiting for the Qase runs of request %q", requestID)

	ids, err := waitQaseRuns(ctx, qase, plan.QaseTitles, requestID, 20*time.Second, a.cfg.QaseTimeout)
	if err != nil {
		return err
	}
	for _, t := range plan.QaseTitles {
		notify("info", "Qase run %d: %s", ids[t], t)
	}

	return applyQaseRunIDs(plan.Jobs, ids)
}

// preflight refuses a plan that cannot run and checks the Qase and Jenkins credentials before
// anything is dispatched; qase is nil when no job needs a run id.
func (a *App) preflight(p *Prepared) (qase *Qase, builders map[string]Builder, err error) {
	if a.cfg.SkipJobs {
		return nil, nil, nil
	}
	if err = p.Plan.RunnableError(); err != nil {
		return nil, nil, fmt.Errorf("not running: %w", err)
	}
	if jobsNeedQaseRun(p.Plan.Jobs) {
		if qase, err = a.cfg.NewQase(); err != nil {
			return nil, nil, fmt.Errorf("jobs use {{QASE_RUN_ID}}: %w", err)
		}
	}
	if len(p.Plan.Jobs) > 0 {
		controllers := p.controllers
		if controllers == nil {
			controllers = a.controllerClients(p.Matrix)
		}
		if builders, err = buildersFor(p.Matrix, controllers); err != nil {
			return nil, nil, err
		}
	}

	return qase, builders, nil
}

func jobsNeedQaseRun(jobs []JenkinsJob) bool {
	for i := range jobs {
		for _, v := range jobs[i].Params {
			if strings.Contains(v, "{{QASE_RUN_ID}}") {
				return true
			}
		}
	}

	return false
}

// dispatchWorkflows reports whether any workflow may have started, even when it fails: only a
// first dispatch rejected or never sent proves nothing did; a lost response may hide an accepted one.
func dispatchWorkflows(
	ctx context.Context,
	notify LogFunc,
	gh *GitHub,
	workflows []WorkflowDispatch,
) (started bool, err error) {
	for i, w := range workflows {
		if dErr := gh.Dispatch(ctx, w); dErr != nil {
			nothing := errors.Is(dErr, ErrDispatchRejected) || errors.Is(dErr, ErrDispatchNotSent)

			return i > 0 || !nothing, dErr
		}
		notify("info", "dispatched %s on %s@%s", w.Workflow, w.Repo, w.Ref)
	}

	return len(workflows) > 0, nil
}

func (a *App) runJobs(
	ctx context.Context,
	h *Hooks,
	matrix *Matrix,
	builders map[string]Builder,
	jobs []JenkinsJob,
) error {
	s := &Scheduler{
		Builders:   builders,
		Limits:     matrix.Controller,
		Poll:       a.cfg.Poll,
		Notify:     h.Notify,
		Capacity:   h.Capacity,
		Stop:       h.Stop,
		Owner:      h.Owner,
		Triage:     h.Triage,
		DeepTriage: h.Deep,
		Commands:   h.Commands,
		Help:       h.Help,
	}
	if h.SetStatus != nil {
		h.SetStatus(s.Status)
	}

	outcomes := s.Run(ctx, jobs)
	failed := 0
	h.Notify("info", "Summary:")
	for i := range outcomes {
		oc := &outcomes[i]
		status := oc.Result
		if oc.Err != nil {
			status = "ERROR: " + oc.Err.Error()
		}

		// SKIPPED is a person's decision to count the job as passed, not a failure.
		level := "info"
		if status != resultSuccess && status != ResultSkipped {
			failed++
			level = "warn"
		}

		h.Notify(level, "  %-8s %-55s %s %s", status, oc.Job.Path, oc.Job.Version, oc.BuildURL)
	}
	if err := unreconciled(outcomes); err != nil {
		return fmt.Errorf("%d of %d jobs did not succeed; %w", failed, len(jobs), err)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d jobs did not succeed", failed, len(jobs))
	}

	return nil
}

func checkTags(ctx context.Context, gh *GitHub, req Request) error {
	var missing []string
	check := func(product string, tags []string) error {
		for _, t := range tags {
			ok, err := gh.TagExists(ctx, product, t)
			if err != nil {
				return err
			}
			if !ok {
				missing = append(missing, t)
			}
		}

		return nil
	}
	if err := check("k3s", req.K3s); err != nil {
		return err
	}
	if err := check("rke2", req.RKE2); err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("tags not found on GitHub: %s", strings.Join(missing, ", "))
	}

	return nil
}
