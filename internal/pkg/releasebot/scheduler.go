package releasebot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	// ErrQueueCanceled means Jenkins dropped the queue item: the job never ran, so its slot is free.
	ErrQueueCanceled = errors.New("queue item was canceled")

	// ErrTriggerUnknown means the trigger may have been accepted (lost response, gateway error):
	// a build may exist, so the job keeps its slot.
	ErrTriggerUnknown = errors.New("trigger outcome unknown")
	// ErrStateUnknown means a build's status could not be read before giving up: it may still run.
	ErrStateUnknown = errors.New("build state unknown")
	// ErrUnreconciledBuilds is returned by a run that ended with builds that may still be running
	// on Jenkins. Their capacity was never released, so no new run may start until a person checks.
	ErrUnreconciledBuilds = errors.New("builds left in unknown state")
	// ErrPartialRun marks a run that failed after it may have dispatched or triggered something:
	// repeating it could duplicate workflows, Qase runs or builds, so a person checks first.
	ErrPartialRun = errors.New("run stopped part-way")
)

// defaultMaxPollErrors is how many consecutive failed status queries a job tolerates.
const defaultMaxPollErrors = 10

const resultSuccess = "SUCCESS"

// ResultSkipped marks a failed job a person counted as passed (`skip`).
const ResultSkipped = "SKIPPED"

// Outcome is the final state of one scheduled job.
type Outcome struct {
	Job      JenkinsJob
	BuildURL string
	Result   string
	Err      error
}

// Scheduler triggers jobs in plan order, never exceeding each controller's MaxConcurrent.
type Scheduler struct {
	Builders map[string]Builder
	Limits   map[string]Limits
	Poll     time.Duration

	// Capacity is shared by every run of the bot, so concurrent runs respect MaxConcurrent
	// together; nil gives this Scheduler its own.
	Capacity *Capacity

	// Stop, once closed, starts no new job; running builds are still followed to the end.
	Stop <-chan struct{}
	// Owner names this run in Capacity, so `unblock` can free the slots its unknown builds hold.
	Owner string

	// Triage looks at each failed build and may ask for one automatic rerun; nil never reruns.
	Triage Triager
	// DeepTriage is the full analysis a person asks for with `triage` on a waiting job; may be nil.
	DeepTriage Triager
	// Commands carries people's answers for jobs waiting for help. When nil nobody can answer,
	// so a failure that needs help simply fails the job.
	Commands <-chan Command
	// Help reaches the people following the run (the run's thread); nil logs through Notify.
	Help func(string)

	// MaxPollErrors bounds consecutive failed queue/build queries per job (default 10).
	MaxPollErrors int

	// Notify receives every state change with a level ("debug", "info", "warn", "error"), matching
	// resources.LogLevel; may be nil.
	Notify func(level, format string, args ...any)

	statusMu sync.Mutex
	status   string
}

// Decision is triage's answer for a failed build.
type Decision struct {
	Rerun   bool
	Summary string
}

// Triager explains a failed build and says whether rerunning it automatically is worth it. The
// scheduler still refuses reruns its fixed rules forbid (see rerunDenied).
type Triager func(ctx context.Context, o *Outcome) Decision

// askTriager is the triager until automatic triage exists: every failure goes to a person.
func askTriager(context.Context, *Outcome) Decision {
	return Decision{Summary: "automatic triage is not set up yet"}
}

// Actions a person can take on a job waiting for help.
const (
	CommandRetry  = "retry"  // run it again
	CommandSkip   = "skip"   // count it as passed
	CommandTriage = "triage" // run the full triage and post it; the job keeps waiting
)

// Command is a person's answer, from the run's thread, for a job waiting for help.
type Command struct {
	Action  string // CommandRetry, CommandSkip or CommandTriage
	Job     string
	Version string // RC tag; may be empty when only one waiting job has that name
	By      string
	Reply   chan<- string // buffered (1): the scheduler never blocks on it and drops a reply that does not fit
}

