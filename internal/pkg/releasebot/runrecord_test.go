package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resumeBuilder finishes builds with results (default SUCCESS); a held path keeps building until
// its channel is closed. It counts triggers per path.
type resumeBuilder struct {
	mu       sync.Mutex
	results  map[string]string
	hold     map[string]chan struct{}
	triggers map[string]int
}

func (b *resumeBuilder) Trigger(_ context.Context, j *JenkinsJob) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.triggers[j.Path]++

	return "q/" + j.Path, nil
}

func (*resumeBuilder) BuildFromQueue(_ context.Context, q string) (string, error) {
	return "b/" + strings.TrimPrefix(q, "q/"), nil
}

func (b *resumeBuilder) Finished(_ context.Context, u string) (done bool, result string, err error) {
	path := strings.TrimPrefix(u, "b/")
	b.mu.Lock()
	ch, res := b.hold[path], b.results[path]
	b.mu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
			return false, "", nil
		}
	}
	if res == "" {
		res = resultSuccess
	}

	return true, res, nil
}

func (b *resumeBuilder) count(path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.triggers[path]
}

// journalTap keeps the last journal a run wrote.
type journalTap struct {
	mu   sync.Mutex
	last []JobRecord
}

func (j *journalTap) save(recs []JobRecord, _ bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.last = recs

	return nil
}

func (j *journalTap) states() map[string]string {
	j.mu.Lock()
	defer j.mu.Unlock()
	m := map[string]string{}
	for i := range j.last {
		m[j.last[i].Job.Name] = j.last[i].State
	}

	return m
}

func resumeJobs() []JenkinsJob {
	return []JenkinsJob{
		{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke", Phase: 1},
		{Name: "rpm", Product: "rke2", Version: "v1", Controller: "mower", Path: "rpm", Phase: 1},
		{
			Name: "conf", Product: "rke2", Version: "v1", Controller: "mower", Path: "conf", Phase: 2,
			DependsOn: []string{"smoke"},
		},
	}
}

// journalOfKilledRun runs the jobs until smoke passed, rpm waits for help and conf is running, kills
// the run there and returns its last journal as read back from the state file.
func journalOfKilledRun(t *testing.T, b *resumeBuilder) []JobRecord {
	t.Helper()
	tap := &journalTap{}
	first := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 3}},
		Poll: time.Millisecond, Triage: askTriager, Commands: make(chan Command), Help: func(string) {},
		Journal: tap.save,
	}
	ctx, kill := context.WithCancel(testContext(t))
	killed := make(chan []Outcome, 1)
	go func() { killed <- first.Run(ctx, resumeJobs()) }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		st := tap.states()
		if st["smoke"] == jobDone && st["rpm"] == jobHeld && st["conf"] == jobRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("journal never reached the expected state: %v", st)
		}
	}
	kill()
	receive(t, killed)

	raw, _ := json.Marshal(tap.last) // as written to the state file
	var saved []JobRecord
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}

	return saved
}

// A run killed mid-way resumes from its last journal (as saved to disk): finished jobs are not run
// again, the running build is followed from its URL with its slot taken back, and the job waiting
// for help is posted again and answered.
func TestResumeCarriesRunOn(t *testing.T) {
	b := &resumeBuilder{
		results: map[string]string{"rpm": "FAILURE"}, triggers: map[string]int{},
		hold: map[string]chan struct{}{"conf": make(chan struct{})},
	}
	saved := journalOfKilledRun(t, b)
	var help atomic.Value
	commands := make(chan Command, 1)
	capacity := &Capacity{}
	second := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 3}},
		Poll: time.Millisecond, Triage: askTriager, Commands: commands, Capacity: capacity,
		Help: func(m string) { help.Store(m) }, Resume: saved,
	}
	done := make(chan []Outcome, 1)
	go func() { done <- second.Run(testContext(t), resumeJobs()) }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if m, _ := help.Load().(string); strings.Contains(m, "Needs help: rpm v1 ended with FAILURE") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the job waiting for help was not posted again")
		}
	}
	if capacity.InUse("mower") != 1 {
		t.Fatalf("the running build did not take its slot back: in use %d", capacity.InUse("mower"))
	}
	close(b.hold["conf"])
	reply := make(chan string, 1)
	commands <- Command{Action: CommandSkip, Job: "rpm", By: "U1", Reply: reply}
	got := outcomesByPath(receive(t, done))
	if got["smoke v1"].Result != resultSuccess || got["conf v1"].Result != resultSuccess ||
		got["rpm v1"].Result != ResultSkipped {
		t.Fatalf("outcomes %+v", got)
	}
	if b.count("smoke") != 1 || b.count("conf") != 1 || b.count("rpm") != 1 || capacity.InUse("mower") != 0 {
		t.Fatalf("triggers smoke %d conf %d rpm %d (each once), in use %d", b.count("smoke"), b.count("conf"),
			b.count("rpm"), capacity.InUse("mower"))
	}
}

