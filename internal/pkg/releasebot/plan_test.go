package releasebot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// fakeResolver answers JobParams from jobs (path -> params; missing path = no such job) and
// LatestGA from ga (rc -> start version).
type fakeResolver struct {
	jobs    map[string][]string
	ga      map[string]string
	jobErr  error
	checks  int
	gaCalls int
}

func (f *fakeResolver) JobParams(_ context.Context, _, path string) (params []string, exists bool, err error) {
	f.checks++
	if f.jobErr != nil {
		return nil, false, f.jobErr
	}
	params, exists = f.jobs[path]

	return params, exists, nil
}

func (f *fakeResolver) LatestGA(_ context.Context, _, rc string) (version, note string, err error) {
	f.gaCalls++
	if v, ok := f.ga[rc]; ok {
		return v, "", nil
	}

	return "", "", fmt.Errorf("no GA for %s", rc)
}

func loadMatrixText(t *testing.T, body string) *Matrix {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := loadMatrix(path)
	if err != nil {
		t.Fatal(err)
	}

	return m
}

const phasedMatrix = `
controllers:
  mower: {url: https://mower.example, maxConcurrent: 2}
defaultParams:
  INSTALL_VERSION: "{{VERSION}}"
  HOSTNAME_PREFIX: "{{PREFIX}}"
jobs:
  - {name: tar, product: rke2, controller: mower, path: p/tar, phase: 1, split: install, code: vc}
  - {name: rpm, product: rke2, controller: mower, path: p/rpm, phase: 1, split: install, code: vr}
  - {name: arm, product: rke2, controller: mower, path: p/arm, phase: 1, code: va}
  - {name: conf, product: rke2, controller: mower, path: p/conf, phase: 2, code: cf}
  - {name: dual, product: rke2, controller: mower, path: p/dual, phase: 2, code: ds, optional: true}
  - {name: gap, product: rke2, controller: mower, path: p/gap, phase: 3, code: tb}
  - name: up
    product: rke2
    controller: mower
    path: p/up
    phase: 4
    code: su
    params: {INSTALL_VERSION: "{{UPGRADE_FROM}}", UPGRADE_VERSION: "{{VERSION}}"}
`

var fourRKE2 = []string{"v1.34.12-rc1+rke2r1", "v1.35.8-rc1+rke2r1", "v1.36.4-rc1+rke2r1", "v1.37.1-rc1+rke2r1"}

func jobsFor(p *Plan, version string) map[string]*JenkinsJob {
	out := map[string]*JenkinsJob{}
	for i := range p.Jobs {
		if p.Jobs[i].Version == version {
			out[p.Jobs[i].Name] = &p.Jobs[i]
		}
	}

	return out
}

func allParams(extra ...string) []string {
	return append([]string{"INSTALL_VERSION", "HOSTNAME_PREFIX", "QASE_RUN_ID"}, extra...)
}

