package releasebot

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/resources"
)

// Patches as GitHub returns them for real bump PRs (rke2#11743, rke2#11926, k3s#14989).
const (
	gatewayAPIPatch = `@@ -68,7 +68,7 @@ charts:
   - version: 0.1.901
     filename: /charts/rke2-security-responder.yaml
     bootstrap: false
-  - version: 1.6.300
+  - version: 1.7.1
     filename: /charts/rke2-gateway-api-crd.yaml
     bootstrap: false
     package: rke2-gateway-api-crd`
	canalChartPatch = `@@ -1,6 +1,6 @@ charts:
-  - version: v3.32.2-build2026102200
+  - version: 3.33.101-build20261120
     filename: /charts/rke2-canal.yaml
     bootstrap: true`
	canalImagesPatch = `@@ -40,8 +40,9 @@ xargs -n1 -t docker image pull --quiet << EOF > build/images-canal.txt
-    ${REGISTRY}/rancher/hardened-calico:v3.32.2-build20261022
-    ${REGISTRY}/rancher/hardened-flannel:v0.28.10-build20261022
+    ${REGISTRY}/rancher/hardened-cni-plugins:v1.10.0-build20261110
+    ${REGISTRY}/rancher/hardened-calico:v3.33.1-build20261120
+    ${REGISTRY}/rancher/hardened-flannel:v0.28.11-build20261120`
	traefikManifestPatch = `@@ -7,7 +7,7 @@ metadata:
 spec:
-  chart: https://%{KUBERNETES_API}%/static/charts/traefik-crd-40.1.7+up40.1.0.tgz
+  chart: https://%{KUBERNETES_API}%/static/charts/traefik-crd-40.1.8+up40.1.0.tgz
 ---
-  chart: https://%{KUBERNETES_API}%/static/charts/traefik-40.1.7+up40.1.0.tgz
+  chart: https://%{KUBERNETES_API}%/static/charts/traefik-40.1.8+up40.1.0.tgz`
)

func TestBumpTitlesAndBranches(t *testing.T) {
	for title, branch := range map[string]string{
		"Bump gateway-api to 1.7.0":                                          "",
		"[Release-1.36] Bump gateway-api to 1.7.0":                           "release-1.36",
		"[Release 1.37] Update Canal with Flannel and Calico latest version": "release-1.37",
		"[Release-1.37] - Bump rke2-security-responder":                      "release-1.37",
		"Update Cilium chart to 1.20.501":                                    "",
	} {
		if !IsBumpTitle(title) || BranchOf(title) != branch {
			t.Fatalf("%q: bump %v, branch %q (want %q)", title, IsBumpTitle(title), BranchOf(title), branch)
		}
	}
	for _, title := range []string{"Validate SLES 16.1", "FIPS provider missing in NGINX Ingress Controller 1.25.1"} {
		if IsBumpTitle(title) {
			t.Fatalf("%q taken for a bump", title)
		}
	}

	for body, want := range map[string][2]int{
		"Backport: https://github.com/rancher/rke2/issues/11722":    {11722, 0},
		"Backport fix for Bump rke2-security-responder\n\n* #11757": {11757, 0},
		"Backport for #11882 ":                                 {11882, 0},
		"Backport: https://github.com/rancher/rke2/pull/11776": {0, 11776},
		"no reference here":                                    {0, 0},
	} {
		if issue, pull := ParentOf(body); issue != want[0] || pull != want[1] {
			t.Fatalf("ParentOf(%q) = %d, %d, want %v", body, issue, pull, want)
		}
	}
}

