package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// abandon marks a build whose state is unknown: it keeps its slot and is watched, first one
// WatchPoll after since (its state was just lost). It has no result yet: its dependents wait.
func (s *Scheduler) abandon(r *running, ctrl string, since time.Time, err error) {
	r.abandoned, r.since, r.err, r.nextWatch = true, since, err, since.Add(s.watchPoll())
	s.Capacity.abandon(ctrl, s.Owner)
}

// watch checks an unknown build now and then; resolved means its slot is free again: it finished
// (its result then goes through settle like any other), never ran, or was not seen for WatchLimit.
func (s *Scheduler) watch(ctx context.Context, st *runState, ctrl string, r *running) (resolved bool) {
	now := s.now()
	expired := func() bool { return now.Sub(latest(r.since, r.seen)) >= s.watchLimit() }
	if now.Before(r.nextWatch) && !expired() {
		return false
	}
	r.nextWatch = now.Add(s.watchPoll())

	if b := s.Builders[ctrl]; b != nil {
		if r.buildURL == "" && r.queueURL != "" {
			u, err := s.buildFromQueue(ctx, b, r)
			switch {
			case errors.Is(err, errQueueCanceled):
				s.resolve(ctrl, r, "its queue item was canceled, so it never ran")
				s.settle(st, &Outcome{Job: r.job, Err: errQueueCanceled})
				return true
			case err == nil:
				r.seen = now // the queue answered: still waiting to start, or started
				r.buildURL = u
			}
		}
		if r.buildURL != "" {
			if done, result, err := b.Finished(ctx, r.buildURL); err == nil {
				if done {
					s.resolve(ctrl, r, "finished with "+result)
					s.settle(st, &Outcome{Job: r.job, BuildURL: r.buildURL, Result: result})
					return true
				}
				r.seen = now
			}
		}
	}
	if !expired() {
		return false
	}
	s.resolve(ctrl, r, fmt.Sprintf("was not seen for %s; check %s by hand", s.watchLimit(),
		firstNonEmpty(r.buildURL, firstNonEmpty(r.queueURL, r.job.Path))))
	st.finish(&Outcome{Job: r.job, BuildURL: r.buildURL, Err: r.err})

	return true
}

// resolve frees an unknown build's slot and says why in the run's thread.
func (s *Scheduler) resolve(ctrl string, r *running, what string) {
	s.Capacity.releaseAbandoned(ctrl, s.Owner)
	s.notify("info", "%s %s (state unknown since %s UTC) %s; its jenkins slot is free again",
		r.job.Path, r.job.Version, r.since.UTC().Format("15:04"), what)
}

// buildFromQueue asks the queue for the build and, when the queue no longer knows the item (jenkins
// forgets started items after a few minutes, e.g. while the bot restarted), the job's builds.
func (*Scheduler) buildFromQueue(ctx context.Context, b Builder, r *running) (string, error) {
	u, err := b.BuildFromQueue(ctx, r.queueURL)
	if err == nil || errors.Is(err, errQueueCanceled) {
		return u, err
	}
	if finder, ok := b.(queueFinder); ok {
		if found, findErr := finder.buildForQueueItem(ctx, r.job.Path, r.queueURL); findErr == nil && found != "" {
			return found, nil
		}
	}

	return "", err
}

// queueFinder finds the build a queue item became among the job's builds.
type queueFinder interface {
	buildForQueueItem(ctx context.Context, jobPath, queueURL string) (buildURL string, err error)
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}

	return a
}

func (st *runState) watchedCount() int {
	n := 0
	for _, list := range st.active {
		for _, r := range list {
			if r.abandoned {
				n++
			}
		}
	}

	return n
}

// journal hands the run's jobs to Journal when they changed since the last call. An error (the state
// could not be saved) is returned, and the same state is offered again next time.
func (s *Scheduler) journal(st *runState) error {
	if s.Journal == nil {
		return nil
	}
	recs := st.records()
	raw, err := json.Marshal(struct {
		Jobs    []JobRecord
		Stopped bool
	}{recs, st.stopped})
	if err != nil || string(raw) == st.lastJournal {
		return nil
	}
	if err = s.Journal(recs, st.stopped); err != nil {
		if !st.saveFailing {
			st.saveFailing = true
			s.notify("error", "could not save the run state (%v): no job is triggered until it can be saved", err)
		}

		return err
	}
	if st.saveFailing {
		st.saveFailing = false
		s.notify("info", "the run state is saved again; triggering resumes")
	}
	st.lastJournal = string(raw)

	return nil
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}

	return time.Now()
}

func (s *Scheduler) watchPoll() time.Duration {
	if s.WatchPoll > 0 {
		return s.WatchPoll
	}

	return 5 * time.Minute
}

func (s *Scheduler) watchLimit() time.Duration {
	if s.WatchLimit > 0 {
		return s.WatchLimit
	}

	return 4 * time.Hour
}
