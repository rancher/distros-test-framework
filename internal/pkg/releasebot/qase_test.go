package releasebot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"

	"github.com/rancher/distros-test-framework/internal/pkg/qase"
)

func TestQaseRunTitleMatchesScript(t *testing.T) {
	// scripts/qase-patch-validation.sh: "$PRODUCT $MONTH $YEAR Patch Validation for $VERSION+<rke2r1|k3s1>".
	got := qaseRunTitle("rke2", "v1.37.1-rc2+rke2r2", testNow)
	if got != "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1" {
		t.Fatal(got)
	}
	if got := qaseRunTitle("k3s", "v1.36.5-rc1", testNow); got != "K3S September 2026 Patch Validation for v1.36.5+k3s1" {
		t.Fatal(got)
	}
}

// fakeQase serves a scripted sequence of search results per title (the last one repeats).
type fakeQase struct {
	mu    sync.Mutex
	calls map[string]int
	runs  map[string][][]QaseRun
}

func (f *fakeQase) SearchRuns(_ context.Context, title string) ([]QaseRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seq := f.runs[title]
	n := min(f.calls[title], len(seq)-1)
	f.calls[title]++

	return seq[n], nil
}

func TestQaseRunIDsFlowIntoJobs(t *testing.T) {
	m := testMatrix(t)
	m.Defaults["QASE_RUN_ID"] = "{{QASE_RUN_ID}}"
	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.QaseTitles) != 2 || p.Workflows[1].Inputs["request_id"] != "rb-1" {
		t.Fatalf("titles %v, qase inputs %v", p.QaseTitles, p.Workflows[1].Inputs)
	}

	rTitle := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	kTitle := "K3S September 2026 Patch Validation for v1.37.1+k3s1"
	mine := func(id int64, title string) QaseRun {
		return QaseRun{ID: id, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-1"}
	}
	fq := &fakeQase{calls: map[string]int{}, runs: map[string][][]QaseRun{
		rTitle: {{}, {mine(1016, rTitle)}},
		kTitle: {{mine(1017, kTitle)}},
	}}

	ids, err := waitQaseRuns(context.Background(), fq, p.QaseTitles, "rb-1", time.Millisecond, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if ids[rTitle] != 1016 || ids[kTitle] != 1017 {
		t.Fatalf("ids = %v", ids)
	}

	if err = applyQaseRunIDs(p.Jobs, ids); err != nil {
		t.Fatal(err)
	}
	for _, j := range p.Jobs {
		if j.Params["QASE_RUN_ID"] != "1016" {
			t.Fatalf("%s QASE_RUN_ID = %q", j.Path, j.Params["QASE_RUN_ID"])
		}
	}
}

// A run from another dispatch (manual or an earlier workflow finishing late) with the same title
// must not be taken, even when it is newer: only the run carrying this request id counts.
func TestMatchQaseRunIgnoresOtherDispatches(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	runs := []QaseRun{
		{ID: 900, Title: title, Description: "Version: v1.37.1-rc1"},
		{ID: 901, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-other"},
		{ID: 903, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-1x"},
		{ID: 902, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-1"},
		{ID: 950, Title: title + " (copy)", Description: "Version: v1.37.1-rc1 | Release bot request: rb-1"},
	}
	if id, ok := matchQaseRun(runs, title, "rb-1"); !ok || id != 902 {
		t.Fatalf("got %d %v, want 902", id, ok)
	}
	if _, ok := matchQaseRun(runs[:2], title, "rb-1"); ok {
		t.Fatal("must wait while only other dispatches' runs exist")
	}
	// Runs created by hand (-skip-workflows): newest with the exact title.
	if id, _ := matchQaseRun(runs, title, ""); id != 903 {
		t.Fatalf("got %d, want 903", id)
	}
}

func TestWaitQaseRunsTimesOut(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	fq := &fakeQase{calls: map[string]int{}, runs: map[string][][]QaseRun{
		title: {{{ID: 901, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-other"}}},
	}}
	if _, err := waitQaseRuns(context.Background(), fq, []string{title}, "rb-1",
		time.Millisecond, 20*time.Millisecond); err == nil {
		t.Fatal("expected timeout: only another dispatch's run exists")
	}
}

func TestApplyQaseRunIDsMissing(t *testing.T) {
	jobs := []JenkinsJob{{Path: "a", QaseTitle: "x", Params: map[string]string{"QASE_RUN_ID": "{{QASE_RUN_ID}}"}}}
	if err := applyQaseRunIDs(jobs, map[string]int64{}); err == nil {
		t.Fatal("expected error for unresolved run")
	}
}

// Cancellation and deadline stay identifiable through the Qase wait error.
func TestWaitQaseRunsKeepsContextError(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	fq := &fakeQase{calls: map[string]int{}, runs: map[string][][]QaseRun{title: {{}}}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitQaseRuns(ctx, fq, []string{title}, "rb-1", time.Millisecond, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}

	_, err = waitQaseRuns(context.Background(), fq, []string{title}, "rb-1", time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}

// The adapter over the shared qase-go client maps runs (including a null description) and matches them.
func TestQaseAdapterAgainstFakeAPI(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":true,"result":{"entities":[
			{"id":901,"title":%[1]q,"description":null},
			{"id":902,"title":%[1]q,"description":"Version: v1.37.1-rc1 | Release bot request: rb-1"}]}}`, title)
	}))
	defer srv.Close()

	cfg := qaseclient.NewConfiguration()
	cfg.Servers = qaseclient.ServerConfigurations{{URL: srv.URL + "/v1"}}
	ctx := context.WithValue(context.Background(), qaseclient.ContextAPIKeys,
		map[string]qaseclient.APIKey{"TokenAuth": {Key: "tok"}})
	q := newQaseFrom(&qase.Client{QaseAPI: qaseclient.NewAPIClient(cfg), Ctx: ctx})

	ids, err := waitQaseRuns(context.Background(), q, []string{title}, "rb-1", time.Millisecond, time.Second)
	if err != nil || ids[title] != 902 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

func TestNewQaseRequiresToken(t *testing.T) {
	t.Setenv("QASE_AUTOMATION_TOKEN", "")
	if _, err := newQase(); err == nil {
		t.Fatal("expected error without QASE_AUTOMATION_TOKEN")
	}
}