// Only version changes pass; an added image-only line (canal's cni-plugins) is a version change, a
// changed setting is not, also on an image's line.
func TestPureBump(t *testing.T) {
	for name, files := range map[string][]PRFile{
		"chart version": {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
		"chart and images, one image added": {
			{Name: "charts/chart_versions.yaml", Patch: canalChartPatch},
			{Name: "scripts/build-images", Patch: canalImagesPatch},
		},
		"k3s manifest chart url": {{Name: "manifests/traefik.yaml", Patch: traefikManifestPatch}},
		"go.sum":                 {{Name: "go.sum", Patch: "-a v1 h1:x\n+a v2 h1:y\n+b v1 h1:z"}},
	} {
		if ok, why := PureBump(files); !ok {
			t.Fatalf("%s: not a pure bump: %s", name, why)
		}
	}

	for name, patch := range map[string]string{
		"setting changed": "-  bootstrap: false\n+  bootstrap: true",
		"line added":      "+  newFlag: enabled",
		"line removed":    "-  oldFlag: enabled",
		"setting on an image's line": "-  - {image: rancher/hardened-calico:v3.32.2, privileged: false}\n" +
			"+  - {image: rancher/hardened-calico:v3.33.1, privileged: true}",
		"image key added": "+    image: rancher/hardened-calico:v3.33.1",
		"image removed":   "-    ${REGISTRY}/rancher/hardened-calico:v3.32.2 \\",
	} {
		if ok, _ := PureBump([]PRFile{{Name: "charts/chart_versions.yaml", Patch: patch}}); ok {
			t.Fatalf("%s: taken for a pure bump", name)
		}
	}
}

// The checks come from the PR: RKE2 chart files, the K3s server manifest and default node images; a
// file no rule covers is left to a person.
func TestBumpChecks(t *testing.T) {
	rke2Images := []string{"hardened-calico", "hardened-flannel"}

	gatewayAPI := []PRFile{{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}}
	checks, unsupported := BumpChecks("rke2", gatewayAPI, rke2Images)
	if len(unsupported) != 0 || len(checks) != 1 || checks[0].Expected != "rke2-gateway-api-crd-1.7.1.tgz" ||
		checks[0].Cmd != "sudo grep -h chart-url /var/lib/rancher/rke2/data/*/charts/rke2-gateway-api-crd.yaml" {
		t.Fatalf("gateway-api: %+v, unsupported %v", checks, unsupported)
	}

	checks, unsupported = BumpChecks("rke2", []PRFile{
		{Name: "charts/chart_versions.yaml", Patch: canalChartPatch},
		{Name: "scripts/build-images", Patch: canalImagesPatch},
	}, rke2Images)
	got := map[string]bool{}
	for _, c := range checks {
		got[c.Expected] = true

		// What the job runs: its JoinCommands must leave the grep pattern whole.
		runs := resources.JoinCommands(c.Cmd, " --kubeconfig=k")
		if strings.HasPrefix(c.Cmd, "kubectl") && !strings.HasSuffix(runs, "| grep rancher/"+strings.Fields(c.What)[0]) {
			t.Fatalf("the job runs %q", runs)
		}
	}
	if len(checks) != 4 || !got["rke2-canal-3.33.101-build20261120.tgz"] ||
		!got["rancher/hardened-calico:v3.33.1-build20261120"] || !got["rancher/hardened-flannel:v0.28.11-build20261120"] ||
		!got["v1.10.0-build20261110"] || len(unsupported) != 0 {
		t.Fatalf("canal (cni-plugins, no default image here, is read from the canal chart): %+v, unsupported %v",
			checks, unsupported)
	}

	// Calico is checked, Cilium is not: the issue cannot pass on Calico alone.
	_, unsupported = BumpChecks("rke2", []PRFile{{Name: "scripts/build-images", Patch: "" +
		"-    ${REGISTRY}/rancher/hardened-calico:v3.32.2\n+    ${REGISTRY}/rancher/hardened-calico:v3.33.1\n" +
		"-    ${REGISTRY}/rancher/mirrored-cilium-cilium:v1.18.1\n+    ${REGISTRY}/rancher/mirrored-cilium-cilium:v1.18.2"}},
		rke2Images)
	if len(unsupported) != 1 || !strings.HasPrefix(unsupported[0], "rancher/mirrored-cilium-cilium:v1.18.2") {
		t.Fatalf("calico and cilium: unsupported %v", unsupported)
	}

	checks, _ = BumpChecks("k3s", []PRFile{{Name: "manifests/traefik.yaml", Patch: traefikManifestPatch}}, nil)
	if len(checks) != 2 || checks[0].Expected != "traefik-crd-40.1.8+up40.1.0.tgz" ||
		checks[1].Cmd != "sudo grep -h static/charts/ /var/lib/rancher/k3s/server/manifests/traefik.yaml" {
		t.Fatalf("k3s traefik: %+v", checks)
	}

	_, unsupported = BumpChecks("rke2", []PRFile{{Name: "pkg/images/images.go", Patch: "-a v1\n+a v2"}}, rke2Images)
	if len(unsupported) != 1 || unsupported[0] != "pkg/images/images.go" {
		t.Fatalf("a file no rule covers: %v", unsupported)
	}
}

// One job per branch: every check in -cmd/-expectedValue, the install mode of an RC or a commit, an
// 8-character hostname prefix; a check the job cannot be given (a comma) is refused.
func TestBumpTargetJob(t *testing.T) {
	cfg := &BumpConfig{PrefixBase: "fmor",
		Jobs: map[string]string{"rke2": "distros_qa/rke2-tests/rke2-version-bump_qainfra"}}
	target := &BumpTarget{Product: "rke2", Branch: "release-1.37", Version: "0123abcd", Commit: true, Checks: []BumpCheck{
		{What: "a", Cmd: "sudo grep -h chart-url /x/a.yaml", Expected: "a-1.tgz"},
		{What: "b", Cmd: "kubectl get node -o yaml : | grep rancher/b", Expected: "rancher/b:v2"},
	}}

	j, err := target.Job(cfg, "mower", "qa-infra-RC-1")
	if err != nil {
		t.Fatal(err)
	}
	want := `-tags=versionbump -cmd="sudo grep -h chart-url /x/a.yaml, kubectl get node -o yaml : | grep rancher/b" ` +
		`-expectedValue="a-1.tgz,rancher/b:v2" -destroy true`
	if j.Params["TEST_ARGS"] != want || j.Params["INSTALL_MODE"] != "INSTALL_RKE2_COMMIT" ||
		j.Params["HOSTNAME_PREFIX"] != "fmor137r" || j.Path != cfg.Jobs["rke2"] || j.Params["REPORT_TO_QASE"] != "" {
		t.Fatalf("job %+v", j)
	}

	target.Checks = append(target.Checks, BumpCheck{What: "c", Cmd: "echo a,b", Expected: "x"})
	if _, err = target.Job(cfg, "mower", "main"); err == nil || !strings.Contains(err.Error(), "comma") {
		t.Fatalf("a comma in a check: %v", err)
	}
}

// Backports go under the main issue their body points at; one whose main issue is not "To Test"
// still gets a group of its own.
func TestGroupBumps(t *testing.T) {
	issues := []*BumpIssue{
		{Repo: "rancher/rke2", Number: 1, Title: "Bump x"},
		{Repo: "rancher/rke2", Number: 2, Branch: "release-1.36", Parent: 1},
		{Repo: "rancher/rke2", Number: 3, Branch: "release-1.37", Parent: 1},
		{Repo: "rancher/rke2", Number: 5, Title: "[Release-1.35] Bump y", Branch: "release-1.35", Parent: 4},
	}

	issues = append(issues, &BumpIssue{
		Repo: "rancher/rke2", Number: 6, Title: "[Release-1.34] BUMP X", Branch: "release-1.34",
	})
	groups := GroupBumps(issues)
	if len(groups[0].Backports) != 3 {
		t.Fatalf("a backport naming nothing is grouped by title: %+v", groups[0].Backports)
	}
	groups[0].Backports = groups[0].Backports[:2]
	if len(groups) != 2 || groups[0].Main.Number != 1 || len(groups[0].Backports) != 2 ||
		groups[0].Backports[0].Branch != "release-1.37" || groups[1].Main.Number != 4 || groups[1].Main.Title != "Bump y" {
		t.Fatalf("groups %+v / %+v", groups[0], groups[1])
	}
}

// fakeBumpSource answers the planner from tables.
type fakeBumpSource struct {
	// "org/number"
	issues map[string][]projectIssue
	// by issue
	prs map[int][]BumpPR
	// by pull request
	files map[int][]PRFile
	tags  []string
	// "tag|commit"
	contains map[string]bool
	head     string
	// pulls are numbers that are pull requests; backports are by main issue, in any column
	pulls     map[int]bool
	backports map[int][]projectIssue
	// atRef is "ref|path" → content; later are a path's commits after the issue's, newest first
	atRef map[string]string
	later map[string][]fakeCommit
}

// fakeCommit is a commit's patch of one file and the pull request that merged it.
type fakeCommit struct{ by, patch string }

func (f *fakeBumpSource) FileAt(_ context.Context, _, path, ref string) (string, error) {
	return f.atRef[ref+"|"+path], nil
}

func (f *fakeBumpSource) BumpedBy(
	_ context.Context, _, _, path string, adds func(patch string) bool, _ []string,
) (string, error) {
	for _, c := range f.later[path] {
		if adds(c.patch) {
			return c.by, nil
		}
	}

	return "", nil
}

func (f *fakeBumpSource) BackportIssues(_ context.Context, _ string, issue int) ([]projectIssue, error) {
	return f.backports[issue], nil
}

// IssueRef finds the number among the boards' issues and backports; f.pulls are pull requests.
func (f *fakeBumpSource) IssueRef(_ context.Context, _ string, n int) (title, body string, pull bool, err error) {
	if f.pulls[n] {
		return "", "", true, nil
	}
	for _, issues := range f.issues {
		for _, i := range issues {
			if i.Number == n {
				return i.Title, i.Body, false, nil
			}
		}
	}
	for _, issues := range f.backports {
		for _, i := range issues {
			if i.Number == n {
				return i.Title, i.Body, false, nil
			}
		}
	}

	return "", "", false, nil
}

func (f *fakeBumpSource) ToTestIssues(_ context.Context, org string, number int) ([]projectIssue, error) {
	return f.issues[fmt.Sprintf("%s/%d", org, number)], nil
}

func (f *fakeBumpSource) MergedPRs(_ context.Context, _ string, number int) ([]BumpPR, error) {
	return f.prs[number], nil
}

func (*fakeBumpSource) ClosingIssue(_ context.Context, _ string, pull int) (int, error) {
	// the tests' main pull requests close the issue just before them
	return pull - 1, nil
}

func (f *fakeBumpSource) PRFiles(_ context.Context, _ string, number int) ([]PRFile, error) {
	return f.files[number], nil
}

func (f *fakeBumpSource) TagContains(_ context.Context, _, tag, commit string) (bool, error) {
	return f.contains[tag+"|"+commit], nil
}

func (f *fakeBumpSource) BranchHead(context.Context, string, string) (string, error) {
	return f.head, nil
}

func (f *fakeBumpSource) matchingTags(_ context.Context, _, prefix string) ([]string, error) {
	var out []string
	for _, t := range f.tags {
		if strings.HasPrefix(t, prefix) {
			out = append(out, t)
		}
	}

	return out, nil
}

// planSource is the RKE2 board TestPlanBumps plans.
func planSource() *fakeBumpSource {
	return &fakeBumpSource{
		issues: map[string][]projectIssue{"rancher/31": {
			{Repo: "rancher/rke2", Number: 10, Title: "Bump gateway-api to 1.7.0"},
			{Repo: "rancher/rke2", Number: 11, Title: "[Release 1.37] Bump gateway API to 1.7.0", Body: "Backport: #10"},
			{Repo: "rancher/rke2", Number: 12, Title: "[Release 1.36] Bump gateway API to 1.7.0", Body: ".../pull/11"},
			{Repo: "rancher/rke2", Number: 13, Title: "[Release 1.35] Bump gateway API to 1.7.0", Body: "Backport: #10"},
			{Repo: "rancher/rke2", Number: 20, Title: "Update CoreDNS chart to 1.50.801"},
			{Repo: "rancher/rke2", Number: 30, Title: "FIPS provider missing in NGINX"},

			// No main issue: the body names none and no To Test issue has the title.
			{Repo: "rancher/rke2", Number: 18, Title: "[Release 1.37] Bump gateway API CRDs"},

			// Cilium: the main issue is on the board, its backports in another column (Working).
			{Repo: "rancher/rke2", Number: 40, Title: "Update Cilium chart to 1.20.501"},

			// Two bumps of the same chart on release-1.34, to different versions.
			{Repo: "rancher/rke2", Number: 15, Title: "[Release 1.34] Bump gateway API to 1.7.0", Body: "Backport: #10"},
			{Repo: "rancher/rke2", Number: 17, Title: "Bump gateway API to 1.7.2"},
			{Repo: "rancher/rke2", Number: 16, Title: "[Release 1.34] Bump gateway API to 1.7.2", Body: "Backport: #17"},
		}},
		prs: map[int][]BumpPR{
			11: {{Number: 111, Base: "release-1.37", MergeCommit: "c137"}},
			12: {{Number: 112, Base: "release-1.36", MergeCommit: "c136"}},
			13: {{Number: 113, Base: "release-1.35", MergeCommit: "c135"}},
			15: {{Number: 115, Base: "release-1.34", MergeCommit: "c134a"}},
			16: {{Number: 116, Base: "release-1.34", MergeCommit: "c134b"}},
			18: {{Number: 118, Base: "release-1.37", MergeCommit: "c137b"}},
			41: {{Number: 141, Base: "release-1.35", MergeCommit: "c135c"}},
		},
		backports: map[int][]projectIssue{
			// on the board too
			10: {{Repo: "rancher/rke2", Number: 11, Title: "[Release 1.37] Bump gateway API to 1.7.0"}},
			40: {
				{Repo: "rancher/rke2", Number: 41, Title: "[Release-1.35] Update Cilium chart to 1.20.501",
					Body: "Backport #40"},
				// not merged
				{Repo: "rancher/rke2", Number: 42, Title: "[Release-1.36] Update Cilium chart to 1.20.501"},
			},
		},
		files: map[int][]PRFile{
			111: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			112: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			113: {{Name: "charts/chart_versions.yaml", Patch: "-  bootstrap: false\n+  bootstrap: true"}},
			115: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			116: {{Name: "charts/chart_versions.yaml", Patch: strings.ReplaceAll(gatewayAPIPatch, "1.7.1", "1.7.2")}},
			118: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			141: {{Name: "charts/chart_versions.yaml", Patch: "-  - version: 1.20.500\n+  - version: 1.20.501\n" +
				"     filename: /charts/rke2-cilium.yaml"}},
		},
		tags:     []string{"v1.37.2-rc1+rke2r1", "v1.37.2-rc2+rke2r1", "v1.36.6-rc2+rke2r1"},
		contains: map[string]bool{"v1.37.2-rc2+rke2r1|c137": true},
		head:     "deadbeef",
	}
}

// A plan validates clean backports, on the RC holding the merge or the branch head; every other issue
// is listed with its reason.
func TestPlanBumps(t *testing.T) {
	src := planSource()
	cfg := &BumpConfig{PrefixBase: "fmor", Jobs: map[string]string{"k3s": "k", "rke2": "r"}}

	plan, err := PlanBumps(testContext(t), src, cfg, "2026-11")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Targets) != 3 {
		t.Fatalf("targets %+v", plan.Targets)
	}
	r135, r136, r137 := plan.Targets[0], plan.Targets[1], plan.Targets[2]
	if r135.Branch != "release-1.35" || len(r135.Checks) != 1 || r135.Checks[0].Issue != 41 ||
		r135.Checks[0].Expected != "rke2-cilium-1.20.501.tgz" {
		t.Fatalf("the off-board Cilium backport is validated: %+v", r135)
	}
	if r137.Branch != "release-1.37" || r137.Version != "v1.37.2-rc2+rke2r1" || r137.Commit || len(r137.Checks) != 1 ||
		r137.Checks[0].Issue != 11 {
		t.Fatalf("1.37 on its RC: %+v", r137)
	}
	if r136.Version != "deadbeef" || !r136.Commit {
		t.Fatalf("1.36 has no RC with the merge, so the branch head: %+v", r136)
	}

	reasons := map[int]string{}
	for _, p := range plan.Person {
		reasons[p.Number] = p.Reason
	}
	if !strings.Contains(reasons[13], "not only a version bump") ||
		!strings.Contains(reasons[20], "master is never validated") || !strings.Contains(reasons[18], "no main issue") ||
		!strings.Contains(reasons[15], "2 versions") || !strings.Contains(reasons[16], "2 versions") ||
		!strings.Contains(reasons[42], "no merged pull request into release-1.36") || reasons[40] != "" {
		t.Fatalf("person: %+v", plan.Person)
	}
	for _, g := range plan.Groups {
		if g.Main.Number == 0 {
			t.Fatalf("a group for issue #0: %+v", g.Backports)
		}
		if g.Main.Number == 40 && plan.backportStates(g) !=
			"#42 release-1.36 person (not in To Test), #41 release-1.35 validate (not in To Test)" {
			t.Fatalf("Cilium's backports: %s", plan.backportStates(g))
		}
	}
	if len(plan.NotBumps) != 1 || plan.NotBumps[0].Number != 30 {
		t.Fatalf("not bumps: %+v", plan.NotBumps)
	}

	// -bumps-product k3s: the RKE2 board is not read.
	cfg.Products = []string{"k3s"}
	if only, onlyErr := PlanBumps(testContext(t), src, cfg, "2026-11"); onlyErr != nil ||
		len(only.Targets)+len(only.Person)+len(only.NotBumps) != 0 {
		t.Fatalf("k3s only: %+v, %v", only, onlyErr)
	}
}