// A job whose triage was cut by the restart is triaged again.
func TestResumeTriagesAgain(t *testing.T) {
	var triaged atomic.Int32
	commands := make(chan Command, 1)
	var help atomic.Value
	s := &Scheduler{
		Builders: map[string]Builder{"mower": &resumeBuilder{triggers: map[string]int{}}},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 1}}, Poll: time.Millisecond, Commands: commands,
		Triage: func(context.Context, *Outcome) Decision {
			triaged.Add(1)
			return Decision{Summary: "PRODUCT"}
		},
		Help: func(m string) { help.Store(m) },
		Resume: []JobRecord{{
			Job:   JenkinsJob{Name: "rpm", Version: "v1", Controller: "mower", Path: "rpm"},
			State: jobTriaging, BuildURL: "b/rpm", Result: "FAILURE",
		}},
	}
	done := make(chan []Outcome, 1)
	go func() { done <- s.Run(testContext(t), nil) }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if m, _ := help.Load().(string); strings.Contains(m, "Triage: PRODUCT") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the cut triage was not redone")
		}
	}
	commands <- Command{Action: CommandSkip, Job: "rpm", By: "U1", Reply: make(chan string, 1)}
	receive(t, done)
	if triaged.Load() != 1 {
		t.Fatalf("triaged %d times", triaged.Load())
	}
}

// A trigger cut by the restart may have created a build: the job is in unknown state, holds its slot
// and is not triggered again; with no URL to check, the slot is freed once the watch limit passes.
func TestResumeTriggeringIsUnknown(t *testing.T) {
	b := &resumeBuilder{triggers: map[string]int{}}
	capacity := &Capacity{}
	var held atomic.Int32
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 1}},
		Poll: time.Millisecond, Capacity: capacity, WatchPoll: time.Hour, WatchLimit: 50 * time.Millisecond,
		Notify: func(string, string, ...any) {
			if capacity.Abandoned("mower") == 1 {
				held.Store(1)
			}
		},
		Resume: []JobRecord{{
			Job: JenkinsJob{Name: "a", Version: "v1", Controller: "mower", Path: "a"}, State: jobTriggering,
		}},
	}
	out := s.Run(testContext(t), nil)
	if len(out) != 1 || !errors.Is(out[0].Err, errStateUnknown) || b.count("a") != 0 || held.Load() != 1 ||
		capacity.InUse("mower") != 0 {
		t.Fatalf("out %+v, triggers %d, held %d, in use %d", out, b.count("a"), held.Load(), capacity.InUse("mower"))
	}
}

// A build whose state was lost keeps being watched: when it finishes, its slot is freed, the thread
// is told, and its outcome is the real result.
func TestWatchedBuildFinishes(t *testing.T) {
	e := errors.New("GET build: 502")
	b := &scriptBuilder{finished: map[string][]error{"a": {e, e, e}}}
	var mu sync.Mutex
	var logs []string
	capacity := &Capacity{}
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 1}},
		Poll: time.Millisecond, MaxPollErrors: 3, Capacity: capacity,
		WatchPoll: time.Millisecond, WatchLimit: 5 * time.Second,
		Notify: func(_, f string, a ...any) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, fmt.Sprintf(f, a...))
		},
	}
	out := s.Run(testContext(t), []JenkinsJob{{Name: "a", Controller: "mower", Path: "a", Version: "v1"}})
	mu.Lock()
	defer mu.Unlock()
	if out[0].Result != resultSuccess || out[0].Err != nil || unreconciled(out) != nil || capacity.InUse("mower") != 0 ||
		!strings.Contains(strings.Join(logs, "\n"), "finished with SUCCESS; its jenkins slot is free again") {
		t.Fatalf("out %+v, in use %d, logs:\n%s", out, capacity.InUse("mower"), strings.Join(logs, "\n"))
	}
}

// slowTrigger holds every trigger until released, as a slow jenkins answer would.
type slowTrigger struct {
	resumeBuilder
	release chan struct{}
}

func (b *slowTrigger) Trigger(ctx context.Context, j *JenkinsJob) (string, error) {
	<-b.release

	return b.resumeBuilder.Trigger(ctx, j)
}

