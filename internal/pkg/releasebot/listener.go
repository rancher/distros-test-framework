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
	mu       sync.Mutex
	status   func() string
}

// SetStatus registers what `status` replies with (the scheduler's snapshot).
func (c *RunControl) SetStatus(f func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = f
}

// Status returns the run's current snapshot.
func (c *RunControl) Status() string {
	c.mu.Lock()
	f := c.status
	c.mu.Unlock()
	if f == nil {
		return "Starting: workflows and Qase runs first, then the Jenkins jobs."
	}

	return f()
}

// Listener runs the release conversation: a mention with RC tags starts the plan at once and
// replies in its thread; `stop`, `status` and `unblock` act on a run (docs/releasebot.md).
type Listener struct {
	// Channels are served; in OpenChannels anyone may start runs, elsewhere only Allowed users.
	Channels     map[string]bool
	OpenChannels map[string]bool
	BotUserID    string
	Allowed      map[string]bool
	Plan         Planner
	Post         func(ctx context.Context, channel, threadTS, text string) error

	// Capacity is shared with the runs' schedulers; `unblock` frees the slots its run's unknown
	// builds still hold. May be nil.
	Capacity *Capacity

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

	// StatePath persists the block state and in-progress markers, so a restart (even SIGKILL)
	// mid-run keeps new runs blocked until someone checks Jenkins. Empty keeps it in memory only.
	StatePath string
	Now       func() time.Time

	mu     sync.Mutex
	active map[string]*activeRun // by thread key

	// blocks, by run thread: runs that left builds in unknown state or stopped part-way. No new
	// run starts while any is left; `unblock` in a run's thread clears that run's block only.
	blocks    map[string]string
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

// listenerState is what StatePath holds.
type listenerState struct {
	Blocks  map[string]string `json:"blocks,omitempty"`  // thread key -> reason
	Running []string          `json:"running,omitempty"` // thread keys
}

// Restore loads StatePath. A run that was in progress when the previous process stopped becomes a
// block: its builds may still be running on Jenkins.
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

	l.mu.Lock()
	defer l.mu.Unlock()
	l.blocks = st.Blocks
	for _, key := range st.Running {
		if _, ok := l.blocks[key]; !ok {
			_, ts, _ := strings.Cut(key, "|")
			l.block(key, "the previous bot process stopped during the run in thread "+ts+
				"; its builds may still be running")
		}
	}

	return l.saveLocked()
}

// block records a run's block. Callers hold l.mu.
func (l *Listener) block(key, reason string) {
	if l.blocks == nil {
		l.blocks = map[string]string{}
	}
	l.blocks[key] = reason
}

// Blocked describes the blocks left, if any: one reason per run, oldest thread first.
func (l *Listener) Blocked() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.blockedLocked()
}

func (l *Listener) blockedLocked() string {
	keys := make([]string, 0, len(l.blocks))
	for k := range l.blocks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	reasons := make([]string, 0, len(keys))
	for _, k := range keys {
		_, ts, _ := strings.Cut(k, "|")
		reasons = append(reasons, "thread "+ts+": "+l.blocks[k])
	}

	return strings.Join(reasons, "; ")
}

// saveLocked writes the state atomically, with every active run marked. Callers hold l.mu.
func (l *Listener) saveLocked() error {
	if l.StatePath == "" {
		return nil
	}
	st := listenerState{Blocks: l.blocks}
	for k := range l.active {
		st.Running = append(st.Running, k)
	}
	sort.Strings(st.Running)
	raw, err := json.Marshal(st)
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

	if len(l.blocks) > 0 {
		return nil, nil, fmt.Sprintf("new plans are blocked because %d earlier runs did not finish cleanly (%s). "+
			"Check what they started, then reply `unblock` in each run's thread and ask again.",
			len(l.blocks), l.blockedLocked())
	}
	if dup, why = l.duplicatesLocked(planned.Tags); len(dup) > 0 {
		return nil, dup, why
	}

	stop := make(chan struct{})
	commands := make(chan Command, 4)
	key := threadKey(e.Channel, e.TS)
	run = &activeRun{
		channel: e.Channel, ts: e.TS, user: e.User, tags: planned.Tags, started: l.now(),
		ctl: &RunControl{ID: key, Stop: stop, Commands: commands}, stop: stop, commands: commands,
	}
	if l.active == nil {
		l.active = map[string]*activeRun{}
	}
	l.active[key] = run
	// Record the run before starting it, so even a killed process leaves new runs blocked.
	if err := l.saveLocked(); err != nil {
		delete(l.active, key)
		return nil, nil, "could not record the run in the bot state (" + err.Error() + ")."
	}

	return run, nil, ""
}