// metricsServerPatch is rke2#11847: the chart, its default image and addon-resizer, which the chart ships
// disabled.
const metricsServerPatch = `@@ -49,8 +49,8 @@ xargs -n1 -t $PULL_CMD_CORE << EOF >> $BUILD_DIR/images-core.txt
-    ${REGISTRY}/rancher/hardened-k8s-metrics-server:v0.9.0-build20261020
-    ${REGISTRY}/rancher/hardened-addon-resizer:1.8.24-build20261021
+    ${REGISTRY}/rancher/hardened-k8s-metrics-server:v0.9.1-build20261119
+    ${REGISTRY}/rancher/hardened-addon-resizer:1.8.24-build20261110`

// An image no default install runs is checked in the values of the one chart its pull request bumps,
// in the chart file on the node; with no chart, two charts, or on K3s, it is left to a person.
func TestBumpValuesChecks(t *testing.T) {
	chart := PRFile{Name: "charts/chart_versions.yaml",
		Patch: "-  - version: 3.14.004\n+  - version: 3.14.7\n     filename: /charts/rke2-metrics-server.yaml"}
	images := PRFile{Name: "scripts/build-images", Patch: metricsServerPatch}

	checks, unsupported := BumpChecks("rke2", []PRFile{chart, images}, []string{"hardened-k8s-metrics-server"})
	want := "sudo grep -h chartContent /var/lib/rancher/rke2/data/*/charts/rke2-metrics-server.yaml" +
		" | sed s/.*chartContent.// | base64 -di | tar xzOf - rke2-metrics-server/values.yaml" +
		" | grep -A1 rancher/hardened-addon-resizer"
	if len(unsupported) != 0 || len(checks) != 3 || checks[2].Cmd != want || checks[2].Expected != "1.8.24-build20261110" ||
		checks[2].What != "hardened-addon-resizer image 1.8.24-build20261110 in rke2-metrics-server values" {
		t.Fatalf("metrics-server: %+v, unsupported %v", checks, unsupported)
	}
	if _, argsErr := (&BumpTarget{Checks: checks}).TestArgs(); argsErr != nil {
		t.Fatalf("the values check cannot be given to the job: %v", argsErr)
	}

	// The cilium chart names one operator image; its cloud variants are read from it.
	cilium := []PRFile{
		{Name: "charts/chart_versions.yaml", Patch: "-  - version: 1.20.500\n+  - version: 1.20.501\n" +
			"     filename: /charts/rke2-cilium.yaml"},
		{Name: "scripts/build-images", Patch: "+    ${REGISTRY}/rancher/mirrored-cilium-operator-aws:v1.20.5"},
	}
	if checks, _ = BumpChecks("rke2", cilium, nil); len(checks) != 2 ||
		!strings.HasSuffix(checks[1].Cmd, "| grep -A1 rancher/mirrored-cilium-operator") {
		t.Fatalf("cilium operator-aws: %+v", checks)
	}

	twoCharts := PRFile{Name: "charts/chart_versions.yaml", Patch: chart.Patch +
		"\n-  - version: 1.0.0\n+  - version: 1.0.1\n     filename: /charts/rke2-coredns.yaml"}
	for name, run := range map[string]func() []string{
		"no chart":   func() []string { _, u := BumpChecks("rke2", []PRFile{images}, nil); return u },
		"two charts": func() []string { _, u := BumpChecks("rke2", []PRFile{twoCharts, images}, nil); return u },
		"k3s":        func() []string { _, u := BumpChecks("k3s", []PRFile{images}, nil); return u },
	} {
		if u := run(); len(u) == 0 || !strings.Contains(u[0], "no single chart") {
			t.Fatalf("%s: unsupported %v", name, u)
		}
	}
}

