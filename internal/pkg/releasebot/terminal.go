package releasebot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TerminalRun drives one run from a terminal as a Slack thread does: commands come from In, replies go
// to Out, and the progress is saved to StatePath for -resume.
type TerminalRun struct {
	StatePath string
	// nil: no commands, so a job that fails just fails
	In  io.Reader
	Out io.Writer
	// who answers, in the run's messages
	User string

	// LimitFor lowers a controller's limit before each trigger (Scheduler.LimitFor); may be nil.
	LimitFor func(controller string, configured, ours int) int

	mu   sync.Mutex
	last *SavedTerminalRun
}

// Last is the run's progress as last saved (its jobs, builds and results), also once the file is gone.
func (t *TerminalRun) Last() Progress {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		return Progress{}
	}

	return t.last.Progress.clone()
}

// SavedTerminalRun is what StatePath keeps.
type SavedTerminalRun struct {
	Version  int       `json:"version"`
	Request  string    `json:"request"`
	Started  time.Time `json:"started"`
	Progress Progress  `json:"progress"`
}

// Saved returns the run StatePath keeps, or nil when there is none.
func (t *TerminalRun) Saved() (*SavedTerminalRun, error) {
	raw, readErr := os.ReadFile(t.StatePath)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, nil
	}
	if readErr != nil {
		return nil, readErr
	}

	var saved SavedTerminalRun
	if decodeErr := json.Unmarshal(raw, &saved); decodeErr != nil {
		return nil, fmt.Errorf("read the saved run %s: %w", t.StatePath, decodeErr)
	}
	if saved.Version != stateVersion {
		return nil, fmt.Errorf("the saved run %s has format %d, this releasebot reads %d", t.StatePath,
			saved.Version, stateVersion)
	}

	return &saved, nil
}

// Execute runs a prepared plan for request.
func (t *TerminalRun) Execute(ctx context.Context, a *App, p *prepared, request string, failures *FailureTriage) error {
	saved := &SavedTerminalRun{Version: stateVersion, Request: request, Started: time.Now().UTC()}

	return t.drive(ctx, saved, func(rc *RunControl) error {
		return a.Execute(ctx, p, a.runHooks(rc, nil, failures))
	})
}

// Resume carries on the run a previous CLI saved.
func (t *TerminalRun) Resume(ctx context.Context, a *App, saved *SavedTerminalRun, failures *FailureTriage) error {
	t.print("Resuming %q, started %s UTC.", saved.Request, saved.Started.Format("2006-01-02 15:04"))

	return t.drive(ctx, saved, func(rc *RunControl) error {
		progress := saved.Progress.clone()

		return a.Resume(ctx, &progress, a.runHooks(rc, nil, failures))
	})
}

// drive wires the run to the terminal. Cut by ctx (Ctrl-C), or not set up again, the run stays saved;
// once it ends on its own the file goes.
func (t *TerminalRun) drive(ctx context.Context, saved *SavedTerminalRun, body func(*RunControl) error) error {
	stop := make(chan struct{})
	commands := make(chan Command, 4)
	var stopOnce sync.Once
	t.mu.Lock()
	t.last = saved
	t.mu.Unlock()
	rc := &RunControl{
		ID: saved.Progress.RequestID, Stop: stop, Notify: func(string) {}, LimitFor: t.LimitFor,
		Help: func(text string) { t.print("\aNEEDS HELP: %s", text) },
		Save: func(p Progress) error {
			t.mu.Lock()
			defer t.mu.Unlock()
			// A typed stop is kept before the scheduler sees it; the copy is the run's own to change.
			p.Stopped = p.Stopped || saved.Progress.Stopped
			saved.Progress = p.clone()

			return t.writeLocked(saved)
		},
	}
	stopRun := func() {
		stopOnce.Do(func() {
			close(stop)
			t.mu.Lock()
			// a restart right after keeps the run stopped
			saved.Progress.Stopped = true
			_ = t.writeLocked(saved)
			t.mu.Unlock()
		})
	}
	if t.In != nil {
		rc.Commands = commands
		go t.readCommands(rc, commands, stopRun)
	}

	err := body(rc)

	shutdown := ctx.Err() != nil && errors.Is(err, context.Canceled)
	notResumed := errors.Is(err, errResumeFailed)
	t.mu.Lock()
	stage := saved.Progress.Stage
	t.mu.Unlock()
	uncertain := uncertainOutcome(err, stage)
	if !shutdown && !notResumed && !uncertain {
		if removeErr := os.Remove(t.StatePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			t.print("could not remove the finished run's state %s: %v", t.StatePath, removeErr)
		}
	}

	t.report(err, shutdown, notResumed, uncertain)
	if shutdown {
		return ErrInterrupted
	}

	return err
}

