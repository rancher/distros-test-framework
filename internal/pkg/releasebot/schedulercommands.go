package releasebot

import (
	"fmt"
	"sort"
	"strings"
)

// settle handles a job that ended: success is final; a failed build goes to triage; any other
// failure (rejected trigger, canceled queue item) goes straight to the people.
func (s *Scheduler) settle(st *runState, o *Outcome) {
	denied := rerunDenied(o)
	switch {
	case o.Err == nil && o.Result == resultSuccess:
		st.finish(o)
	case o.Err == nil && denied != "":
		// Triage could not change the outcome: no automatic rerun is possible anyway.
		s.askHelp(st, o, "not run: no automatic rerun is possible ("+denied+")")
	case o.Err == nil && s.Triage != nil && !st.stopped:
		st.triaging[o.Job.key()] = o
		go func(o Outcome) {
			r := triageResult{out: o, d: s.Triage(st.triageCtx, &o)}
			select {
			case st.triaged <- r:
			case <-st.quit:
			}
		}(*o)
	default:
		s.askHelp(st, o, "")
	}
}

// decide acts on triage: one automatic rerun when triage asks for it and the fixed rules allow
// it; otherwise the job waits for a person.
func (s *Scheduler) decide(st *runState, r *triageResult) {
	if _, waiting := st.triaging[r.out.Job.key()]; !waiting {
		return // late: stop already finished the job (drain empties triaging)
	}
	delete(st.triaging, r.out.Job.key())
	why := rerunDenied(&r.out)
	switch {
	case r.d.Rerun && why == "":
		j := r.out.Job
		j.Attempt++
		s.notify("info", "rerunning %s %s (triage: %s)", j.Name, j.Version, r.d.Summary)
		st.pending = append([]JenkinsJob{j}, st.pending...)
	default:
		summary := r.d.Summary
		if r.d.Rerun {
			summary += " (no automatic rerun: " + why + ")"
		}
		s.askHelp(st, &r.out, summary)
	}
}

// rerunDenied returns why a failed build must not be rerun automatically, or "".
func rerunDenied(o *Outcome) string {
	switch {
	case o.Job.Attempt > 0:
		return "it was already rerun once"
	case o.Result == "ABORTED":
		return "the build was aborted"
	}

	return ""
}

// askHelp holds a failed job for a person to `retry` or `skip`, or fails it when nobody can answer.
func (s *Scheduler) askHelp(st *runState, o *Outcome, summary string) {
	what := o.Result
	if o.Err != nil {
		what = "error: " + o.Err.Error()
	}
	if o.BuildURL != "" {
		what += " (" + o.BuildURL + ")"
	}
	if summary != "" {
		what += ". Triage: " + summary
	}
	if s.Commands == nil || st.stopped {
		s.notify("warn", "%s %s failed: %s", o.Job.Name, o.Job.Version, what)
		st.finish(o)

		return
	}

	st.held[o.Job.key()] = o
	st.helpNote[o.Job.key()] = summary
	j := o.Job.Name + " " + o.Job.Version
	st.help.push(fmt.Sprintf("Needs help: %s ended with %s. Reply `retry %s` to run it again or `skip %s` to "+
		"count it as passed (`triage %s` for a full analysis); later phases of this RC wait until then.",
		j, what, j, j, j))
}

// apply runs a person's `retry` or `skip` on a job waiting for help and returns the reply.
func (s *Scheduler) apply(st *runState, c *Command) string {
	if st.stopped {
		return "This run was stopped; start a new one to run it again."
	}
	var keys []string
	for k, o := range st.held {
		if o.Job.Name == c.Job && (c.Version == "" || o.Job.Version == c.Version) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	switch len(keys) {
	case 0:
		return fmt.Sprintf("No job waiting for help matches %s %s. Waiting: %s.", c.Job, c.Version, st.heldList())
	case 1:
	default:
		return fmt.Sprintf("Several jobs named %s are waiting; add the RC: %s.", c.Job, st.heldList())
	}

	o := st.held[keys[0]]
	switch c.Action {
	case CommandTriage:
		return s.deepTriage(st, o, c.By)
	case CommandRetry:
		delete(st.held, keys[0])
		j := o.Job
		j.Attempt++
		st.pending = append([]JenkinsJob{j}, st.pending...)

		return fmt.Sprintf("Retrying %s %s, asked by <@%s>.", j.Name, j.Version, c.By)
	case CommandSkip:
		delete(st.held, keys[0])
		st.out = append(st.out, Outcome{Job: o.Job, BuildURL: o.BuildURL, Result: ResultSkipped})
		st.done[o.Job.key()] = resultSuccess

		return fmt.Sprintf("Skipped %s %s, asked by <@%s>: it counts as passed, so the next phase can start.",
			o.Job.Name, o.Job.Version, c.By)
	default:
		return fmt.Sprintf("Unknown action %q; use retry, skip or triage.", c.Action)
	}
}

// deepTriage runs the full triage of a waiting job in the background and posts it; the job keeps
// waiting for `retry` or `skip`.
func (s *Scheduler) deepTriage(st *runState, o *Outcome, by string) string {
	if s.DeepTriage == nil {
		return "Full triage is not available on this bot."
	}
	key := o.Job.key()
	st.deepMu.Lock()
	if st.deep[key] {
		st.deepMu.Unlock()
		return fmt.Sprintf("A full triage of %s %s is already running; its result will be posted here.",
			o.Job.Name, o.Job.Version)
	}
	if st.deep == nil {
		st.deep = map[string]bool{}
	}
	st.deep[key] = true
	st.deepMu.Unlock()

	out := *o
	st.deepWG.Add(1)
	go func() {
		defer st.deepWG.Done()
		defer func() {
			st.deepMu.Lock()
			delete(st.deep, key)
			st.deepMu.Unlock()
		}()
		d := s.DeepTriage(st.deepCtx, &out)
		st.help.push(fmt.Sprintf("Full triage of %s %s (asked by <@%s>): %s", out.Job.Name, out.Job.Version, by,
			d.Summary))
	}()

	return fmt.Sprintf("Running the full triage of %s %s; it takes several minutes and is posted here. "+
		"The job keeps waiting for `retry` or `skip`.", o.Job.Name, o.Job.Version)
}

func (st *runState) heldList() string {
	var names []string
	for _, o := range st.held {
		names = append(names, o.Job.Name+" "+o.Job.Version)
	}
	if len(names) == 0 {
		return "none"
	}
	sort.Strings(names)

	return strings.Join(names, ", ")
}

func stopRequested(stop <-chan struct{}) bool {
	if stop == nil {
		return false
	}
	select {
	case <-stop:
		return true
	default:
		return false
	}
}