// ErrStopped marks jobs left untriggered because the run was stopped.
var ErrStopped = errors.New("not triggered: run stopped")

type running struct {
	job      JenkinsJob
	queueURL string
	buildURL string
	errs     int

	// abandoned: state unknown (lost trigger response or too many failed queries); the entry
	// only holds its slot, since a build may be running.
	abandoned bool
}

// runState is one Run: pending jobs, what occupies each controller, and finished results.
type runState struct {
	pending []JenkinsJob
	active  map[string][]*running
	down    map[string]bool
	done    map[string]string
	out     []Outcome

	// Failed jobs being triaged, and jobs waiting for a person; both hold no slot and keep
	// their dependents waiting.
	triaging map[string]*Outcome
	held     map[string]*Outcome
	triaged  chan triageResult
	quit     chan struct{}
	stopped  bool

	// triageCtx is canceled by stop, so a slow triage cannot keep a stopped run alive.
	triageCtx    context.Context
	cancelTriage context.CancelFunc
	// help delivers "Needs help" messages from its own goroutine: a slow Slack post never holds
	// up the scheduler.
	help *asyncQueue

	// deep marks jobs with a full triage in progress (it costs several dollars): one at a time.
	// A run that ends waits for them (their result is promised); stop cancels them.
	deepMu     sync.Mutex
	deep       map[string]bool
	deepWG     sync.WaitGroup
	deepCtx    context.Context
	cancelDeep context.CancelFunc
}

type triageResult struct {
	out Outcome
	d   Decision
}

// jobKey identifies a job within a plan: dependencies resolve within the same product and version.
func jobKey(product, version, name string) string {
	return product + "|" + version + "|" + name
}

func (j *JenkinsJob) key() string { return jobKey(j.Product, j.Version, j.Name) }

func (j *JenkinsJob) depKeys() []string {
	keys := make([]string, 0, len(j.DependsOn))
	for _, d := range j.DependsOn {
		keys = append(keys, jobKey(j.Product, j.Version, d))
	}

	return keys
}

// Run blocks until every job finished or ctx is done; failures of one job never stop the others.
func (s *Scheduler) Run(ctx context.Context, jobs []JenkinsJob) []Outcome {
	st := s.newRunState(ctx, jobs)
	defer st.end()

	if err := validateDependencies(jobs); err != nil {
		for i := range jobs {
			st.out = append(st.out, Outcome{Job: jobs[i], Err: fmt.Errorf("not triggered: %w", err)})
		}

		return st.out
	}

	poll := s.Poll
	if poll <= 0 {
		poll = 30 * time.Second
	}
	if s.Capacity == nil {
		s.Capacity = &Capacity{}
	}
	stop := s.Stop

	for {
		if ctx.Err() != nil {
			s.cancelRun(st, ctx.Err())

			return st.out
		}
		if stopRequested(stop) {
			stop = nil
			s.drain(st)
		}
		progressed := s.startWhatFits(ctx, st)
		s.reap(ctx, st)
		s.markUnreachable(st)
		s.publish(st)

		live := st.liveCount()
		waiting := len(st.triaging) + len(st.held)
		if len(st.pending) == 0 && live == 0 && waiting == 0 {
			return st.out
		}
		if live == 0 && waiting == 0 && !progressed && !s.anyStartable(st) {
			// Nothing running and nothing can start: report instead of waiting forever.
			for i := range st.pending {
				st.finish(&Outcome{Job: st.pending[i], Err: errors.New("not triggered: dependencies never resolved")})
			}

			return st.out
		}

		if !s.wait(ctx, st, stop, poll) {
			s.cancelRun(st, ctx.Err())

			return st.out
		}
	}
}