// ErrInterrupted: the run was cut (Ctrl-C) and stays saved for -resume, as the terminal already said.
var ErrInterrupted = errors.New("run interrupted and saved for -resume")

// uncertainOutcome: workflows may have been dispatched or builds may still run, so the state stays as
// the guard against a duplicate until a person checks and removes it.
func uncertainOutcome(err error, stage string) bool {
	dispatching := stage == StageDispatching && errors.Is(err, errPartialRun)

	return dispatching || errors.Is(err, errNotResumable) || errors.Is(err, errUnreconciledBuilds)
}

func (t *TerminalRun) report(err error, shutdown, notResumed, uncertain bool) {
	switch {
	case shutdown:
		t.print("Interrupted: the run is saved in %s and its builds on jenkins keep running. Carry it on "+
			"with the same command plus -resume (no new request needed).", t.StatePath)
	case notResumed:
		t.print("%v\nThe run stays saved in %s; fix the cause and start again with -resume.", err, t.StatePath)
	case errors.Is(err, errUnreconciledBuilds):
		t.print("Release plan finished; some builds stayed in unknown state: %v\nThe run stays saved in %s: "+
			"-resume watches them again, or remove the file once you checked them on jenkins.", err, t.StatePath)
	case uncertain:
		t.print("%v\nIts workflows may have been dispatched, so the run stays saved in %s: check GitHub "+
			"Actions and Qase for this request, then remove that file before asking again.", err, t.StatePath)
	case errors.Is(err, errPartialRun):
		t.print("Release plan stopped part-way: %v\nAsking again for the same tags may dispatch workflows, "+
			"create Qase runs or trigger builds again: check what already started first.", err)
	case err != nil:
		t.print("Release plan finished with errors: %v", err)
	default:
		t.print("Release plan finished.")
	}
}

// readCommands takes the terminal's lines as a run's thread replies.
func (t *TerminalRun) readCommands(rc *RunControl, commands chan<- Command, stopRun func()) {
	usage := "Commands: status, stop, retry <job> [rc], skip <job> [rc], triage <job> [rc]."
	lines := bufio.NewScanner(t.In)
	for lines.Scan() {
		words := strings.Fields(lines.Text())
		if len(words) == 0 {
			continue
		}

		switch action := strings.ToLower(words[0]); {
		case len(words) == 1 && action == "status":
			t.print("%s", rc.Status())
		case len(words) == 1 && action == "stop":
			stopRun()
			t.print("Stopped: no new job will start; running builds are followed until they finish.")
		case (action == CommandRetry || action == CommandSkip || action == CommandTriage) && len(words) <= 3 &&
			len(words) >= 2:
			t.send(commands, action, words[1:])
		default:
			t.print("%s", usage)
		}
	}
}

// send passes a job command to the scheduler and prints its answer when it comes.
func (t *TerminalRun) send(commands chan<- Command, action string, args []string) {
	reply := make(chan string, 1)
	c := Command{Action: action, Job: args[0], By: t.User, Reply: reply}
	if len(args) == 2 {
		c.Version = args[1]
	}

	select {
	case commands <- c:
	default:
		t.print("The run is busy with earlier commands; try again in a minute.")
		return
	}

	go func() {
		select {
		case text := <-reply:
			t.print("%s", text)
		case <-time.After(5 * time.Minute):
			t.print("No answer from the run yet; check status.")
		}
	}()
}

// writeLocked replaces StatePath durably; callers hold t.mu.
func (t *TerminalRun) writeLocked(saved *SavedTerminalRun) error {
	raw, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(t.StatePath), 0o700); mkErr != nil {
		return mkErr
	}

	return writeDurable(t.StatePath, raw)
}

func (t *TerminalRun) print(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(t.Out, format+"\n", args...)
}

// LocalTriage runs the triage in this process, one at a time as the broker does: on a captain's laptop
// the CLI and the sandbox belong to the same user, so no spool is needed.
type LocalTriage struct {
	Triage TriageFunc
	// Wait bounds the wait for the triage slot, like the broker's -triage-wait; 0 waits as long as ctx.
	Wait time.Duration

	once sync.Once
	slot chan struct{}
}

func (l *LocalTriage) Ask(ctx context.Context, req *TriageRequest) (Decision, bool) {
	l.once.Do(func() { l.slot = make(chan struct{}, 1) })
	waitCtx := ctx
	if l.Wait > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, l.Wait)
		defer cancel()
	}

	select {
	case l.slot <- struct{}{}:
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return Decision{Summary: "automatic triage canceled"}, false
		}

		return Decision{Summary: fmt.Sprintf("automatic triage unavailable: another triage held it for %s", l.Wait)}, false
	}
	defer func() { <-l.slot }()

	v, err := l.Triage(ctx, req)
	if err != nil {
		v = &Verdict{Error: err.Error()}
	}

	return v.Decision(), v.Error == ""
}
