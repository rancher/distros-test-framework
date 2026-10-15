package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/slack"
)

type Planned struct {
	Summary string
	Tags    []string
	Run     func(ctx context.Context, rc *RunControl) error

	// Refusal, when set, is why the plan must not run (it is still shown).
	Refusal string

	// Without rebuilds the plan (workflows, Qase runs, jobs) minus the given tags.
	Without func(ctx context.Context, drop []string) (*Planned, error)
}

type Planner func(ctx context.Context, text string) (*Planned, error)

type RunControl struct {
	ID       string
	Notify   func(string)
	Stop     <-chan struct{}
	Commands <-chan Command
	Help     func(string)

	// Save keeps the run's progress in the bot state, so a restart resumes it.
	Save func(Progress) error

	// Ready and Go hold resumed runs until all of them took their builds' slots back.
	Ready func()
	Go    <-chan struct{}

	mu     sync.Mutex
	status func() string
}

func (c *RunControl) SetStatus(f func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = f
}

func (c *RunControl) Status() string {
	c.mu.Lock()
	f := c.status
	c.mu.Unlock()
	if f == nil {
		return "Starting: workflows and Qase runs first, then the jenkins jobs."
	}

	return f()
}

// Listener runs the release conversation: a mention with RC tags starts the plan at once and
// replies in its thread; `stop`, `status`, `retry`, `skip` and `triage` act on a run.
type Listener struct {
	// Channels are served; in OpenChannels anyone may start runs, elsewhere only Allowed users.
	Channels map[string]bool

	OpenChannels map[string]bool
	BotUserID    string
	Allowed      map[string]bool
	Plan         Planner
	Post         func(ctx context.Context, channel, threadTS, text string) error

	// Resume carries on a run saved before a restart (see ResumeRuns); nil drops saved runs.
	Resume func(ctx context.Context, rc *RunControl, saved Progress) error

	// ProgressInterval groups a run's progress lines into one message per interval (default 10s),
	// so a busy run does not hit Slack's per-channel posting limit.
	ProgressInterval time.Duration
	PostTimeout      time.Duration
	DryRun           bool

	// Planning times out after 3m by default; separate workers keep stop/status responsive.
	PlanTimeout time.Duration

	// Default 2 workers; excess mentions wait in the bounded queue below.
	MaxPlanning int

	// Default 4 queued plans; further mentions receive "busy" immediately.
	QueuedPlans int

	// StatePath keeps every run in progress, saved at each change, so a restart (even SIGKILL)
	// resumes them in their threads. Empty keeps it in memory only.
	StatePath string
	Now       func() time.Time

	mu     sync.Mutex
	active map[string]*activeRun
	saved  map[string]*savedRun

	runs      sync.WaitGroup
	planQueue chan planRequest
	queueOnce sync.Once
}

type planRequest struct {
	ctx context.Context
	e   *slack.Event
}

type activeRun struct {
	channel, ts, user string
	tags              []string
	started           time.Time
	ctl               *RunControl
	stop              chan struct{}
	stopped           bool
	commands          chan Command
}

func threadKey(channel, ts string) string { return channel + "|" + ts }

// savedRun is a run in progress as StatePath keeps it.
type savedRun struct {
	Channel  string    `json:"channel"`
	TS       string    `json:"ts"`
	User     string    `json:"user"`
	Tags     []string  `json:"tags"`
	Started  time.Time `json:"started"`
	Progress Progress  `json:"progress"`
}

// stateVersion is the format of StatePath; a newer one is refused rather than misread.
const stateVersion = 1

type listenerState struct {
	Version int                  `json:"version"`
	Runs    map[string]*savedRun `json:"runs,omitempty"`
}