// newRunState sets up one Run; end releases what it started.
func (s *Scheduler) newRunState(ctx context.Context, jobs []JenkinsJob) *runState {
	st := &runState{
		pending:  append([]JenkinsJob{}, jobs...),
		active:   map[string][]*running{},
		down:     map[string]bool{},
		done:     map[string]string{},
		triaging: map[string]*Outcome{},
		held:     map[string]*Outcome{},
		triaged:  make(chan triageResult),
		quit:     make(chan struct{}),
	}
	st.triageCtx, st.cancelTriage = context.WithCancel(ctx)
	st.deepCtx, st.cancelDeep = context.WithCancel(ctx)
	st.help = newAsyncQueue(func(msg string) {
		if s.Help != nil {
			s.Help(msg)
		} else {
			s.notify("warn", "%s", msg)
		}
	})

	return st
}

func (st *runState) end() {
	close(st.quit)
	st.cancelTriage()
	st.deepWG.Wait()
	st.cancelDeep()
	st.help.close()
}

// wait sleeps until the next poll, acting on triage results and commands as they come; it
// returns false when ctx is done.
func (s *Scheduler) wait(ctx context.Context, st *runState, stop <-chan struct{}, poll time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-stop:
	case r := <-st.triaged:
		s.decide(st, &r)
	case c := <-s.Commands:
		reply := s.apply(st, &c)
		select {
		case c.Reply <- reply:
		default:
			s.notify("warn", "reply to %s %s dropped (no buffered Reply): %s", c.Action, c.Job, reply)
		}
	case <-time.After(poll):
	}

	return true
}

// drain reports every pending job as stopped and finishes the failed ones waiting for triage or
// help; running builds are still followed.
func (s *Scheduler) drain(st *runState) {
	st.stopped = true
	for i := range st.pending {
		st.finish(&Outcome{Job: st.pending[i], Err: ErrStopped})
	}
	st.pending = nil
	for k, o := range st.held {
		st.finish(o)
		delete(st.held, k)
	}
	// Triage still running is canceled; its failed outcome stands and a late verdict is ignored.
	st.cancelTriage()
	st.cancelDeep()
	for k, o := range st.triaging {
		st.finish(o)
		delete(st.triaging, k)
	}
	s.notify("info", "stopped: no new job will start; %d running builds are followed", st.liveCount())
}

// startWhatFits walks pending jobs in plan order and reports whether any was started or resolved.
func (s *Scheduler) startWhatFits(ctx context.Context, st *runState) bool {
	if ctx.Err() != nil {
		return false // canceled: nothing is sent, Run reports the pending jobs
	}
	progressed := false
	var rest []JenkinsJob
	for i := range st.pending {
		j := st.pending[i]

		ready, blocker := st.depsState(&j)
		if blocker != "" {
			s.notify("warn", "not triggering %s %s: %s", j.Path, j.Version, blocker)
			st.finish(&Outcome{Job: j, Err: errors.New("not triggered: " + blocker)})
			progressed = true
			continue
		}
		if !ready {
			rest = append(rest, j) // waiting for dependencies: takes no capacity
			continue
		}

		b, ok := s.Builders[j.Controller]
		if !ok || s.Limits[j.Controller].MaxConcurrent < 1 {
			st.finish(&Outcome{Job: j, Err: fmt.Errorf("controller %q has no builder or limit", j.Controller)})
			progressed = true
			continue
		}

		if st.down[j.Controller] {
			s.notify("warn", "not triggering %s %s: controller %s is unreachable", j.Path, j.Version, j.Controller)
			st.finish(&Outcome{Job: j, Err: fmt.Errorf("not triggered: controller %s unreachable", j.Controller)})
			progressed = true
			continue
		}

		// A trigger can take a while: re-check stop and cancellation before each one.
		if stopRequested(s.Stop) || ctx.Err() != nil {
			rest = append(rest, st.pending[i:]...)
			break
		}
		if !s.Capacity.acquire(j.Controller, s.Limits[j.Controller].MaxConcurrent) {
			rest = append(rest, j)
			continue
		}

		progressed = true
		s.trigger(ctx, st, b, &j)
	}
	st.pending = rest

	return progressed
}

