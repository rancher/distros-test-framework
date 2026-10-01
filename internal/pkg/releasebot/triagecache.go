package releasebot

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FailureTriage is the bot's triager: a failure already triaged (same log signature, and same job
// for a test failure) reuses that verdict; anything new goes to the broker in quick mode.
type FailureTriage struct {
	// Log returns a build's console text; nil or failing leaves the model to read the log itself.
	Log    func(ctx context.Context, buildURL string) (string, error)
	Broker *BrokerClient
	// Reuse is how long a verdict is reused for the same failure (default 24h).
	Reuse time.Duration
	Now   func() time.Time

	mu   sync.Mutex
	seen map[string]*triaged
}

type triaged struct {
	done    chan struct{}
	d       Decision
	ok      bool
	version string
	at      time.Time
}

// Quick is the Triager used for every failure. A rerun verdict (fresh or reused) only stands
// when this build's failing log also matches a transient-infrastructure rule.
func (f *FailureTriage) Quick(ctx context.Context, o *Outcome) Decision {
	req, fail := f.request(ctx, o, TriageQuick)

	return GateRerun(f.decide(ctx, req, o, reuseKey(o, &fail)), &fail)
}

// reuseKey: an infrastructure failure (no spec failed) is the same across jobs, so one outage that
// fails many jobs is triaged once; a test failure is only the same within its job.
func reuseKey(o *Outcome, fail *Failure) string {
	switch {
	case fail.Signature == "":
		return ""
	case fail.TestFailed:
		return o.Job.Name + "|" + fail.Signature
	default:
		return "infra|" + fail.Signature
	}
}

// decide returns the verdict for this failure: reused under key when one exists, otherwise asked
// from the broker, with concurrent occurrences waiting for one triage.
func (f *FailureTriage) decide(ctx context.Context, req *TriageRequest, o *Outcome, key string) Decision {
	if key == "" {
		d, _ := f.Broker.Ask(ctx, req)
		return d
	}

	for range 3 {
		e, first := f.claim(key, o.Job.Name+" "+o.Job.Version)
		if first {
			e.d, e.ok = f.Broker.Ask(ctx, req)
			if !e.ok {
				f.mu.Lock()
				if f.seen[key] == e {
					delete(f.seen, key) // failed triage: the next occurrence tries again
				}
				f.mu.Unlock()
			}
			close(e.done)

			return e.d
		}
		select {
		case <-e.done:
		case <-ctx.Done():
			return Decision{Summary: "automatic triage canceled"}
		}
		if e.ok {
			return Decision{Rerun: e.d.Rerun, Summary: fmt.Sprintf("same failure as %s, triaged at %s UTC: %s",
				e.version, e.at.UTC().Format("15:04"), e.d.Summary)}
		}
		// That triage failed and its entry is gone: claim again, so one waiter asks for all.
	}
	d, _ := f.Broker.Ask(ctx, req)

	return d
}

// Full is the complete skill with its verifier, for a person's `triage` request; never reused.
func (f *FailureTriage) Full(ctx context.Context, o *Outcome) Decision {
	req, _ := f.request(ctx, o, TriageFull)
	d, _ := f.Broker.Ask(ctx, req)

	return d
}

// claim returns the entry for key, creating it (first=true) unless a fresh one exists.
func (f *FailureTriage) claim(key, version string) (e *triaged, first bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reuse := f.Reuse
	if reuse <= 0 {
		reuse = 24 * time.Hour
	}
	for k, old := range f.seen {
		if f.now().Sub(old.at) > reuse {
			delete(f.seen, k)
		}
	}
	if old := f.seen[key]; old != nil {
		return old, false
	}
	if f.seen == nil {
		f.seen = map[string]*triaged{}
	}
	e = &triaged{done: make(chan struct{}), version: version, at: f.now()}
	f.seen[key] = e

	return e, true
}

func (f *FailureTriage) request(ctx context.Context, o *Outcome, mode string) (req *TriageRequest, fail Failure) {
	req = &TriageRequest{
		Mode: mode, JobName: o.Job.Name, JobPath: o.Job.Path, Product: o.Job.Product, Version: o.Job.Version,
		Result: o.Result, BuildURL: o.BuildURL, Attempt: o.Job.Attempt + 1,
	}
	if f.Log != nil && o.BuildURL != "" {
		if log, err := f.Log(ctx, o.BuildURL); err == nil {
			fail = AnalyzeFailure(log)
			req.Excerpt = fail.Excerpt
		}
	}

	return req, fail
}

func (f *FailureTriage) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}

	return time.Now()
}