// A trigger in flight is saved before it is sent: a restart then knows a build may exist.
func TestJournalSavesTriggerInFlight(t *testing.T) {
	b := &slowTrigger{resumeBuilder: resumeBuilder{triggers: map[string]int{}}, release: make(chan struct{})}
	tap := &journalTap{}
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 1}},
		Poll: time.Millisecond, Journal: tap.save,
	}
	done := make(chan []Outcome, 1)
	go func() {
		done <- s.Run(testContext(t), []JenkinsJob{{Name: "a", Version: "v1", Controller: "mower", Path: "a"}})
	}()
	for deadline := time.Now().Add(5 * time.Second); tap.states()["a"] != jobTriggering; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("trigger in flight not saved: %v", tap.states())
		}
	}
	close(b.release)
	receive(t, done)
}

func phasedPair() []JenkinsJob {
	return []JenkinsJob{
		{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke", Phase: 1},
		{
			Name: "install", Product: "rke2", Version: "v1", Controller: "mower", Path: "install", Phase: 2,
			DependsOn: []string{"smoke"},
		},
	}
}

func resumed(b Builder, recs []JobRecord) *Scheduler {
	return &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 2}},
		Poll: time.Millisecond, WatchPoll: time.Millisecond, WatchLimit: time.Second, Resume: recs,
	}
}

// A job a person skipped still counts as passed after a restart: the next phase runs.
func TestSkippedStaysPassedAfterRestart(t *testing.T) {
	jobs := phasedPair()
	b := &resumeBuilder{triggers: map[string]int{}}
	resumed(b, []JobRecord{{Job: jobs[0], State: jobDone, Result: ResultSkipped}, {Job: jobs[1], State: jobPending}}).
		Run(testContext(t), jobs)
	if b.count("install") != 1 {
		t.Fatalf("install triggered %d times after a skipped smoke", b.count("install"))
	}
}

// While a build is in unknown state its dependents wait; once it is seen finishing, its result
// goes the normal way: a success lets the next phase run, a failure is triaged.
func TestRecoveredUnknownBuildGoesTheNormalWay(t *testing.T) {
	jobs := phasedPair()
	b := &resumeBuilder{triggers: map[string]int{}}
	unknown := JobRecord{Job: jobs[0], State: jobUnknown, BuildURL: "b/smoke", Since: time.Now()}
	resumed(b, []JobRecord{unknown, {Job: jobs[1], State: jobPending}}).Run(testContext(t), jobs)
	if b.count("install") != 1 {
		t.Fatalf("install triggered %d times after smoke was seen passing", b.count("install"))
	}

	failing := &resumeBuilder{triggers: map[string]int{}, results: map[string]string{"smoke": "FAILURE"}}
	s := resumed(failing, []JobRecord{unknown})
	var triaged atomic.Int32
	s.Triage = func(context.Context, *Outcome) Decision {
		triaged.Add(1)
		return Decision{Summary: "ask"}
	}
	s.Run(testContext(t), jobs[:1])
	if triaged.Load() != 1 {
		t.Fatalf("a failure seen after an unknown state was triaged %d times", triaged.Load())
	}
}

// A build jenkins still reports as running is not given up at the watch limit: it is queried first.
func TestRunningBuildOutlivesWatchLimit(t *testing.T) {
	job := phasedPair()[0]
	b := &resumeBuilder{triggers: map[string]int{}, hold: map[string]chan struct{}{"smoke": make(chan struct{})}}
	s := resumed(b, nil)
	s.Capacity = &Capacity{}
	st := s.newRunState(testContext(t), nil)
	defer st.end()
	now := time.Now()
	s.Now = func() time.Time { return now }
	s.WatchLimit = 4 * time.Hour
	s.seed(st, []JobRecord{{Job: job, State: jobUnknown, BuildURL: "b/smoke", Since: now.Add(-4 * time.Hour)}}, now)
	if s.watch(testContext(t), st, "mower", st.active["mower"][0]) || s.Capacity.InUse("mower") != 1 {
		t.Fatalf("a build still running was given up: slots %d", s.Capacity.InUse("mower"))
	}
}

// A stopped run stays stopped after a restart: a failure of a build it was following is not
// triaged and nothing new is triggered.
func TestStopSurvivesRestart(t *testing.T) {
	job := phasedPair()[0]
	b := &resumeBuilder{triggers: map[string]int{}, results: map[string]string{"smoke": "FAILURE"}}
	var stopped bool
	first := resumed(b, nil)
	first.Journal = func(_ []JobRecord, s bool) error {
		stopped = s
		return nil
	}
	st := first.newRunState(testContext(t), nil)
	defer st.end()
	st.active["mower"] = []*running{{job: job, buildURL: "b/smoke"}}
	first.drain(st)
	_ = first.journal(st)

	second := resumed(b, st.records())
	second.ResumeStopped = stopped
	second.Triage = func(context.Context, *Outcome) Decision { return Decision{Rerun: true, Summary: "transient"} }
	out := second.Run(testContext(t), []JenkinsJob{job})
	if !stopped || b.count("smoke") != 0 || len(out) != 1 || out[0].Result != "FAILURE" {
		t.Fatalf("stopped saved %v, triggers %d, out %+v", stopped, b.count("smoke"), out)
	}
}

