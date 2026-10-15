package releasebot

import (
	"errors"
	"strings"
	"testing"
)

func depJobs(versions ...string) []JenkinsJob {
	var jobs []JenkinsJob
	for _, v := range versions {
		jobs = append(jobs,
			JenkinsJob{Name: "smoke", Product: "rke2", Version: v, Controller: "mower", Path: "smoke", Priority: 1},
			JenkinsJob{
				Name: "conf", Product: "rke2", Version: v, Controller: "mower", Path: "conf", Priority: 2,
				DependsOn: []string{"smoke"},
			},
			JenkinsJob{
				Name: "rpm", Product: "rke2", Version: v, Controller: "mower", Path: "rpm", Priority: 2,
				DependsOn: []string{"conf"},
			},
		)
	}

	return jobs
}

// Dependents start only after their dependency succeeded, and transitively.
func TestDependsOnReleasesAfterSuccess(t *testing.T) {
	b := &scriptBuilder{finished: map[string][]error{"smoke": {nil, nil}}}
	out := runJobs(t, b, 3, depJobs("v1"))
	for _, o := range out {
		if o.Result != "SUCCESS" {
			t.Fatalf("%+v", o)
		}
	}
	if strings.Join(b.order, ",") != "smoke,conf,rpm" {
		t.Fatalf("trigger order %v", b.order)
	}
}

// A failed smoke blocks its own dependents (transitively) but not the other version.
// Jobs waiting for dependencies take no capacity: with limit 1 the v2 chain still runs.
func TestDependsOnFailureBlocksOnlySameVersion(t *testing.T) {
	b := &scriptBuilder{results: map[string]string{}}
	jobs := depJobs("v1", "v2")
	// only v1's smoke fails: tell them apart by path
	for i := range jobs {
		jobs[i].Path += "-" + jobs[i].Version
	}
	b.results["smoke-v1"] = "FAILURE"

	out := runJobs(t, b, 1, jobs)
	got := outcomesByPath(out)
	if got["smoke-v1 v1"].Result != "FAILURE" {
		t.Fatalf("smoke v1: %+v", got["smoke-v1 v1"])
	}
	for _, p := range []string{"conf-v1 v1", "rpm-v1 v1"} {
		if e := got[p].Err; e == nil || !strings.Contains(e.Error(), "not triggered: dependency") {
			t.Fatalf("%s must be blocked: %+v", p, got[p])
		}
	}
	for _, p := range []string{"smoke-v2 v2", "conf-v2 v2", "rpm-v2 v2"} {
		if got[p].Result != "SUCCESS" {
			t.Fatalf("%s must run: %+v", p, got[p])
		}
	}
	if b.peak > 1 {
		t.Fatalf("peak %d > 1", b.peak)
	}
}

// Unknown state (lost trigger response) is not a success: dependents stay blocked.
func TestDependsOnUnknownBlocksDependents(t *testing.T) {
	b := &scriptBuilder{lost: map[string]bool{"smoke": true}}
	out := runJobs(t, b, 2, depJobs("v1"))
	got := outcomesByPath(out)
	if !errors.Is(got["smoke v1"].Err, errTriggerUnknown) || got["conf v1"].Err == nil || got["rpm v1"].Err == nil {
		t.Fatalf("%+v", out)
	}
	if b.triggers != 1 {
		t.Fatalf("only smoke may be triggered, got %v", b.order)
	}
}

// Unknown references and cycles are rejected before anything is triggered.
func TestDependsOnValidatedBeforeTriggering(t *testing.T) {
	unknown := depJobs("v1")
	unknown[2].DependsOn = []string{"nope"}
	cycle := depJobs("v1")
	cycle[0].DependsOn = []string{"rpm"}
	// Same product/version/name twice (e.g. a plan built by hand): ambiguous for dependents.
	duplicate := append(depJobs("v1"), depJobs("v1")[0])

	for name, jobs := range map[string][]JenkinsJob{"unknown": unknown, "cycle": cycle, "duplicate": duplicate} {
		b := &scriptBuilder{}
		for _, o := range runJobs(t, b, 3, jobs) {
			if o.Err == nil || !strings.HasPrefix(o.Err.Error(), "not triggered:") {
				t.Fatalf("%s: %+v", name, o)
			}
		}
		if b.triggers != 0 {
			t.Fatalf("%s: triggered %v", name, b.order)
		}
	}
}