// trigger starts one job. A confirmed rejection frees the slot; a trigger that may have been
// accepted keeps it (abandoned), since a build may be running.
func (s *Scheduler) trigger(ctx context.Context, st *runState, b Builder, j *JenkinsJob) {
	q, err := b.Trigger(ctx, j)
	switch {
	case errors.Is(err, ErrTriggerUnknown):
		s.notify("warn", "trigger of %s %s may have been accepted: %v", j.Path, j.Version, err)
		st.finish(&Outcome{Job: *j, Err: fmt.Errorf("%w; check %s for a build before re-running", err, j.Path)})
		st.active[j.Controller] = append(st.active[j.Controller], &running{job: *j, abandoned: true})
		s.Capacity.abandon(j.Controller, s.Owner)
	case err != nil:
		s.notify("error", "failed to trigger %s %s: %v", j.Path, j.Version, err)
		s.Capacity.release(j.Controller)
		s.settle(st, &Outcome{Job: *j, Err: err})
	default:
		if j.Attempt > 0 {
			s.notify("info", "triggered %s %s (attempt %d)", j.Path, j.Version, j.Attempt+1)
		} else {
			s.notify("info", "triggered %s %s", j.Path, j.Version)
		}
		st.active[j.Controller] = append(st.active[j.Controller], &running{job: *j, queueURL: q})
	}
}

// anyStartable reports whether a pending job has all dependencies resolved (it only waits for capacity).
func (*Scheduler) anyStartable(st *runState) bool {
	for i := range st.pending {
		if ready, _ := st.depsState(&st.pending[i]); ready {
			return true
		}
	}

	return false
}

// depsState: ready when every dependency succeeded; blocker is set once one finished otherwise.
func (st *runState) depsState(j *JenkinsJob) (ready bool, blocker string) {
	ready = true
	for i, k := range j.depKeys() {
		result, finished := st.done[k]
		switch {
		case !finished:
			ready = false
		case result != resultSuccess:
			return false, fmt.Sprintf("dependency %s %s ended with %s", j.DependsOn[i], j.Version, result)
		}
	}

	return ready, ""
}

// cancelRun reports what is left when ctx ends. Jenkins accepted the active jobs and they keep
// running, so they are in unknown state and keep their slots; pending ones never started.
func (s *Scheduler) cancelRun(st *runState, cause error) {
	for ctrl, list := range st.active {
		for _, r := range list {
			if !r.abandoned {
				s.Capacity.abandon(ctrl, s.Owner)
				stopped := fmt.Errorf("%w: run stopped while it was queued or running: %w", ErrStateUnknown, cause)
				st.out = append(st.out, Outcome{Job: r.job, BuildURL: r.buildURL, Err: stopped})
			}
		}
	}
	for i := range st.pending {
		st.out = append(st.out, Outcome{Job: st.pending[i], Err: cause})
	}
	for _, o := range st.triaging {
		st.out = append(st.out, *o)
	}
	for _, o := range st.held {
		st.out = append(st.out, *o)
	}
}

func (st *runState) finish(o *Outcome) {
	st.out = append(st.out, *o)
	result := o.Result
	if o.Err != nil {
		result = "error: " + o.Err.Error()
	}
	st.done[o.Job.key()] = result
}

func (st *runState) liveCount() int {
	n := 0
	for _, list := range st.active {
		for _, r := range list {
			if !r.abandoned {
				n++
			}
		}
	}

	return n
}

