package releasebot

import (
	"fmt"
	"sort"
	"strings"
)

// Status returns the latest snapshot of the run: where each RC is and what it is doing.
func (s *Scheduler) Status() string {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if s.status == "" {
		return "Starting the jenkins jobs."
	}

	return s.status
}

// rcCounts is where one product+RC stands: the lowest phase with work left and its job counts.
type rcCounts struct {
	phase, waiting, running, triage, help, watched, passed, failed int
	busy                                                           bool
}

func (c *rcCounts) at(phase int) {
	if !c.busy || phase < c.phase {
		c.phase = phase
	}
	c.busy = true
}

func (c *rcCounts) String() string {
	where := "done"
	switch {
	case c.busy && c.phase > 0:
		where = fmt.Sprintf("phase %d", c.phase)
	case c.busy:
		where = "in progress"
	}

	extra := ""
	if c.triage > 0 {
		extra += fmt.Sprintf(", %d in triage", c.triage)
	}
	if c.help > 0 {
		extra += fmt.Sprintf(", %d need help", c.help)
	}
	if c.watched > 0 {
		extra += fmt.Sprintf(", %d in unknown state (watched)", c.watched)
	}

	return fmt.Sprintf("%s; %d running, %d waiting%s, %d passed, %d failed or not run",
		where, c.running, c.waiting, extra, c.passed, c.failed)
}

// countByRC tallies the run per "product version".
func countByRC(st *runState) map[string]*rcCounts {
	per := map[string]*rcCounts{}
	get := func(j *JenkinsJob) *rcCounts {
		k := j.Product + " " + j.Version
		if per[k] == nil {
			per[k] = &rcCounts{}
		}

		return per[k]
	}
	for i := range st.pending {
		c := get(&st.pending[i])
		c.waiting++
		c.at(st.pending[i].Phase)
	}
	for _, list := range st.active {
		for _, r := range list {
			c := get(&r.job)
			if r.abandoned {
				c.watched++
				continue
			}
			c.running++
			c.at(r.job.Phase)
		}
	}
	for _, o := range st.triaging {
		c := get(&o.Job)
		c.triage++
		c.at(o.Job.Phase)
	}
	for _, o := range st.held {
		c := get(&o.Job)
		c.help++
		c.at(o.Job.Phase)
	}
	for i := range st.out {
		c := get(&st.out[i].Job)
		if r := st.out[i].Result; st.out[i].Err == nil && (r == resultSuccess || r == ResultSkipped) {
			c.passed++
		} else {
			c.failed++
		}
	}

	return per
}

// publish stores the snapshot Status returns: one line per product and RC, newest first.
func (s *Scheduler) publish(st *runState) {
	per := countByRC(st)
	keys := make([]string, 0, len(per))
	for k := range per {
		keys = append(keys, k)
	}
	// rke2 before k3s, then newest release first (v1.36.10 before v1.36.9).
	sort.Slice(keys, func(i, j int) bool {
		pi, vi, _ := strings.Cut(keys[i], " ")
		pj, vj, _ := strings.Cut(keys[j], " ")
		if pi != pj {
			return pi > pj
		}

		return newerTag(vi, vj)
	})
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+": "+per[k].String())
	}

	s.statusMu.Lock()
	s.status = strings.Join(lines, "\n")
	s.statusMu.Unlock()
}
