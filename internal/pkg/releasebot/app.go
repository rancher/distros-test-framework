package releasebot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type logFunc func(level, format string, args ...any)

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
	NewGitHub     func(token string) *gitHub
	NewJenkins    func(baseURL, user, token string) *jenkins
	Log           logFunc
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

// hooks are one run's controls; listen mode sets them per run, the one-shot CLI leaves them empty.
type hooks struct {
	Notify    logFunc
	Stop      <-chan struct{}
	SetStatus func(func() string)
	Capacity  *Capacity
	Owner     string
	Commands  <-chan Command
	Help      func(string)
	Triage    Triager
	Deep      Triager

	// Save keeps the run's progress at each change, so a restarted bot can resume it; may be nil.
	// While it fails no job is triggered (a restart would not know about the build).
	Save func(Progress) error

	// Ready and Go hold a resumed run until every resumed run took its builds' slots back (see
	// Scheduler.Ready); both may be nil.
	Ready func()
	Go    <-chan struct{}
}

func (h *hooks) save(p *Progress) error {
	if h.Save == nil {
		return nil
	}

	return h.Save(*p)
}

func (h *hooks) ready() {
	if h.Ready != nil {
		h.Ready()
	}
}

type prepared struct {
	Plan        *Plan
	Matrix      *Matrix
	GitHub      *gitHub
	controllers map[string]*jenkins
}

func (a *App) Prepare(ctx context.Context, req Request) (*prepared, error) {
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

	return &prepared{Plan: plan, Matrix: matrix, GitHub: gh, controllers: controllers}, nil
}

// Execute dispatches the workflows, waits for the Qase runs and runs the jobs.
func (a *App) Execute(ctx context.Context, p *prepared, h *hooks) (err error) {
	if h == nil {
		h = &hooks{}
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
		if err != nil && started && !errors.Is(err, errPartialRun) {
			err = fmt.Errorf("%w: %w", errPartialRun, err)
		}
	}()

	progress := &Progress{RequestID: p.Plan.RequestID, Stage: StageDispatching}
	if !a.cfg.SkipWorkflows {
		// Unsaved, a restart could not tell that workflows were dispatched: nothing is sent then.
		if saveErr := h.save(progress); saveErr != nil {
			return fmt.Errorf("could not save the run state before dispatching: %w", saveErr)
		}
		if started, err = dispatchWorkflows(ctx, h.Notify, p.GitHub, p.Plan.Workflows); err != nil {
			return err
		}
	}

	// From here on the run can be resumed: nothing it did so far would be repeated.
	progress.Stage, progress.QaseTitles, progress.Jobs = StageDispatched, p.Plan.QaseTitles, pendingRecords(p.Plan.Jobs)
	_ = h.save(progress)

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
		if qErr := a.fillQaseRunIDs(ctx, h.Notify, qase, p.Plan.QaseTitles, p.Plan.Jobs, requestID); qErr != nil {
			return qErr
		}
	}

	started = true

	return a.runJobs(ctx, h, progress, p.Matrix, builders, p.Plan.Jobs, nil)
}

// errResumeFailed means a saved run could not be set up again (missing credentials, an unreadable
// matrix, no Qase runs).
var errResumeFailed = errors.New("could not resume the run")

// resumeFailed holds the slots of the run's builds still on jenkins (until a later start resumes it).
func (*App) resumeFailed(saved *Progress, h *hooks, cause error) error {
	if saved.Stage == StageJobs && h.Capacity != nil {
		for i := range saved.Jobs {
			switch rec := &saved.Jobs[i]; rec.State {
			case jobQueued, jobRunning, jobTriggering, jobUnknown:
				h.Capacity.take(rec.Job.Controller)
				h.Capacity.abandon(rec.Job.Controller, h.Owner)
			}
		}
	}
	h.ready()

	return fmt.Errorf("%w: %w", errResumeFailed, cause)
}

// errNotResumable marks a saved run a restarted bot cannot carry on: it stopped before its
// workflows were all dispatched, so whether something started is unknown.
var errNotResumable = errors.New("run cannot be resumed")

