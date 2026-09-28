package releasebot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrQueueCanceled means Jenkins dropped the queue item: the job never ran, so its slot is free.
	ErrQueueCanceled = errors.New("queue item was canceled")

	// ErrTriggerUnknown means the trigger may have been accepted (lost response, gateway error):
	// a build may exist, so the job keeps its slot.
	ErrTriggerUnknown = errors.New("trigger outcome unknown")
)

// defaultMaxPollErrors is how many consecutive failed status queries a job tolerates.
const defaultMaxPollErrors = 10

const resultSuccess = "SUCCESS"

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

	// MaxPollErrors bounds consecutive failed queue/build queries per job (default 10).
	MaxPollErrors int

	// Notify receives every state change with a level ("debug", "info", "warn", "error"), matching
	// resources.LogLevel; may be nil.
	Notify func(level, format string, args ...any)
}

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
}

// JobKey identifies a job within a plan: dependencies resolve within the same product and version.
func JobKey(product, version, name string) string {
	return product + "|" + version + "|" + name
}

func (j *JenkinsJob) key() string { return JobKey(j.Product, j.Version, j.Name) }

func (j *JenkinsJob) depKeys() []string {
	keys := make([]string, 0, len(j.DependsOn))
	for _, d := range j.DependsOn {
		keys = append(keys, JobKey(j.Product, j.Version, d))
	}

	return keys
}

// Run blocks until every job finished or ctx is done; failures of one job never stop the others.
func (s *Scheduler) Run(ctx context.Context, jobs []JenkinsJob) []Outcome {
	st := &runState{
		pending: append([]JenkinsJob{}, jobs...),
		active:  map[string][]*running{},
		down:    map[string]bool{},
		done:    map[string]string{},
	}

	if err := ValidateDependencies(jobs); err != nil {
		for i := range jobs {
			st.out = append(st.out, Outcome{Job: jobs[i], Err: fmt.Errorf("not triggered: %w", err)})
		}

		return st.out
	}

	poll := s.Poll
	if poll <= 0 {
		poll = 30 * time.Second
	}

	for {
		progressed := s.startWhatFits(ctx, st)
		s.reap(ctx, st)
		s.markUnreachable(st)

		live := st.liveCount()
		if len(st.pending) == 0 && live == 0 {
			return st.out
		}
		if live == 0 && !progressed && !s.anyStartable(st) {
			// Nothing running and nothing can start: report instead of waiting forever.
			for i := range st.pending {
				st.finish(&Outcome{Job: st.pending[i], Err: errors.New("not triggered: dependencies never resolved")})
			}

			return st.out
		}

		select {
		case <-ctx.Done():
			for _, list := range st.active {
				for _, r := range list {
					if !r.abandoned {
						st.out = append(st.out, Outcome{Job: r.job, BuildURL: r.buildURL, Err: ctx.Err()})
					}
				}
			}
			for i := range st.pending {
				st.out = append(st.out, Outcome{Job: st.pending[i], Err: ctx.Err()})
			}

			return st.out
		case <-time.After(poll):
		}
	}
}

// startWhatFits walks pending jobs in plan order and reports whether any was started or resolved.
func (s *Scheduler) startWhatFits(ctx context.Context, st *runState) bool {
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

		if len(st.active[j.Controller]) >= s.Limits[j.Controller].MaxConcurrent {
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
	case err != nil:
		s.notify("error", "failed to trigger %s %s: %v", j.Path, j.Version, err)
		st.finish(&Outcome{Job: *j, Err: err})
	default:
		s.notify("info", "triggered %s %s", j.Path, j.Version)
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
				keep = append(keep, r)
			default:
				st.finish(&outcome)
			}
		}
		st.active[ctrl] = keep
	}
}

// markUnreachable flags a controller once unknown-state jobs fill its whole limit: no capacity can
// ever free up there, so its remaining jobs are reported. Below the limit it keeps being used.
func (s *Scheduler) markUnreachable(st *runState) {
	for ctrl, list := range st.active {
		abandoned := 0
		for _, r := range list {
			if r.abandoned {
				abandoned++
			}
		}
		if abandoned > 0 && abandoned >= s.Limits[ctrl].MaxConcurrent && !st.down[ctrl] {
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
		Err: fmt.Errorf("state unknown after %d failed status queries (last: %w); check %s by hand",
			r.errs, err, firstNonEmpty(r.buildURL, r.queueURL)),
	}, true
}

// ValidateDependencies rejects duplicate job identities (product, version, name), references to
// jobs that are not in the plan and dependency cycles, before anything is triggered.
func ValidateDependencies(jobs []JenkinsJob) error {
	var problems []string
	deps := map[string][]string{}
	for i := range jobs {
		k := jobs[i].key()
		if _, dup := deps[k]; dup {
			problems = append(problems,
				fmt.Sprintf("duplicate job %s %s %q", jobs[i].Product, jobs[i].Version, jobs[i].Name))
			continue
		}
		deps[k] = jobs[i].depKeys()
	}

	for i := range jobs {
		for n, k := range jobs[i].depKeys() {
			if _, ok := deps[k]; !ok {
				problems = append(problems, fmt.Sprintf("%s %s depends on unknown job %q",
					jobs[i].Name, jobs[i].Version, jobs[i].DependsOn[n]))
			}
		}
	}

	if hasCycle(deps) {
		problems = append(problems, "dependency cycle between jobs")
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}

	return nil
}

// hasCycle runs Kahn's algorithm: whatever cannot be ordered is on (or behind) a cycle.
// References to unknown keys are ignored here; ValidateDependencies reports them.
func hasCycle(deps map[string][]string) bool {
	indegree := map[string]int{}
	dependents := map[string][]string{}
	for k, ds := range deps {
		for _, d := range ds {
			if _, ok := deps[d]; ok {
				indegree[k]++
				dependents[d] = append(dependents[d], k)
			}
		}
	}
	var queue []string
	for k := range deps {
		if indegree[k] == 0 {
			queue = append(queue, k)
		}
	}
	ordered := 0
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		ordered++
		for _, d := range dependents[k] {
			indegree[d]--
			if indegree[d] == 0 {
				queue = append(queue, d)
			}
		}
	}

	return ordered < len(deps)
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
