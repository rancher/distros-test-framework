package releasebot

import (
	"context"
	"fmt"
)

// Planner builds the listener's plans: the matrix is re-read and the tags checked on every request.
// capacity is shared by all runs; failures (nil without a broker) triages their failed builds.
func (a *App) Planner(capacity *Capacity, failures *FailureTriage) Planner {
	return func(ctx context.Context, text string) (*Planned, error) {
		return a.planned(ctx, ParseRequest(text), capacity, failures)
	}
}

// planned plans req and wraps it for the listener, including how to rebuild it without some tags
// (when other runs already validate them).
func (a *App) planned(
	ctx context.Context,
	req Request,
	capacity *Capacity,
	failures *FailureTriage,
) (*Planned, error) {
	p, err := a.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	a.PrintPlan(p.Plan, "") // no mode: whether it runs is decided later (allowlist, dry-run)

	run := func(ctx context.Context, rc *RunControl) error {
		return a.Execute(ctx, p, a.runHooks(rc, capacity, failures))
	}

	out := &Planned{
		Summary: planSummary(p.Plan),
		Tags:    append(append([]string{}, req.K3s...), req.RKE2...),
		Run:     run,
		Without: func(ctx context.Context, drop []string) (*Planned, error) {
			return a.planned(ctx, req.Without(drop), capacity, failures)
		},
	}
	if runErr := p.Plan.runnableError(); runErr != nil {
		out.Refusal = runErr.Error()
	}

	return out, nil
}

// Resumer carries on the listener's saved runs after a restart, with the same capacity and triage
// as new runs.
func (a *App) Resumer(capacity *Capacity, failures *FailureTriage) func(context.Context, *RunControl, Progress) error {
	return func(ctx context.Context, rc *RunControl, saved Progress) error {
		return a.Resume(ctx, &saved, a.runHooks(rc, capacity, failures))
	}
}

// runHooks wires a run to its thread: progress, status, stop, commands, help and saving.
func (a *App) runHooks(rc *RunControl, capacity *Capacity, failures *FailureTriage) *hooks {
	h := &hooks{
		Stop: rc.Stop, SetStatus: rc.SetStatus, Capacity: capacity, Owner: rc.ID,
		Commands: rc.Commands, Help: rc.Help, Triage: askTriager, Save: rc.Save, Ready: rc.Ready, Go: rc.Go,
		Notify: func(level, format string, args ...any) {
			a.cfg.Log(level, format, args...)
			if level != "debug" {
				rc.Notify(fmt.Sprintf(format, args...))
			}
		},
	}
	if failures != nil {
		h.Triage, h.Deep = failures.Quick, failures.Full
	}

	return h
}