// Resume carries on a run saved by Execute (see hooks.Save) after the bot restarted: Qase run ids
// are looked up again if they were not applied yet, and the jobs continue where they were.
func (a *App) Resume(ctx context.Context, saved *Progress, h *hooks) (err error) {
	if h.Notify == nil {
		h.Notify = a.cfg.Log
	}
	if !saved.Resumable() {
		h.ready()
		return fmt.Errorf("%w: it stopped while dispatching its workflows", errNotResumable)
	}
	if saved.Stage != StageJobs {
		h.ready() // no build on jenkins yet: nothing to take back before the others go on
	}

	defer func() {
		if err != nil && !errors.Is(err, errPartialRun) && !errors.Is(err, errResumeFailed) {
			err = fmt.Errorf("%w: %w", errPartialRun, err)
		}
	}()

	matrix, err := LoadMatrixWithRef(a.cfg.MatrixPath, a.cfg.DTFRef)
	if err != nil {
		return a.resumeFailed(saved, h, err)
	}

	builders, err := buildersFor(matrix, a.controllerClients(matrix))
	if err != nil {
		return a.resumeFailed(saved, h, err)
	}

	jobs := make([]JenkinsJob, 0, len(saved.Jobs))
	for i := range saved.Jobs {
		jobs = append(jobs, saved.Jobs[i].Job)
	}

	progress := saved
	if saved.Stage == StageDispatched {
		if jobsNeedQaseRun(jobs) {
			qase, qErr := a.cfg.NewQase()
			if qErr != nil {
				return a.resumeFailed(saved, h, fmt.Errorf("jobs use {{QASE_RUN_ID}}: %w", qErr))
			}
			if fillErr := a.fillQaseRunIDs(ctx, h.Notify, qase, saved.QaseTitles, jobs, saved.RequestID); fillErr != nil {
				return a.resumeFailed(saved, h, fillErr)
			}
		}

		return a.runJobs(ctx, h, progress, matrix, builders, jobs, pendingRecords(jobs))
	}

	return a.runJobs(ctx, h, progress, matrix, builders, jobs, saved.Jobs)
}

func pendingRecords(jobs []JenkinsJob) []JobRecord {
	recs := make([]JobRecord, 0, len(jobs))
	for i := range jobs {
		recs = append(recs, JobRecord{Job: jobs[i], State: jobPending})
	}

	return recs
}

func (a *App) fillQaseRunIDs(
	ctx context.Context,
	notify logFunc,
	qase *Qase,
	titles []string,
	jobs []JenkinsJob,
	requestID string,
) error {
	notify("info", "waiting for the Qase runs of request %q", requestID)

	ids, err := waitQaseRuns(ctx, qase, titles, requestID, 20*time.Second, a.cfg.QaseTimeout)
	if err != nil {
		return err
	}
	for _, t := range titles {
		notify("info", "Qase run %d: %s", ids[t], t)
	}

	return applyQaseRunIDs(jobs, ids)
}

// preflight refuses a plan that cannot run and checks the Qase and jenkins credentials before
// anything is dispatched; qase is nil when no job needs a run id.
func (a *App) preflight(p *prepared) (qase *Qase, builders map[string]Builder, err error) {
	if a.cfg.SkipJobs {
		return nil, nil, nil
	}

	if err = p.Plan.runnableError(); err != nil {
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
	notify logFunc,
	gh *gitHub,
	workflows []workflowDispatch,
) (started bool, err error) {
	for i, w := range workflows {
		if dErr := gh.Dispatch(ctx, w); dErr != nil {
			nothing := errors.Is(dErr, errDispatchRejected) || errors.Is(dErr, errDispatchNotSent)

			return i > 0 || !nothing, dErr
		}
		notify("info", "dispatched %s on %s@%s", w.Workflow, w.Repo, w.Ref)
	}

	return len(workflows) > 0, nil
}

// runJobs schedules jobs, or carries on resume (a saved run) when set, saving progress at each change.
func (a *App) runJobs(
	ctx context.Context,
	h *hooks,
	progress *Progress,
	matrix *Matrix,
	builders map[string]Builder,
	jobs []JenkinsJob,
	resume []JobRecord,
) error {
	s := &Scheduler{
		Builders:      builders,
		Limits:        matrix.Controller,
		Poll:          a.cfg.Poll,
		Notify:        h.Notify,
		Capacity:      h.Capacity,
		Stop:          h.Stop,
		Owner:         h.Owner,
		Triage:        h.Triage,
		DeepTriage:    h.Deep,
		Commands:      h.Commands,
		Help:          h.Help,
		Resume:        resume,
		ResumeStopped: progress.Stopped,
		Ready:         h.Ready,
		Go:            h.Go,
		Journal: func(recs []JobRecord, stopped bool) error {
			progress.Stage, progress.Jobs, progress.Stopped = StageJobs, recs, stopped
			return h.save(progress)
		},
	}
	if h.SetStatus != nil {
		h.SetStatus(s.Status)
	}

	err := summarize(h.Notify, s.Run(ctx, jobs))
	if s.canceled {
		return errors.Join(err, fmt.Errorf("run interrupted: %w", ctx.Err()))
	}

	return err
}

// summarize reports every job's outcome and returns an error unless all passed (or were skipped).
func summarize(notify logFunc, outcomes []Outcome) error {
	failed := 0
	notify("info", "Summary:")
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

		notify(level, "  %-8s %-55s %s %s", status, oc.Job.Path, oc.Job.Version, oc.BuildURL)
	}

	if err := unreconciled(outcomes); err != nil {
		return fmt.Errorf("%d of %d jobs did not succeed; %w", failed, len(outcomes), err)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d jobs did not succeed", failed, len(outcomes))
	}

	return nil
}

func checkTags(ctx context.Context, gh *gitHub, req Request) error {
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
		return fmt.Errorf("tags not found on gitHub: %s", strings.Join(missing, ", "))
	}

	return nil
}