// The newest half of the RCs gets the tarball smoke, the rest rpm; every other job runs for all,
// and each phase waits for the whole previous phase of the same RC.
func TestBuildPlanSplitsSmokeAndGatesPhases(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	res := &fakeResolver{
		jobs: map[string][]string{
			"p/tar": allParams(), "p/rpm": allParams(), "p/arm": allParams(), "p/conf": allParams(),
			"p/dual": allParams(), "p/gap": allParams(), "p/up": allParams("UPGRADE_VERSION"),
		},
		ga: map[string]string{
			"v1.37.1-rc1+rke2r1": "v1.37.0+rke2r1", "v1.36.4-rc1+rke2r1": "v1.36.3+rke2r1",
			"v1.35.8-rc1+rke2r1": "v1.35.7+rke2r1", "v1.34.12-rc1+rke2r1": "v1.34.11+rke2r1",
		},
	}

	p, err := buildPlan(context.Background(), Request{RKE2: fourRKE2}, m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}

	for rc, smoke := range map[string]string{
		"v1.37.1-rc1+rke2r1": "tar", "v1.36.4-rc1+rke2r1": "tar",
		"v1.35.8-rc1+rke2r1": "rpm", "v1.34.12-rc1+rke2r1": "rpm",
	} {
		jobs := jobsFor(p, rc)
		other := map[string]string{"tar": "rpm", "rpm": "tar"}[smoke]
		if jobs[smoke] == nil || jobs[other] != nil || jobs["arm"] == nil {
			t.Fatalf("%s: smoke jobs %v, want %s + arm", rc, slices.Sorted(mapsKeys(jobs)), smoke)
		}
		want := map[string][]string{
			"conf": {smoke, "arm"}, "dual": {smoke, "arm"}, "gap": {"conf", "dual"}, "up": {"gap"},
		}
		for name, deps := range want {
			got := append([]string{}, jobs[name].DependsOn...)
			sort.Strings(got)
			sort.Strings(deps)
			if !slices.Equal(got, deps) {
				t.Fatalf("%s %s depends on %v, want %v", rc, name, got, deps)
			}
		}
	}

	up := jobsFor(p, "v1.37.1-rc1+rke2r1")["up"]
	if up.Params["INSTALL_VERSION"] != "v1.37.0+rke2r1" || up.Params["UPGRADE_VERSION"] != "v1.37.1-rc1+rke2r1" ||
		up.Params["HOSTNAME_PREFIX"] != "rbr137su" || p.UpgradeFrom["v1.37.1-rc1+rke2r1"] != "v1.37.0+rke2r1" {
		t.Fatalf("upgrade job params: %v (from %v)", up.Params, p.UpgradeFrom)
	}
}

// An optional job missing on Jenkins is skipped and does not hold the next phase. A required job
// that cannot run (missing, missing a parameter, unknown upgrade start) blocks the whole plan.
func TestBuildPlanSkipsOptionalBlocksRequired(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	res := &fakeResolver{jobs: map[string][]string{
		"p/tar": allParams(), "p/rpm": allParams(), "p/arm": allParams(), "p/conf": allParams(),
		"p/gap": allParams(), "p/up": allParams(), // p/dual missing (optional); p/up lacks UPGRADE_VERSION
	}, ga: map[string]string{"v1.37.1-rc1+rke2r1": "v1.37.0+rke2r1"}}

	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1", "v1.36.4-rc1+rke2r1"}},
		m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}

	list := func(jobs []SkippedJob) []string {
		var out []string
		for _, s := range jobs {
			out = append(out, s.Path+" "+s.Version+": "+s.Reason)
		}
		sort.Strings(out)

		return out
	}
	if got, want := list(p.Skipped), []string{
		"p/dual v1.36.4-rc1+rke2r1: not on mower yet", "p/dual v1.37.1-rc1+rke2r1: not on mower yet",
	}; !slices.Equal(got, want) {
		t.Fatalf("skipped %v, want %v", got, want)
	}
	if got, want := list(p.Blocked), []string{
		"p/up v1.36.4-rc1+rke2r1: upgrade start version unknown",
		"p/up v1.37.1-rc1+rke2r1: job has no parameter UPGRADE_VERSION",
	}; !slices.Equal(got, want) {
		t.Fatalf("blocked %v, want %v", got, want)
	}
	if gap := jobsFor(p, "v1.37.1-rc1+rke2r1")["gap"]; !slices.Equal(gap.DependsOn, []string{"conf"}) {
		t.Fatalf("gap depends on %v, want only conf", gap.DependsOn)
	}
	if err = p.RunnableError(); err == nil || !strings.Contains(err.Error(), "2 required jobs cannot run") {
		t.Fatalf("RunnableError = %v", err)
	}
}