// While the run state cannot be saved nothing is triggered (a restart would not know about the
// build); once saving works again the job is triggered, once.
func TestUnsavedStateBlocksTriggers(t *testing.T) {
	b := &resumeBuilder{triggers: map[string]int{}}
	var failing atomic.Bool
	failing.Store(true)
	var blocked atomic.Int32
	s := resumed(b, nil)
	s.Journal = func([]JobRecord, bool) error {
		if failing.Load() {
			blocked.Add(1)
			return errors.New("disk full")
		}

		return nil
	}
	done := make(chan []Outcome, 1)
	go func() { done <- s.Run(testContext(t), phasedPair()[:1]) }()
	for deadline := time.Now().Add(5 * time.Second); blocked.Load() < 3; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the run never tried to save")
		}
	}
	if b.count("smoke") != 0 {
		t.Fatalf("triggered %d times while the state could not be saved", b.count("smoke"))
	}
	failing.Store(false)
	receive(t, done)
	if b.count("smoke") != 1 {
		t.Fatalf("triggered %d times once saving worked", b.count("smoke"))
	}
}

// A job saved as queued whose queue item jenkins has already forgotten (the bot was down longer
// than a few minutes) is found among the job's builds by its queue id, not lost.
func TestResumeFindsBuildOfForgottenQueueItem(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/queue/item/42/api/json", http.NotFound)
	mux.HandleFunc("/job/smoke/api/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"builds":[{"url":%q,"queueId":43},{"url":%q,"queueId":42}]}`,
			srv.URL+"/job/smoke/8/", srv.URL+"/job/smoke/7/")
	})
	mux.HandleFunc("/job/smoke/7/api/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"building":false,"result":"SUCCESS"}`)
	})
	j := &jenkins{BaseURL: srv.URL, User: "qa", Token: "dummy", HTTP: srv.Client()}
	s := resumed(j, []JobRecord{{Job: phasedPair()[0], State: jobQueued, QueueURL: srv.URL + "/queue/item/42/"}})
	var failed atomic.Int32
	s.Notify = func(_, format string, _ ...any) {
		if strings.Contains(format, "status query failed") {
			failed.Add(1)
		}
	}
	out := s.Run(testContext(t), nil)
	if len(out) != 1 || out[0].Result != resultSuccess || out[0].BuildURL != srv.URL+"/job/smoke/7/" ||
		failed.Load() != 0 {
		t.Fatalf("out %+v, failed status queries %d (found at once, not after giving up)", out, failed.Load())
	}
}

// A saved unknown build's reason is shown once when the watch gives up, and still counts as unknown.
func TestSavedUnknownReasonIsNotRepeated(t *testing.T) {
	s := resumed(&resumeBuilder{triggers: map[string]int{}}, []JobRecord{{
		Job: phasedPair()[0], State: jobUnknown, Since: time.Now(),
		Err: "build state unknown after 10 failed status queries (last: 403); check b/smoke by hand",
	}})
	s.WatchLimit = 10 * time.Millisecond
	out := s.Run(testContext(t), nil)
	if len(out) != 1 || !errors.Is(out[0].Err, errStateUnknown) ||
		strings.Count(out[0].Err.Error(), "build state unknown") != 1 {
		t.Fatalf("out %+v", out)
	}
}

// A controller marked unreachable while unknown builds filled it is usable again once watching frees
// a slot: the next phase runs there.
func TestControllerReachableAgain(t *testing.T) {
	jobs := phasedPair()
	b := &resumeBuilder{triggers: map[string]int{}}
	s := resumed(b, []JobRecord{
		{Job: jobs[0], State: jobUnknown, BuildURL: "b/smoke", Since: time.Now()}, {Job: jobs[1], State: jobPending},
	})
	s.Limits = map[string]Limits{"mower": {MaxConcurrent: 1}}
	out := outcomesByPath(s.Run(testContext(t), jobs))
	if b.count("install") != 1 || out["install v1"].Result != resultSuccess {
		t.Fatalf("install triggered %d times: %+v", b.count("install"), out)
	}
}