// Restore loads the runs StatePath kept; ResumeRuns then carries them on.
func (l *Listener) Restore() error {
	if l.StatePath == "" {
		return nil
	}

	raw, err := os.ReadFile(l.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var st listenerState
	if err = json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("release bot state %s: %w", l.StatePath, err)
	}
	if st.Version > stateVersion {
		return fmt.Errorf("release bot state %s has format %d, newer than this bot's %d", l.StatePath,
			st.Version, stateVersion)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.saved = st.Runs

	return nil
}

// ResumeRuns carries on, each in its own thread, the runs the previous process left.
func (l *Listener) ResumeRuns(ctx context.Context) int {
	if l.Resume == nil {
		return 0
	}
	l.mu.Lock()
	keys := make([]string, 0, len(l.saved))
	for k := range l.saved {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	var resumed []*activeRun
	var dropped []*savedRun

	goAhead := make(chan struct{})
	var ready sync.WaitGroup
	for _, k := range keys {
		sr := l.saved[k]
		if !sr.Progress.Resumable() {
			dropped = append(dropped, sr)
			delete(l.saved, k)
			continue
		}
		run := l.newRunLocked(sr.Channel, sr.TS, sr.User, sr.Tags, sr.Started)
		run.stopped = sr.Progress.Stopped
		ready.Add(1)
		run.ctl.Ready, run.ctl.Go = sync.OnceFunc(ready.Done), goAhead
		resumed = append(resumed, run)
	}
	_ = l.saveLocked()
	l.mu.Unlock()

	for _, sr := range dropped {
		l.postDetached(ctx, sr.Channel, sr.TS, "<@"+sr.User+"> The bot restarted before this run had dispatched all "+
			"its workflows, so it cannot tell what started and does not resume it. Check the release-checks "+
			"and Qase workflow runs, then ask again.")
	}
	for _, run := range resumed {
		saved := l.savedProgress(run) // the run's own copy: it changes it (Qase run ids) as it goes
		l.postDetached(ctx, run.channel, run.ts, "The bot restarted; resuming this run where it was "+
			"(builds are followed again, failed jobs waiting for help are listed again).")
		l.start(ctx, run, &Planned{Run: func(ctx context.Context, rc *RunControl) error {
			return l.Resume(ctx, rc, saved)
		}})
	}

	waitReady(&ready, readyTimeout)
	close(goAhead)

	return len(resumed)
}

// readyTimeout bounds how long resumed runs wait for each other before triggering anyway.
const readyTimeout = 2 * time.Minute

func waitReady(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (l *Listener) SavedRuns() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.saved)
}

func (l *Listener) savedProgress(run *activeRun) Progress {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.saved[threadKey(run.channel, run.ts)].Progress.clone()
}

// saveLocked writes the state atomically. Callers hold l.mu.
func (l *Listener) saveLocked() error {
	if l.StatePath == "" {
		return nil
	}
	raw, err := json.Marshal(listenerState{Version: stateVersion, Runs: l.saved})
	if err != nil {
		return err
	}

	if mkErr := os.MkdirAll(filepath.Dir(l.StatePath), 0o700); mkErr != nil {
		return mkErr
	}
	tmp := l.StatePath + ".tmp"
	if writeErr := os.WriteFile(tmp, raw, 0o600); writeErr != nil {
		return writeErr
	}

	return os.Rename(tmp, l.StatePath)
}

// Handle processes one message event. Plans run in the background; Wait blocks until they end.
func (l *Listener) Handle(ctx context.Context, e *slack.Event) {
	// thread_broadcast is a thread reply also sent to the channel ("Also send to ..."); other
	// subtypes (edits, joins, bot messages) are not requests.
	if !l.Channels[e.Channel] || e.BotID != "" || (e.Subtype != "" && e.Subtype != "thread_broadcast") ||
		e.User == "" || e.User == l.BotUserID {
		return
	}

	reply := e.ThreadTS != "" && e.ThreadTS != e.TS
	switch {
	case !reply && strings.Contains(e.Text, "<@"+l.BotUserID+">"):
		l.enqueuePlan(ctx, e)
	case reply:
		l.onReply(ctx, e)
	}
}

func (l *Listener) Wait() { l.runs.Wait() }

func (l *Listener) mayRun(e *slack.Event) bool {
	return l.OpenChannels[e.Channel] || l.Allowed[e.User]
}

// enqueuePlan hands a request to the planning workers; with the queue full it answers "busy"
// at once instead of piling up work (anyone in a served channel can mention the bot).
func (l *Listener) enqueuePlan(ctx context.Context, e *slack.Event) {
	l.queueOnce.Do(func() {
		workers, queued := l.MaxPlanning, l.QueuedPlans
		if workers <= 0 {
			workers = 2
		}
		if queued <= 0 {
			queued = 4
		}
		l.planQueue = make(chan planRequest, queued)
		for range workers {
			go l.planWorker(ctx)
		}
	})

	l.runs.Add(1)
	select {
	case l.planQueue <- planRequest{ctx: ctx, e: e}:
	default:
		l.runs.Done()
		l.post(ctx, e.Channel, e.TS, "The bot is busy building other release plans; ask again in a few minutes.")
	}
}

// planWorker builds queued plans until ctx ends, then drops what is still queued.
func (l *Listener) planWorker(ctx context.Context) {
	for {
		select {
		case r := <-l.planQueue:
			l.onRequest(r.ctx, r.e)
			l.runs.Done()
		case <-ctx.Done():
			for {
				select {
				case <-l.planQueue:
					l.runs.Done()
				default:
					return
				}
			}
		}
	}
}

// bounded builds a plan within PlanTimeout.
func (l *Listener) bounded(ctx context.Context, build func(context.Context) (*Planned, error)) (*Planned, error) {
	timeout := l.PlanTimeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return build(pctx)
}

func (l *Listener) onRequest(ctx context.Context, e *slack.Event) {
	planned, err := l.bounded(ctx, func(c context.Context) (*Planned, error) { return l.Plan(c, e.Text) })
	if err != nil {
		l.post(ctx, e.Channel, e.TS, "Could not build a release plan: "+err.Error())
		return
	}

	switch {
	case l.DryRun:
		l.post(ctx, e.Channel, e.TS, planned.Summary+"\n\nDry-run only: this bot was started without "+
			"`-dry-run=false`, so plans are never executed.")
		return
	case !l.mayRun(e):
		l.post(ctx, e.Channel, e.TS, planned.Summary+"\n\nDry-run only: <@"+e.User+"> is not on the list of "+
			"users allowed to run release plans in this channel.")
		return
	}

	var notes []string
	for attempt := 0; ; attempt++ {
		// Duplicates go first: a problem of a tag another run validates must not refuse the others.
		l.mu.Lock()
		dup, why := l.duplicatesLocked(planned.Tags)
		l.mu.Unlock()
		var run *activeRun
		if len(dup) == 0 {
			if planned.Refusal != "" {
				l.post(ctx, e.Channel, e.TS, planned.Summary+"\n\nNot started: "+planned.Refusal)
				return
			}

			// admit checks again: another run may have started those tags meanwhile.
			run, dup, why = l.admit(e, planned)
		}

		switch {
		case run != nil:
			notes = append(notes, "Started by <@"+e.User+">: phase 1 is starting now; progress follows in this "+
				"thread. Reply `status` for where it is, or `stop` to start no new jobs (running builds are "+
				"followed until they finish).")
			l.post(ctx, e.Channel, e.TS, planned.Summary+"\n\n"+strings.Join(notes, "\n"))
			l.start(ctx, run, planned)

			return
		case len(dup) == 0 || len(dup) == len(planned.Tags) || planned.Without == nil || attempt == 2:
			l.post(ctx, e.Channel, e.TS, planned.Summary+"\n\nNot started: "+why)
			return
		}

		// Some tags are already running elsewhere: run the rest, rebuilt without them.
		notes = append(notes, "Left out: "+why)
		next, err := l.bounded(ctx, func(c context.Context) (*Planned, error) { return planned.Without(c, dup) })
		if err != nil {
			l.post(ctx, e.Channel, e.TS, "Could not build a release plan without "+strings.Join(dup, ", ")+": "+
				err.Error())
			return
		}
		planned = next
	}
}

// admit registers the run under the lock. Otherwise it says why not (why) and, when RC tags are
// already being validated by other runs, which ones (dup).
func (l *Listener) admit(e *slack.Event, planned *Planned) (run *activeRun, dup []string, why string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if dup, why = l.duplicatesLocked(planned.Tags); len(dup) > 0 {
		return nil, dup, why
	}

	run = l.newRunLocked(e.Channel, e.TS, e.User, planned.Tags, l.now())
	// Recorded before it starts, so even a killed process leaves the run to resume.
	if err := l.saveLocked(); err != nil {
		key := threadKey(e.Channel, e.TS)
		delete(l.active, key)
		delete(l.saved, key)
		return nil, nil, "could not record the run in the bot state (" + err.Error() + ")."
	}

	return run, nil, ""
}

// newRunLocked registers a run (new or resumed) with its controls and saved state. Callers hold l.mu.
func (l *Listener) newRunLocked(channel, ts, user string, tags []string, started time.Time) *activeRun {
	key := threadKey(channel, ts)
	stop := make(chan struct{})
	commands := make(chan Command, 4)
	run := &activeRun{
		channel: channel, ts: ts, user: user, tags: tags, started: started,
		ctl: &RunControl{ID: key, Stop: stop, Commands: commands}, stop: stop, commands: commands,
	}

	run.ctl.Save = func(p Progress) error {
		l.mu.Lock()
		defer l.mu.Unlock()
		sr := l.saved[key]
		if sr == nil {
			return nil
		}
		// `stop` is kept even before the scheduler sees it
		p.Stopped = p.Stopped || run.stopped

		// the run keeps changing its own (Qase run ids, params)
		sr.Progress = p.clone()

		return l.saveLocked()
	}
	if l.active == nil {
		l.active = map[string]*activeRun{}
	}
	if l.saved == nil {
		l.saved = map[string]*savedRun{}
	}
	l.active[key] = run
	if l.saved[key] == nil {
		l.saved[key] = &savedRun{Channel: channel, TS: ts, User: user, Tags: tags, Started: started}
	}

	return run
}

// duplicatesLocked returns the tags other runs already validate and where. Callers hold l.mu.
func (l *Listener) duplicatesLocked(tags []string) (dup []string, why string) {
	var where []string
	add := func(otherTags []string, channel, user string, started time.Time) {
		var mine []string
		for _, t := range tags {
			if slices.Contains(otherTags, t) {
				mine = append(mine, t)
			}
		}
		if len(mine) > 0 {
			dup = append(dup, mine...)
			where = append(where, fmt.Sprintf("%s already running in the <#%s> thread started by <@%s> at %s UTC",
				strings.Join(mine, ", "), channel, user, started.UTC().Format("15:04")))
		}
	}

	for _, other := range l.active {
		add(other.tags, other.channel, other.user, other.started)
	}

	// A saved run not running here (its resume failed) still owns its tags: its builds may run.
	for key, sr := range l.saved {
		if l.active[key] == nil {
			add(sr.Tags, sr.Channel, sr.User, sr.Started)
		}
	}
	if len(dup) == 0 {
		return nil, ""
	}
	sort.Strings(where)

	return dup, strings.Join(where, "; ") + "."
}

func (l *Listener) onReply(ctx context.Context, e *slack.Event) {
	words := strings.Fields(e.Text)
	if len(words) == 0 {
		return
	}

	if cmd := strings.ToLower(words[0]); cmd == CommandRetry || cmd == CommandSkip || cmd == CommandTriage {
		l.onJobCommand(ctx, e, cmd, words[1:])
		return
	}

	switch strings.ToLower(strings.TrimSpace(e.Text)) {
	case "stop":
		l.onStop(ctx, e)
	case "status":
		l.mu.Lock()
		run := l.active[threadKey(e.Channel, e.ThreadTS)]
		l.mu.Unlock()
		if run != nil {
			l.post(ctx, e.Channel, e.ThreadTS, run.ctl.Status())
		}
	}
}

// onJobCommand passes `retry|skip|triage <job> [rc]` to the run's scheduler and posts its
// answer; it does not wait here, so other events keep flowing.
func (l *Listener) onJobCommand(ctx context.Context, e *slack.Event, action string, args []string) {
	l.mu.Lock()
	run := l.active[threadKey(e.Channel, e.ThreadTS)]
	l.mu.Unlock()
	switch {
	case run == nil:
		return
	case !l.mayRun(e):
		l.post(ctx, e.Channel, e.ThreadTS, "<@"+e.User+"> is not allowed to retry, skip or triage jobs.")
		return
	case len(args) == 0 || len(args) > 2:
		l.post(ctx, e.Channel, e.ThreadTS, "Usage: `retry <job> [rc]`, `skip <job> [rc]` or `triage <job> [rc]`, "+
			"as in the help message.")
		return
	}

	reply := make(chan string, 1)
	c := Command{Action: action, Job: args[0], By: e.User, Reply: reply}
	if len(args) == 2 {
		c.Version = args[1]
	}
	select {
	case run.commands <- c:
	default:
		l.post(ctx, e.Channel, e.ThreadTS, "The run is busy with earlier commands; try again in a minute.")
		return
	}
	go func() {
		select {
		case text := <-reply:
			l.postDetached(ctx, e.Channel, e.ThreadTS, text)
		case <-time.After(5 * time.Minute):
			l.postDetached(ctx, e.Channel, e.ThreadTS, "No answer from the run yet; check `status`.")
		}
	}()
}

func (l *Listener) onStop(ctx context.Context, e *slack.Event) {
	l.mu.Lock()
	run := l.active[threadKey(e.Channel, e.ThreadTS)]
	switch {
	case run == nil:
		l.mu.Unlock()
		return
	case !l.mayRun(e):
		l.mu.Unlock()
		l.post(ctx, e.Channel, e.ThreadTS, "<@"+e.User+"> is not allowed to stop release plans.")
		return
	case run.stopped:
		l.mu.Unlock()
		return
	}

	run.stopped = true
	close(run.stop)
	if sr := l.saved[threadKey(e.Channel, e.ThreadTS)]; sr != nil {
		sr.Progress.Stopped = true
		_ = l.saveLocked() // the scheduler saves it too once it stops
	}
	l.mu.Unlock()
	l.post(ctx, e.Channel, e.ThreadTS, "Stopped by <@"+e.User+">: no new job will start; running builds are "+
		"followed until they finish.")
}

// start runs an admitted plan in the background.
func (l *Listener) start(ctx context.Context, run *activeRun, planned *Planned) {
	l.runs.Add(1)
	go func() {
		defer l.runs.Done()

		// Posts outlive ctx so the last progress and the outcome still reach Slack on shutdown.
		post := func(text string) { l.postDetached(ctx, run.channel, run.ts, text) }
		progress := newProgressBatcher(post, l.progressInterval())
		run.ctl.Notify = progress.add
		run.ctl.Help = func(text string) { post("<@" + run.user + "> " + text) }
		err := planned.Run(ctx, run.ctl)
		progress.close()
		if run.ctl.Ready != nil {
			run.ctl.Ready() // a resume that ended early must not hold the others back
		}

		// Cut by shutdown, the run stays saved and the next bot process resumes it; one that ended
		// on its own just as the bot stopped is over. A resume that could not be set up stays too.
		shutdown := ctx.Err() != nil && errors.Is(err, context.Canceled)
		notResumed := errors.Is(err, errResumeFailed)
		l.mu.Lock()
		key := threadKey(run.channel, run.ts)
		delete(l.active, key)
		if !shutdown && !notResumed {
			delete(l.saved, key)
		}
		_ = l.saveLocked()
		l.mu.Unlock()

		switch {
		case shutdown:
			post("The bot is stopping; this run is saved and resumes here when the bot starts again.")
		case notResumed:
			post("<@" + run.user + "> " + err.Error() + "\nThe run stays saved, its builds on jenkins keep their " +
				"slots and its RC tags stay taken; fix the cause and restart the bot to resume it here.")
		case errors.Is(err, errUnreconciledBuilds):
			post("Release plan finished; some builds stayed in unknown state until the bot stopped watching " +
				"them: " + err.Error() + "\nCheck them on jenkins before running those jobs again.")
		case errors.Is(err, errPartialRun):
			post("Release plan stopped part-way: " + err.Error() + "\nAsking again for the same tags may " +
				"dispatch workflows, create Qase runs or trigger builds again: check what already started first.")
		case err != nil:
			post("Release plan finished with errors: " + err.Error())
		default:
			post("Release plan finished.")
		}
	}()
}

// post ignores the error: Post implementations log their own failures, and a failed reply must
// not stop the conversation or a running plan.
func (l *Listener) post(ctx context.Context, channel, threadTS, text string) {
	_ = l.Post(ctx, channel, threadTS, text)
}

// postDetached posts with its own timeout, independent of ctx's cancellation and of how long the
// run has been going.
func (l *Listener) postDetached(ctx context.Context, channel, threadTS, text string) {
	timeout := l.PostTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	postCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	l.post(postCtx, channel, threadTS, text)
}

func (l *Listener) progressInterval() time.Duration {
	if l.ProgressInterval > 0 {
		return l.ProgressInterval
	}

	return 10 * time.Second
}

func (l *Listener) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}

	return time.Now()
}