// A smoke job that loses a parameter the bot sets must stop the plan, not let ARM alone open phase 2.
func TestBuildPlanRefusesWhenSmokeCannotRun(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	res := &fakeResolver{jobs: map[string][]string{
		"p/tar": {"HOSTNAME_PREFIX", "QASE_RUN_ID"}, "p/rpm": allParams(), "p/arm": allParams(),
		"p/conf": allParams(), "p/dual": allParams(), "p/gap": allParams(), "p/up": allParams("UPGRADE_VERSION"),
	}, ga: map[string]string{"v1.37.1-rc1+rke2r1": "v1.37.0+rke2r1"}}
	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}
	want := "p/tar v1.37.1-rc1+rke2r1: job has no parameter INSTALL_VERSION"
	if err = p.RunnableError(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("RunnableError = %v", err)
	}
}

// A required job the controller does not have blocks the plan; only optional jobs may be missing.
func TestBuildPlanBlocksMissingRequiredJob(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	res := &fakeResolver{jobs: map[string][]string{
		"p/tar": allParams(), "p/rpm": allParams(), "p/arm": allParams(), "p/dual": allParams(),
		"p/gap": allParams(), "p/up": allParams("UPGRADE_VERSION"), // p/conf (required) missing
	}, ga: map[string]string{"v1.37.1-rc1+rke2r1": "v1.37.0+rke2r1"}}
	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Skipped) != 0 || len(p.Blocked) != 1 || p.Blocked[0].Path != "p/conf" || p.RunnableError() == nil {
		t.Fatalf("skipped %v, blocked %v", p.Skipped, p.Blocked)
	}
}

// Phase dependencies are checked on the expanded plan before anything external happens, even for
// a matrix that did not go through loadMatrix.
func TestBuildPlanRejectsPhaseCycle(t *testing.T) {
	m := &Matrix{PrefixBase: "rb", Controller: map[string]Limits{"mower": {URL: "u", MaxConcurrent: 1}}, Jobs: []MatrixJob{
		{Name: "a", Product: "rke2", Controller: "mower", Path: "p/a", Phase: 1, Code: "a", DependsOn: []string{"b"}},
		{Name: "b", Product: "rke2", Controller: "mower", Path: "p/b", Phase: 2, Code: "b"},
	}}
	_, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", nil)
	if err == nil || !strings.Contains(err.Error(), "plan dependencies") {
		t.Fatalf("buildPlan = %v, want a dependency error", err)
	}
}

// A job without an optional parameter (Qase reporting) still runs without it, with one warning.
func TestBuildPlanDropsOptionalParams(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	m.Defaults["QASE_RUN_ID"] = "{{QASE_RUN_ID}}"
	m.OptionalParams = []string{"QASE_RUN_ID"}
	res := &fakeResolver{jobs: map[string][]string{
		"p/tar": allParams(), "p/rpm": allParams(), "p/arm": {"INSTALL_VERSION", "HOSTNAME_PREFIX"},
		"p/conf": allParams(), "p/dual": allParams(), "p/gap": allParams(), "p/up": allParams("UPGRADE_VERSION"),
	}, ga: map[string]string{"v1.37.1-rc1+rke2r1": "v1.37.0+rke2r1", "v1.36.4-rc1+rke2r1": "v1.36.3+rke2r1"}}

	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1", "v1.36.4-rc1+rke2r1"}},
		m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}
	arm := jobsFor(p, "v1.37.1-rc1+rke2r1")["arm"]
	if _, has := arm.Params["QASE_RUN_ID"]; has || len(p.Skipped) != 0 {
		t.Fatalf("arm params %v, skipped %v", arm.Params, p.Skipped)
	}
	if n := strings.Count(strings.Join(p.Warnings, "\n"), "p/arm runs without QASE_RUN_ID"); n != 1 {
		t.Fatalf("want one warning for p/arm, got %d: %v", n, p.Warnings)
	}
	if _, has := jobsFor(p, "v1.37.1-rc1+rke2r1")["conf"].Params["QASE_RUN_ID"]; !has {
		t.Fatal("a job that defines QASE_RUN_ID lost it")
	}
}