func (s *Scheduler) reap(ctx context.Context, st *runState) {
	if ctx.Err() != nil {
		return // canceled on purpose: Run reports the builds, a failed query would only add noise
	}
	for ctrl, list := range st.active {
		var keep []*running
		for _, r := range list {
			if r.abandoned {
				keep = append(keep, r)
				continue
			}
			outcome, done := s.check(ctx, s.Builders[ctrl], r)
			switch {
			case !done:
				keep = append(keep, r)
			case outcome.Err != nil && !errors.Is(outcome.Err, ErrQueueCanceled):
				// State unknown: the build may still be running, so the slot stays taken.
				st.finish(&outcome)
				r.abandoned = true
				s.Capacity.abandon(ctrl, s.Owner)
				keep = append(keep, r)
			default:
				s.Capacity.release(ctrl)
				s.settle(st, &outcome)
			}
		}
		st.active[ctrl] = keep
	}
}

// markUnreachable flags a controller once unknown-state jobs fill its whole limit: no capacity can
// ever free up there, so its remaining jobs are reported. Below the limit it keeps being used.
func (s *Scheduler) markUnreachable(st *runState) {
	for ctrl, lim := range s.Limits {
		// Counted across runs: another run's unknown builds can fill the controller too.
		abandoned := s.Capacity.Abandoned(ctrl)
		if abandoned > 0 && abandoned >= lim.MaxConcurrent && !st.down[ctrl] {
			st.down[ctrl] = true
			s.notify("warn", "controller %s: all %d slots held by jobs in unknown state; not triggering more there",
				ctrl, abandoned)
		}
	}
}

// check polls one job; done=true means an outcome is final (finished, canceled or given up).
func (s *Scheduler) check(ctx context.Context, b Builder, r *running) (Outcome, bool) {
	maxErrs := s.MaxPollErrors
	if maxErrs <= 0 {
		maxErrs = defaultMaxPollErrors
	}

	var err error
	if r.buildURL == "" {
		var u string
		u, err = b.BuildFromQueue(ctx, r.queueURL)
		if errors.Is(err, ErrQueueCanceled) {
			s.notify("warn", "%s %s: %v", r.job.Path, r.job.Version, err)
			return Outcome{Job: r.job, Err: err}, true
		}
		if err == nil && u != "" {
			r.buildURL = u
			s.notify("info", "started %s %s: %s", r.job.Path, r.job.Version, u)
		}
	}

	if err == nil && r.buildURL != "" {
		var finished bool
		var result string
		finished, result, err = b.Finished(ctx, r.buildURL)
		if err == nil && finished {
			s.notify("info", "finished %s %s: %s %s", r.job.Path, r.job.Version, result, r.buildURL)
			return Outcome{Job: r.job, BuildURL: r.buildURL, Result: result}, true
		}
	}

	if err == nil {
		r.errs = 0
		return Outcome{}, false
	}

	r.errs++
	s.notify("warn", "%s %s: status query failed (%d/%d): %v", r.job.Path, r.job.Version, r.errs, maxErrs, err)
	if r.errs < maxErrs {
		return Outcome{}, false
	}

	return Outcome{
		Job: r.job, BuildURL: r.buildURL,
		Err: fmt.Errorf("%w after %d failed status queries (last: %w); check %s by hand", ErrStateUnknown,
			r.errs, err, firstNonEmpty(r.buildURL, r.queueURL)),
	}, true
}

func (s *Scheduler) notify(level, format string, args ...any) {
	if s.Notify != nil {
		s.Notify(level, format, args...)
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}

	return b
}

// unreconciled lists outcomes whose build may still be running on Jenkins (unknown trigger or
// state) and returns ErrUnreconciledBuilds naming them, or nil when every build is accounted for.
func unreconciled(outcomes []Outcome) error {
	var names []string
	for i := range outcomes {
		o := &outcomes[i]
		if errors.Is(o.Err, ErrTriggerUnknown) || errors.Is(o.Err, ErrStateUnknown) {
			names = append(names, fmt.Sprintf("%s %s %s", o.Job.Path, o.Job.Version,
				firstNonEmpty(o.BuildURL, "(no build url)")))
		}
	}
	if len(names) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrUnreconciledBuilds, strings.Join(names, "; "))
}
