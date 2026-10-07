package releasebot

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"
)

// Where a run is, saved so a restarted bot can resume it.
const (
	StageDispatching = "dispatching"
	StageDispatched  = "dispatched"
	StageJobs        = "jobs"
)

// Job states in a saved run.
const (
	jobPending    = "pending"
	jobTriggering = "triggering"
	jobQueued     = "queued"
	jobRunning    = "running"
	jobUnknown    = "unknown"
	jobTriaging   = "triaging"
	jobHeld       = "held"
	jobDone       = "done"
)

// Progress is what a run saves at each change, enough to resume it after a restart.
type Progress struct {
	RequestID  string      `json:"request_id,omitempty"`
	Stage      string      `json:"stage,omitempty"`
	Stopped    bool        `json:"stopped,omitempty"`
	QaseTitles []string    `json:"qase_titles,omitempty"`
	Jobs       []JobRecord `json:"jobs,omitempty"`
}

// clone is a copy sharing nothing with p, so the saved state never changes under the run's feet.
func (p *Progress) clone() Progress {
	c := *p
	c.QaseTitles = slices.Clone(p.QaseTitles)
	c.Jobs = make([]JobRecord, len(p.Jobs))
	for i := range p.Jobs {
		c.Jobs[i] = p.Jobs[i]
		c.Jobs[i].Job.DependsOn = slices.Clone(p.Jobs[i].Job.DependsOn)
		c.Jobs[i].Job.Params = maps.Clone(p.Jobs[i].Job.Params)
	}

	return c
}

func (p *Progress) Resumable() bool {
	return p.Stage == StageDispatched || p.Stage == StageJobs
}

type JobRecord struct {
	Job      JenkinsJob `json:"job"`
	State    string     `json:"state"`
	QueueURL string     `json:"queue_url,omitempty"`
	BuildURL string     `json:"build_url,omitempty"`
	Result   string     `json:"result,omitempty"`
	Err      string     `json:"error,omitempty"`
	Summary  string     `json:"summary,omitempty"`
	Since    time.Time  `json:"since,omitzero"`
}

// records is the run as saved: every job once, in a stable order, so an unchanged run saves nothing.
// While jobs are being triggered they are still in pending: there, only their newer state counts.
func (st *runState) records() []JobRecord {
	var out []JobRecord
	ctrls := make([]string, 0, len(st.active))
	for c := range st.active {
		ctrls = append(ctrls, c)
	}
	sort.Strings(ctrls)
	for _, c := range ctrls {
		for _, r := range st.active[c] {
			rec := JobRecord{Job: r.job, QueueURL: r.queueURL, BuildURL: r.buildURL, State: jobQueued}
			switch {
			case r.abandoned:
				rec.State, rec.Since = jobUnknown, r.since
				if r.err != nil {
					rec.Err = r.err.Error()
				}
			case r.buildURL != "":
				rec.State = jobRunning
			}
			out = append(out, rec)
		}
	}
	if st.triggering != nil {
		out = append(out, JobRecord{Job: *st.triggering, State: jobTriggering})
	}

	out = append(out, outcomeRecords(st.triaging, jobTriaging, nil)...)
	out = append(out, outcomeRecords(st.held, jobHeld, st.helpNote)...)
	taken := map[string]bool{}
	for i := range out {
		taken[out[i].Job.key()] = true
	}

	for i := range st.out {
		taken[st.out[i].Job.key()] = true
	}

	for i := range st.pending {
		if !taken[st.pending[i].key()] {
			out = append(out, JobRecord{Job: st.pending[i], State: jobPending})
		}
	}

	for i := range st.out {
		o := &st.out[i]
		rec := JobRecord{Job: o.Job, State: jobDone, BuildURL: o.BuildURL, Result: o.Result}
		if o.Err != nil {
			rec.Err = o.Err.Error()
		}
		out = append(out, rec)
	}

	return out
}

func outcomeRecords(m map[string]*Outcome, state string, notes map[string]string) []JobRecord {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]JobRecord, 0, len(keys))
	for _, k := range keys {
		o := m[k]
		rec := JobRecord{Job: o.Job, State: state, BuildURL: o.BuildURL, Result: o.Result, Summary: notes[k]}
		if o.Err != nil {
			rec.Err = o.Err.Error()
		}
		out = append(out, rec)
	}

	return out
}

// seed rebuilds a run from saved records: builds on jenkins take their slots back and are followed
// again, triage is redone, and jobs waiting for help are posted again. Done jobs stay done.
func (s *Scheduler) seed(st *runState, recs []JobRecord, now time.Time) {
	for i := range recs {
		rec := &recs[i]
		o := Outcome{Job: rec.Job, BuildURL: rec.BuildURL, Result: rec.Result}
		if rec.Err != "" {
			o.Err = errors.New(rec.Err)
		}
		ctrl := rec.Job.Controller
		_, known := s.Builders[ctrl]
		onJenkins := rec.State == jobQueued || rec.State == jobRunning || rec.State == jobTriggering ||
			rec.State == jobUnknown
		if onJenkins && !known {
			st.finish(&Outcome{
				Job: rec.Job, BuildURL: rec.BuildURL,
				Err: fmt.Errorf("%w: controller %q is no longer configured", errStateUnknown, ctrl),
			})
			continue
		}

		switch rec.State {
		case jobPending:
			st.pending = append(st.pending, rec.Job)
		case jobQueued, jobRunning:
			s.Capacity.take(ctrl)
			st.active[ctrl] = append(st.active[ctrl], &running{job: rec.Job, queueURL: rec.QueueURL, buildURL: rec.BuildURL})
		case jobTriggering, jobUnknown:
			since, reason := rec.Since, error(savedUnknown(rec.Err))
			if rec.State == jobTriggering {
				since, reason = now, fmt.Errorf("%w: the bot restarted while triggering it", errStateUnknown)
			}
			s.Capacity.take(ctrl)
			r := &running{job: rec.Job, queueURL: rec.QueueURL, buildURL: rec.BuildURL}
			s.abandon(r, ctrl, since, reason)
			st.active[ctrl] = append(st.active[ctrl], r)
		case jobTriaging:
			st.retriage = append(st.retriage, o)
		case jobHeld:
			s.askHelp(st, &o, rec.Summary)
		default:
			st.finish(&o)
		}
	}
}

// savedUnknown is a saved "state unknown" reason: its text already says so, and it still counts as
// errStateUnknown (the build may be running).
type savedUnknown string

func (e savedUnknown) Error() string {
	if e == "" {
		return errStateUnknown.Error()
	}

	return string(e)
}

func (savedUnknown) Is(target error) bool { return target == errStateUnknown }