// Without Jenkins credentials the jobs stay in the plan (dry-run can show it), with one warning.
func TestBuildPlanUncheckedJobsWarnOnce(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	res := &fakeResolver{jobErr: errors.New("JENKINS_MOWER_AUTH is not set"), ga: map[string]string{
		"v1.37.1-rc1+rke2r1": "v1.37.0+rke2r1",
	}}
	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Jobs) != 6 || len(p.Skipped) != 0 || len(p.Warnings) != 1 ||
		!strings.Contains(p.Warnings[0], "6 jobs not checked") {
		t.Fatalf("jobs=%d skipped=%v warnings=%v", len(p.Jobs), p.Skipped, p.Warnings)
	}
	// Unverified parameters would be silently ignored by Jenkins: such a plan must not run.
	if err = p.RunnableError(); err == nil || !strings.Contains(err.Error(), "6 jobs could not be checked") {
		t.Fatalf("RunnableError = %v", err)
	}
}

func mapsKeys(m map[string]*JenkinsJob) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func TestSplitShareOddCounts(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	three := []string{"v1.35.8-rc1+rke2r1", "v1.37.1-rc1+rke2r1", "v1.36.4-rc1+rke2r1"}
	got := map[string]string{}
	for _, tag := range three {
		got[tag] = splitShare(m, "rke2", three, tag)["install"]
	}
	want := map[string]string{"v1.37.1-rc1+rke2r1": "tar", "v1.36.4-rc1+rke2r1": "tar", "v1.35.8-rc1+rke2r1": "rpm"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("split = %v, want %v", got, want)
		}
	}
	if one := splitShare(m, "rke2", three[:1], three[0])["install"]; one != "tar" {
		t.Fatalf("a single RC gets %q, want the tarball smoke", one)
	}
}

func TestReleaseOrder(t *testing.T) {
	ordered := []string{
		"v1.9.9+rke2r1", "v1.37.1-rc1+rke2r1", "v1.37.1-rc2+rke2r1", "v1.37.1+rke2r1",
		"v1.37.1-rc1+rke2r2", "v1.37.1+rke2r2", "v1.37.10+rke2r1",
	}
	for i := 1; i < len(ordered); i++ {
		a, _ := parseRelease(ordered[i-1])
		b, _ := parseRelease(ordered[i])
		if !a.less(b) || b.less(a) {
			t.Fatalf("%s should sort before %s", ordered[i-1], ordered[i])
		}
	}
	if got := newestFirst([]string{"v1.9.1-rc1+k3s1", "v1.10.1-rc1+k3s1"}); got[0] != "v1.10.1-rc1+k3s1" {
		t.Fatalf("newestFirst = %v", got)
	}
}