// duplicatesLocked returns the tags other runs already validate and where. Callers hold l.mu.
func (l *Listener) duplicatesLocked(tags []string) (dup []string, why string) {
	var where []string
	for _, other := range l.active {
		var mine []string
		for _, t := range tags {
			if slices.Contains(other.tags, t) {
				mine = append(mine, t)
			}
		}
		if len(mine) > 0 {
			dup = append(dup, mine...)
			where = append(where, fmt.Sprintf("%s already running in the <#%s> thread started by <@%s> at %s UTC",
				strings.Join(mine, ", "), other.channel, other.user, other.started.UTC().Format("15:04")))
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
	case "unblock":
		l.onUnblock(ctx, e)
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
	l.mu.Unlock()
	l.post(ctx, e.Channel, e.ThreadTS, "Stopped by <@"+e.User+">: no new job will start; running builds are "+
		"followed until they finish.")
}

func (l *Listener) onUnblock(ctx context.Context, e *slack.Event) {
	key := threadKey(e.Channel, e.ThreadTS)
	l.mu.Lock()
	if _, ok := l.blocks[key]; !ok {
		l.mu.Unlock()
		return
	}
	if !l.mayRun(e) {
		l.mu.Unlock()
		l.post(ctx, e.Channel, e.ThreadTS, "<@"+e.User+"> is not allowed to unblock release plans.")
		return
	}

	delete(l.blocks, key)
	left := len(l.blocks)
	saveErr := l.saveLocked()
	l.mu.Unlock()

	msg := "Unblocked this run, by <@" + e.User + ">. New plans can run again."
	if left > 0 {
		msg = fmt.Sprintf("Unblocked this run, by <@%s>. %d other runs are still blocked; new plans wait "+
			"until they are unblocked too.", e.User, left)
	}
	if l.Capacity != nil {
		if freed := l.Capacity.ReleaseAbandoned(key); freed > 0 {
			msg += fmt.Sprintf(" Freed %d Jenkins slots held by that run's builds in unknown state.", freed)
		}
	}
	if saveErr != nil {
		msg += " (Could not save the bot state: " + saveErr.Error() + "; a restart may block again.)"
	}

	l.post(ctx, e.Channel, e.ThreadTS, msg)
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

		l.mu.Lock()
		delete(l.active, threadKey(run.channel, run.ts))
		unreconciled := errors.Is(err, ErrUnreconciledBuilds)
		partial := errors.Is(err, ErrPartialRun)
		if unreconciled || partial {
			l.block(threadKey(run.channel, run.ts), err.Error())
		}
		_ = l.saveLocked() // a failed save keeps the in-progress marker, which also blocks on restart
		l.mu.Unlock()

		switch {
		case unreconciled:
			post("Release plan finished with builds in unknown state: " + err.Error() +
				"\nNew plans are blocked: their Jenkins capacity may still be in use. After checking those " +
				"builds, reply `unblock` in this thread.")
		case partial:
			post("Release plan stopped part-way: " + err.Error() +
				"\nNew plans are blocked: repeating it could dispatch workflows, create Qase runs or trigger " +
				"builds again. After checking what already started, reply `unblock` in this thread.")
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

// ErrAlreadyRunning means another bot process holds the instance lock.
var ErrAlreadyRunning = errors.New("another release bot is already running")

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
			return nil, fmt.Errorf("%w (lock %s)", ErrAlreadyRunning, path)
		}

		return nil, err
	}

	return func() { _ = f.Close() }, nil
}