// progressBatcher groups progress lines and posts them from one goroutine, as one message per
// interval or sooner when maxBatchLines accumulate; close flushes what is left.
type progressBatcher struct {
	mu    sync.Mutex
	lines []string
	flush func(string)
	full  chan struct{}
	stop  chan struct{}
	done  chan struct{}
}

const maxBatchLines = 40

func newProgressBatcher(flush func(string), interval time.Duration) *progressBatcher {
	b := &progressBatcher{
		flush: flush, full: make(chan struct{}, 1), stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go func() {
		defer close(b.done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				b.send()
			case <-b.full:
				b.send()
			case <-b.stop:
				b.send()
				return
			}
		}
	}()

	return b
}

// add never posts itself: it only queues the line (and wakes the batcher when a batch is full),
// so the caller (the scheduler) never waits on Slack and batches keep their order.
func (b *progressBatcher) add(line string) {
	b.mu.Lock()
	b.lines = append(b.lines, line)
	full := len(b.lines) >= maxBatchLines
	b.mu.Unlock()
	if full {
		select {
		case b.full <- struct{}{}:
		default:
		}
	}
}

func (b *progressBatcher) send() {
	b.mu.Lock()
	lines := b.lines
	b.lines = nil
	b.mu.Unlock()
	if len(lines) > 0 {
		b.flush(strings.Join(lines, "\n"))
	}
}

func (b *progressBatcher) close() {
	close(b.stop)
	<-b.done
}

// errAlreadyRunning means another bot process holds the instance lock.
var errAlreadyRunning = errors.New("another release bot is already running")

// AcquireInstanceLock takes an exclusive, non-blocking flock on path and keeps it until release;
// the kernel drops it when the process exits, so a crash never leaves a stale lock.
func AcquireInstanceLock(path string) (release func(), err error) {
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		return nil, mkErr
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w (lock %s)", errAlreadyRunning, path)
		}

		return nil, err
	}

	return func() { _ = f.Close() }, nil
}