func TestLoadMatrixRejectsLongPrefixAndDuplicateCodes(t *testing.T) {
	base := "controllers:\n  mower: {url: https://m, maxConcurrent: 1}\n" +
		"defaultParams: {HOSTNAME_PREFIX: \"{{PREFIX}}\"}\njobs:\n"
	job := func(name, product, code, extra string) string {
		return "  - {name: " + name + ", product: " + product + ", controller: mower, path: " + name +
			", code: " + code + extra + "}\n"
	}
	for name, jobs := range map[string]string{
		"prefix too long": job("a", "rke2", "ab", `, params: {HOSTNAME_PREFIX: "{{PREFIX}}xy"}`),
		"duplicate code":  job("a", "k3s", "ab", "") + job("b", "k3s", "ab", ""),
		"bad code":        job("a", "k3s", "A-1", ""),
		"missing code":    "  - {name: a, product: k3s, controller: mower, path: a}\n",
	} {
		path := filepath.Join(t.TempDir(), "m.yaml")
		_ = os.WriteFile(path, []byte(base+jobs), 0o600)
		if _, err := loadMatrix(path); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

// The shipped matrix: every product has the smoke split, and the prefixes fit qainfra's budget.
func TestShippedMatrixPlan(t *testing.T) {
	m, err := loadMatrix("../../../config/releasebot/matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	req := Request{RKE2: fourRKE2, K3s: []string{"v1.37.1-rc1+k3s1", "v1.36.4-rc1+k3s1"}}
	p, err := buildPlan(context.Background(), req, m, testNow, "rb-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range p.Jobs {
		j := &p.Jobs[i]
		if n := len(j.Params["HOSTNAME_PREFIX"]); n > hostnameBudget-len(j.Product) {
			t.Fatalf("%s %s: prefix %q too long", j.Name, j.Version, j.Params["HOSTNAME_PREFIX"])
		}
		if j.Phase == 4 {
			t.Fatalf("upgrade job %s planned without a resolver", j.Name)
		}
	}
	// Without a resolver the upgrade start is unknown: upgrades are Blocked, so the plan cannot run.
	blockedUpgrades := 0
	for _, b := range p.Blocked {
		if strings.Contains(b.Path, "upgrade") && strings.Contains(b.Reason, "upgrade start") {
			blockedUpgrades++
		}
	}
	if blockedUpgrades == 0 || p.RunnableError() == nil {
		t.Fatalf("upgrades not blocked: %+v", p.Blocked)
	}
	// The tarball half of the rke2 smoke must force tar: on SLES 16 the installer would pick rpm.
	if vc := jobsFor(p, "v1.37.1-rc1+rke2r1")["rke2-validate-cluster"]; vc == nil || vc.Params["INSTALL_METHOD"] != "tar" {
		t.Fatalf("rke2-validate-cluster: %+v", vc)
	}
	if k := jobsFor(p, "v1.36.4-rc1+k3s1"); k["k3s-validate-cluster-rpm"] == nil || k["k3s-docker-cri"] == nil {
		t.Fatalf("k3s v1.36 smoke: %v", slices.Sorted(mapsKeys(k)))
	}
}

func TestGitHubLatestGA(t *testing.T) {
	tags := map[string][]string{
		"v1.37.": {"v1.37.0-rc1+rke2r1", "v1.37.0+rke2r1", "v1.37.1-rc1+rke2r1", "v1.37.1+rke2r1", "v1.37.10+rke2r1"},
		"v1.36.": {"v1.36.3+rke2r1", "v1.36.4+rke2r1", "v1.36.4+rke2r2"},
		"v1.38.": {"v1.38.0-rc1+rke2r1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := strings.TrimPrefix(r.URL.Path, "/repos/rancher/rke2/git/matching-refs/tags/")
		if prefix == "v1.35." {
			w.Header().Set("Link", `<https://api.github.com/x?page=2>; rel="next"`)
		}
		var refs []string
		for _, t := range tags[prefix] {
			refs = append(refs, `{"ref":"refs/tags/`+t+`"}`)
		}
		_, _ = w.Write([]byte("[" + strings.Join(refs, ",") + "]"))
	}))
	defer srv.Close()
	gh := &GitHub{BaseURL: srv.URL, HTTP: srv.Client()}

	// Same minor, older than the RC (v1.37.10 is newer than v1.37.2), highest revision wins.
	for rc, want := range map[string]string{
		"v1.37.2-rc1+rke2r1": "v1.37.1+rke2r1",
		"v1.37.1-rc2+rke2r1": "v1.37.0+rke2r1",
		"v1.36.5-rc1+rke2r1": "v1.36.4+rke2r2",
	} {
		got, note, err := gh.LatestGA(context.Background(), "rke2", rc)
		if err != nil || got != want || note != "" {
			t.Fatalf("%s: got %q note %q err %v, want %q", rc, got, note, err, want)
		}
	}

	// A paged answer could hide the newest GA: refused rather than guessed.
	if got, _, err := gh.LatestGA(context.Background(), "rke2", "v1.35.2-rc1+rke2r1"); err == nil ||
		!strings.Contains(err.Error(), "paged") {
		t.Fatalf("paged tags: got %q, err %v", got, err)
	}

	// A .0 RC has no GA of its minor yet: the previous minor's latest, with a note.
	got, note, err := gh.LatestGA(context.Background(), "rke2", "v1.38.0-rc1+rke2r1")
	if err != nil || got != "v1.37.10+rke2r1" || !strings.Contains(note, "no v1.38 GA yet") {
		t.Fatalf("v1.38.0-rc1: got %q note %q err %v", got, note, err)
	}
	if _, _, err := gh.LatestGA(context.Background(), "rke2", "v1.40.0-rc1+rke2r1"); err == nil {
		t.Fatal("expected an error when neither minor has a GA")
	}
}

func TestJenkinsJobParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/job/f/job/ok/api/json":
			if !strings.Contains(r.URL.RawQuery, "parameterDefinitions") {
				t.Errorf("query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"property":[{},{"parameterDefinitions":` +
				`[{"name":"INSTALL_VERSION"},{"name":"TFVARS"}]}]}`))
		case "/job/f/job/broken/api/json":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	j := &Jenkins{BaseURL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	params, ok, err := j.JobParams(ctx, "f/ok")
	if err != nil || !ok || !slices.Equal(params, []string{"INSTALL_VERSION", "TFVARS"}) {
		t.Fatalf("ok: %v %v %v", params, ok, err)
	}
	if _, ok, err := j.JobParams(ctx, "f/missing"); err != nil || ok {
		t.Fatalf("missing: %v %v", ok, err)
	}
	if _, _, err := j.JobParams(ctx, "f/broken"); err == nil {
		t.Fatal("broken: expected an error")
	}
}

// The DTF branch comes from the matrix and can be flipped by the env file; job BRANCH follows it.
func TestDTFRefOverride(t *testing.T) {
	m, err := loadMatrix("../../../config/releasebot/matrix.yaml")
	if err != nil || m.DTFRef != "qa-infra-RC-1" || m.Defaults["BRANCH"] != "qa-infra-RC-1" {
		t.Fatalf("shipped: ref %q, BRANCH %q, err %v", m.DTFRef, m.Defaults["BRANCH"], err)
	}
	t.Setenv(DTFRefEnv, "main")
	m, err = loadMatrix("../../../config/releasebot/matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if m.DTFRef != "qa-infra-RC-1" {
		t.Fatalf("implicit environment override: ref %q", m.DTFRef)
	}
	m, err = LoadMatrixWithRef("../../../config/releasebot/matrix.yaml", "explicit-ref")
	if err != nil {
		t.Fatal(err)
	}
	if m.DTFRef != "explicit-ref" || m.Defaults["BRANCH"] != "explicit-ref" {
		t.Fatalf("override: ref %q, BRANCH %q", m.DTFRef, m.Defaults["BRANCH"])
	}
}

// An unreachable controller or GitHub costs one check per plan, not one per job: the rest are
// left unchecked at once and the plan is refused.
func TestBuildPlanStopsAskingAFailedSource(t *testing.T) {
	m, err := loadMatrix("../../../config/releasebot/matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{jobErr: errors.New("dial tcp: i/o timeout")}
	p, err := buildPlan(context.Background(), Request{RKE2: fourRKE2}, m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}
	if res.checks != 1 || res.gaCalls != 1 || len(p.Unchecked) == 0 || p.RunnableError() == nil {
		t.Fatalf("checks %d, GA calls %d, unchecked %d, runnable %v", res.checks, res.gaCalls, len(p.Unchecked),
			p.RunnableError())
	}
}

// The optional params go as one group: a job with REPORT_TO_QASE but no QASE_RUN_ID reports nothing,
// instead of reporting to the wrong run.
func TestBuildPlanDropsOptionalParamsTogether(t *testing.T) {
	m := loadMatrixText(t, `
controllers:
  mower: {url: https://mower.example, maxConcurrent: 2}
defaultParams:
  INSTALL_VERSION: "{{VERSION}}"
  REPORT_TO_QASE: "true"
  QASE_RUN_ID: "{{QASE_RUN_ID}}"
optionalParams: [REPORT_TO_QASE, QASE_RUN_ID]
jobs:
  - {name: half, product: rke2, controller: mower, path: p/half, code: vc}
  - {name: full, product: rke2, controller: mower, path: p/full, code: vr}
`)
	res := &fakeResolver{jobs: map[string][]string{
		"p/half": {"INSTALL_VERSION", "REPORT_TO_QASE"},
		"p/full": {"INSTALL_VERSION", "REPORT_TO_QASE", "QASE_RUN_ID"},
	}}
	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", res)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range p.Jobs {
		_, report := j.Params["REPORT_TO_QASE"]
		_, run := j.Params["QASE_RUN_ID"]
		if report != (j.Name == "full") || run != (j.Name == "full") {
			t.Fatalf("%s params %v", j.Name, j.Params)
		}
	}
	want := "p/half runs without QASE_RUN_ID, REPORT_TO_QASE (it does not define QASE_RUN_ID)"
	if !strings.Contains(strings.Join(p.Warnings, "\n"), want) {
		t.Fatalf("warnings %v", p.Warnings)
	}
}

// Newest first is by release, not by string: v1.37 comes before v1.9.
func TestNewerTag(t *testing.T) {
	if !newerTag("v1.37.1-rc1+rke2r1", "v1.9.9-rc1+rke2r1") || newerTag("v1.9.9-rc1+rke2r1", "v1.37.1-rc1+rke2r1") {
		t.Fatal("v1.37 must sort before v1.9")
	}
	if !newerTag("v1.37.1-rc2+rke2r1", "v1.37.1-rc1+rke2r1") {
		t.Fatal("rc2 must sort before rc1")
	}
}

// A dependsOn on a job that does not run for that RC (here the other half of the split) is dropped
// with a note, so the plan shows that the job runs without it.
func TestBuildPlanNotesDroppedDependency(t *testing.T) {
	m := loadMatrixText(t, strings.Replace(phasedMatrix,
		"path: p/conf, phase: 2, code: cf}", "path: p/conf, phase: 2, code: cf, dependsOn: [rpm]}", 1))
	p, err := buildPlan(context.Background(), Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(p.Warnings, "\n"), "conf v1.37.1-rc1+rke2r1 does not wait for rpm") {
		t.Fatalf("warnings %v", p.Warnings)
	}
}

// Within a priority, the plan lists newer releases first by version, not by string (v1.37 before v1.9).
func TestBuildPlanOrdersByRelease(t *testing.T) {
	m := loadMatrixText(t, phasedMatrix)
	req := Request{RKE2: []string{"v1.9.9-rc1+rke2r1", "v1.37.1-rc1+rke2r1"}}
	p, err := buildPlan(context.Background(), req, m, testNow, "rb-1", nil)
	if err != nil || len(p.Jobs) == 0 {
		t.Fatalf("plan: %v", err)
	}
	if p.Jobs[0].Version != "v1.37.1-rc1+rke2r1" {
		t.Fatalf("first job %s %s", p.Jobs[0].Name, p.Jobs[0].Version)
	}
}

// The Qase workflow gets its RCs in release order: v1.36.9 before v1.36.10.
func TestQaseRCsInReleaseOrder(t *testing.T) {
	rcs, _ := qaseRCs(Request{RKE2: []string{"v1.36.10-rc1+rke2r1", "v1.36.9-rc1+rke2r1", "v1.37.1-rc2+rke2r1"}})
	if got := strings.Join(rcs, ","); got != "v1.36.9-rc1,v1.36.10-rc1,v1.37.1-rc2" {
		t.Fatalf("rcs %s", got)
	}
}