// An issue runs on the job that installs the CNI its checks need (canal on the canal job), the others
// on the product's; one needing two CNIs goes to a person.
func TestBumpRoutesByCNI(t *testing.T) {
	cniPatch := func(chart, version string) string {
		return "-  - version: 0.0.1\n+  - version: " + version + "\n     filename: /charts/" + chart + ".yaml"
	}
	src := &fakeBumpSource{
		issues: map[string][]projectIssue{"rancher/31": {
			{Repo: "rancher/rke2", Number: 60, Title: "Update Canal"},
			{Repo: "rancher/rke2", Number: 61, Title: "[Release 1.37] Update Canal", Body: "#60"},
			{Repo: "rancher/rke2", Number: 62, Title: "Bump gateway API"},
			{Repo: "rancher/rke2", Number: 63, Title: "[Release 1.37] Bump gateway API", Body: "#62"},
			{Repo: "rancher/rke2", Number: 64, Title: "Update Canal and Cilium"},
			{Repo: "rancher/rke2", Number: 65, Title: "[Release 1.36] Update Canal and Cilium", Body: "#64"},
		}},
		prs: map[int][]BumpPR{
			61: {{Number: 161, Base: "release-1.37", MergeCommit: "c1"}},
			63: {{Number: 163, Base: "release-1.37", MergeCommit: "c2"}},
			65: {{Number: 165, Base: "release-1.36", MergeCommit: "c3"}},
		},
		files: map[int][]PRFile{
			161: {{Name: "charts/chart_versions.yaml", Patch: cniPatch("rke2-canal", "3.33.101")}},
			163: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			165: {{Name: "charts/chart_versions.yaml",
				Patch: cniPatch("rke2-canal", "3.33.101") + "\n" + cniPatch("rke2-cilium", "1.20.501")}},
		},
		head: "deadbeef",
	}
	cfg := &BumpConfig{PrefixBase: "fmor", Jobs: map[string]string{"k3s": "k", "rke2": "r/generic"},
		CNIJobs: map[string][]CNIJob{"rke2": {
			{Name: "canal", Job: "r/canal", Components: []string{"rke2-canal", "hardened-calico"}},
			{Name: "cilium", Job: "r/generic", Components: []string{"rke2-cilium"}},
		}}}

	plan, err := PlanBumps(testContext(t), src, cfg, "2026-11")
	if err != nil {
		t.Fatal(err)
	}

	paths := map[string]string{}
	for _, tg := range plan.Targets {
		j, jobErr := tg.Job(cfg, "mower", "main")
		if jobErr != nil {
			t.Fatal(jobErr)
		}
		paths[j.Name] = fmt.Sprintf("%s %d %s", j.Path, tg.Checks[0].Issue, j.Params["HOSTNAME_PREFIX"])
	}
	if len(paths) != 2 || paths["rke2-bumps-137-canal"] != "r/canal 61 fmor137rc" ||
		paths["rke2-bumps-137"] != "r/generic 63 fmor137r" {
		t.Fatalf("jobs %v", paths)
	}
	if got := cfg.JobPaths(); !slices.Equal(got, []string{"k", "r/generic", "r/canal"}) {
		t.Fatalf("BusyLimit's job paths: %v", got)
	}
	if len(plan.Person) != 1 || plan.Person[0].Number != 65 ||
		plan.Person[0].Reason != "its checks need both the canal and the cilium install" {
		t.Fatalf("person %+v", plan.Person)
	}
}

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		newer bool
	}{
		{"3.33.101-build20261120", "3.33.103-build20261120", true},
		{"v1.10.0-build20261110", "v1.10.0-build20261120", true},
		{"v0.28.1005", "0.28.1101", true},
		{"1.9.0", "1.10.0", true},
		{"40.1.8+up40.1.0", "40.1.9+up40.1.0", true},
		{"1.20.502", "1.20.501", false},
		{"1.20.501", "1.20.501", false},
		{"v1.2.3", "v1.2.3-build20261120", false},
	} {
		if got := newerVersion(tc.a, tc.b); got != tc.newer {
			t.Errorf("newerVersion(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

// A later bump in the version installed (rke2#11949 after the Canal backport), where the issue's PR set
// it (cilium's cni-plugins aside), is what the job checks; two tags there or an older one go to a person.
func TestPlanBumpsFollowsANewerBump(t *testing.T) {
	cfg := &BumpConfig{PrefixBase: "fmor", Jobs: map[string]string{"rke2": "r"}, DefaultImages: map[string][]string{
		"rke2": {"hardened-cni-plugins", "hardened-calico", "hardened-flannel", "hardened-multus-cni"}}}

	plan, err := PlanBumps(testContext(t), newerBumpSource(), cfg, "2026-11")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Targets) != 1 || len(plan.Targets[0].Checks) != 3 {
		t.Fatalf("targets %+v, person %+v", plan.Targets, plan.Person)
	}
	got := map[string]string{}
	for _, c := range plan.Targets[0].Checks {
		got[c.What] = c.Expected + " | " + c.Replaced + " | " + c.ReplacedBy
	}
	want := map[string]string{
		"rke2-canal chart 3.33.103-build20261120": "rke2-canal-3.33.103-build20261120.tgz | 3.33.101-build20261120 | " +
			"rancher/rke2#11949",
		"hardened-cni-plugins image v1.10.0-build20261120": "rancher/hardened-cni-plugins:v1.10.0-build20261120 | " +
			"v1.10.0-build20261110 | ",
		"hardened-flannel image v0.28.11-build20261120": "rancher/hardened-flannel:v0.28.11-build20261120 |  | ",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("checks %v", got)
	}

	reasons := map[int]string{}
	for _, p := range plan.Person {
		reasons[p.Number] = p.Reason
	}
	if len(reasons) != 2 || !strings.Contains(reasons[71], `"v3.33.2-build20261125, v3.33.3-build20261125"`) ||
		!strings.Contains(reasons[81], `"v4.0.0-build20261022" in scripts/build-images, where its PR set v4.1.0`) {
		t.Fatalf("person %+v", plan.Person)
	}
}

// newerBumpSource: release-1.34 backports of Canal (its head bumped the chart and cni-plugins again),
// Calico (two tags at the head) and Multus (older at the head).
func newerBumpSource() *fakeBumpSource {
	canal := "@@ -16,3 +16,3 @@ xargs -n1 -t $PULL_CMD_CORE << EOF > $BUILD_DIR/images-canal.txt\n"
	bump := func(image, from, to string) string {
		return canal + "-    ${REGISTRY}/rancher/" + image + ":" + from + "\n+    ${REGISTRY}/rancher/" + image + ":" + to
	}
	issue := func(n int, title string) []projectIssue {
		return []projectIssue{{Repo: "rancher/rke2", Number: n - 1, Title: title},
			{Repo: "rancher/rke2", Number: n, Title: "[Release 1.34] " + title, Body: fmt.Sprintf("#%d", n-1)}}
	}

	return &fakeBumpSource{
		issues: map[string][]projectIssue{"rancher/31": slices.Concat(
			issue(61, "Update Canal"), issue(71, "Bump Calico"), issue(81, "Bump Multus"))},
		prs: map[int][]BumpPR{61: {{Number: 161, Base: "release-1.34", MergeCommit: "c1"}},
			71: {{Number: 171, Base: "release-1.34"}}, 81: {{Number: 181, Base: "release-1.34"}}},
		files: map[int][]PRFile{
			161: {{Name: "charts/chart_versions.yaml", Patch: "-  - version: 3.33.100-build20261022\n" +
				"+  - version: 3.33.101-build20261120\n     filename: /charts/rke2-canal.yaml"},
				{Name: "scripts/build-images", Patch: bump("hardened-cni-plugins", "v1.9.1", "v1.10.0-build20261110") +
					"\n-    ${REGISTRY}/rancher/hardened-flannel:v0.28.10\n" +
					"+    ${REGISTRY}/rancher/hardened-flannel:v0.28.11-build20261120"}},
			171: {{Name: "scripts/build-images", Patch: bump("hardened-calico", "v3.32.0", "v3.33.1-build20261120")}},
			181: {{Name: "scripts/build-images", Patch: bump("hardened-multus-cni", "v4.0.0", "v4.1.0")}},
		},
		head: "deadbeef",
		atRef: map[string]string{
			"deadbeef|charts/chart_versions.yaml": "charts:\n  - version: 3.33.103-build20261120\n" +
				"    filename: /charts/rke2-canal.yaml\n    bootstrap: true\n",
			// each CNI lists cni-plugins at its own tag
			"deadbeef|scripts/build-images": "xargs -n1 -t $PULL_CMD_CORE << EOF > $BUILD_DIR/images-canal.txt\n" +
				"    ${REGISTRY}/rancher/hardened-cni-plugins:v1.10.0-build20261120\n" +
				"    ${REGISTRY}/rancher/hardened-flannel:v0.28.11-build20261120\n" +
				"    ${REGISTRY}/rancher/hardened-calico:v3.33.2-build20261125\n" +
				"    ${REGISTRY}/rancher/hardened-calico:v3.33.3-build20261125\n" +
				"    ${REGISTRY}/rancher/hardened-multus-cni:v4.0.0-build20261022\nEOF\n\n" +
				"xargs -n1 -t $PULL_CMD << EOF > $BUILD_DIR/images-cilium.txt\n" +
				"    ${REGISTRY}/rancher/hardened-cni-plugins:v1.10.0-build20261130\nEOF\n",
		},
		// newest first: a newer bump of another chart to the same version string did not replace canal's
		later: map[string][]fakeCommit{"charts/chart_versions.yaml": {
			{by: "rancher/rke2#11111", patch: "-  - version: 1.0.0\n+  - version: 3.33.103-build20261120\n" +
				"     filename: /charts/rke2-other.yaml"},
			{by: "rancher/rke2#11949", patch: "-  - version: 3.33.101-build20261120\n" +
				"+  - version: 3.33.103-build20261120\n     filename: /charts/rke2-canal.yaml"},
		}},
	}
}

// A backport's body may name another backport (k3s#14753 names #14749) or a pull request: the main
// issue is the one they lead to.
func TestResolveParent(t *testing.T) {
	src := &fakeBumpSource{
		issues: map[string][]projectIssue{"k3s-io/6": {
			{Repo: "k3s-io/k3s", Number: 14747, Title: "Bump x"},
			{Repo: "k3s-io/k3s", Number: 14749, Title: "[Release-1.36] Bump x", Body: "Backport of #14747"},
		}},
		pulls: map[int]bool{14761: true},
	}
	for body, want := range map[string]int{
		"Backport of #14749": 14747, "Backport of #14747": 14747, "Fixed by #14761": 14760, "nothing": 0,
	} {
		if got, err := resolveParent(testContext(t), src, "k3s-io/k3s", body); err != nil || got != want {
			t.Fatalf("resolveParent(%q) = %d, %v; want %d", body, got, err, want)
		}
	}
}

// FileAt asks for the ref as given (an RC's "+" escaped), decodes GitHub's wrapped base64, takes a
// missing file as empty and refuses a file GitHub sends no content for (over 1 MB).
func TestFileAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "v1.37.2-rc1+rke2r1" {
			t.Errorf("ref %q", r.URL.Query().Get("ref"))
		}
		switch r.URL.Path {
		case "/repos/rancher/rke2/contents/scripts/build-images":
			encoded := base64.StdEncoding.EncodeToString([]byte("line one\nline two\n"))
			_, _ = fmt.Fprintf(w, `{"content": %q, "encoding": "base64"}`, encoded[:8]+"\n"+encoded[8:])
		case "/repos/rancher/rke2/contents/big":
			_, _ = fmt.Fprint(w, `{"content": "", "encoding": "none"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	gh := &gitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}
	ref := "v1.37.2-rc1+rke2r1"

	got, err := gh.FileAt(testContext(t), "rancher/rke2", "scripts/build-images", ref)
	if err != nil || got != "line one\nline two\n" {
		t.Fatalf("FileAt = %q, %v", got, err)
	}
	if got, err := gh.FileAt(testContext(t), "rancher/rke2", "gone", ref); err != nil || got != "" {
		t.Fatalf("missing file = %q, %v", got, err)
	}
	if _, err := gh.FileAt(testContext(t), "rancher/rke2", "big", ref); err == nil ||
		!strings.Contains(err.Error(), "content not returned") {
		t.Fatalf("big file: %v", err)
	}
}

// BumpedBy names the merged pull request of the newest commit of the path, after the issue's merge,
// that sets the version where the check reads it: not another chart at the same version, not another
// CNI's section of the image list, not an older commit.
func TestBumpedBy(t *testing.T) {
	canalList := "xargs -n1 -t $PULL_CMD_CORE << EOF > $BUILD_DIR/images-canal.txt"
	canal := "@@ -16,3 +16,3 @@ " + canalList + "\n"
	cilium := "@@ -33,3 +33,3 @@ xargs -n1 -t $PULL_CMD << EOF > $BUILD_DIR/images-cilium.txt\n"
	plugins := "+    ${REGISTRY}/rancher/hardened-cni-plugins:v1.10.0-build20261120"
	chart := func(name string) string {
		return "-  - version: 1.0.0\n+  - version: 3.33.103-build20261120\n     filename: /charts/" + name + ".yaml"
	}
	// each path's commits newest first, down to the issue's merge (c1, i1), and their patches
	commits := map[string]string{
		"charts/chart_versions.yaml": `[{"sha": "c3"}, {"sha": "c2"}, {"sha": "c1"}, {"sha": "c0"}]`,
		"scripts/build-images":       `[{"sha": "i4"}, {"sha": "i3"}, {"sha": "i2"}, {"sha": "i1"}, {"sha": "i0"}]`,
	}
	files := map[byte]string{'c': "charts/chart_versions.yaml", 'i': "scripts/build-images"}
	patches := map[string]string{"c3": chart("rke2-other"), "c2": chart("rke2-canal"),
		"i4": cilium + plugins, "i3": canal + plugins, "i2": canal + plugins}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sha, pulls := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/repos/rancher/rke2/commits/"), "/pulls")
		switch {
		case r.URL.Path == "/repos/rancher/rke2/commits":
			_, _ = fmt.Fprint(w, commits[r.URL.Query().Get("path")])
		case pulls:
			// an open pull request, then the merged one: #1190<the commit's digit>
			_, _ = fmt.Fprintf(w, `[{"number": 1, "merged_at": null}, {"number": 1190%c, "merged_at": "2026-11-24"}]`,
				sha[1])
		case patches[sha] != "":
			_, _ = fmt.Fprintf(w, `{"files": [{"filename": %q, "patch": %q}]}`, files[sha[0]], patches[sha])
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	gh := &gitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}

	for _, tc := range []struct{ path, key, version, want string }{
		{files['c'], "rke2-canal chart", "3.33.103-build20261120", "rancher/rke2#11902"},
		{files['i'], "hardened-cni-plugins image|" + sectionOf(canalList), "v1.10.0-build20261120", "rancher/rke2#11903"},
	} {
		adds := func(patch string) bool { return setsVersion("rke2", tc.path, patch, tc.key, tc.version) }
		got, err := gh.BumpedBy(testContext(t), "rancher/rke2", "deadbeef", tc.path, adds, []string{"c1", "i1"})
		if err != nil || got != tc.want {
			t.Errorf("BumpedBy %s = %q, %v; want %s", tc.path, got, err, tc.want)
		}
	}
}
