package releasebot

import (
	"context"
	"fmt"
	"strings"
	"testing"
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

// Only version changes pass; an added image (canal's cni-plugins) is a version change, a changed
// setting is not.
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
	} {
		if ok, _ := PureBump([]PRFile{{Name: "charts/chart_versions.yaml", Patch: patch}}); ok {
			t.Fatalf("%s: taken for a pure bump", name)
		}
	}
}

// The checks come from the PR: the chart file every RKE2 install bundles, the K3s server manifest, and
// the node image of a default component; a file no rule covers is left to a person.
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
	}
	if len(unsupported) != 0 || len(checks) != 3 || !got["rke2-canal-3.33.101-build20261120.tgz"] ||
		!got["rancher/hardened-calico:v3.33.1-build20261120"] || !got["rancher/hardened-flannel:v0.28.11-build20261120"] {
		t.Fatalf("canal (cni-plugins is no default image): %+v, unsupported %v", checks, unsupported)
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
	cfg := &BumpConfig{PrefixBase: "rb", Jobs: map[string]string{"rke2": "distros_qa/rke2-tests/rke2-version-bump_qainfra"}}
	target := &BumpTarget{Product: "rke2", Branch: "release-1.37", Version: "0123abcd", Commit: true, Checks: []BumpCheck{
		{What: "a", Cmd: "sudo grep -h chart-url /x/a.yaml", Expected: "a-1.tgz"},
		{What: "b", Cmd: "kubectl get node -o yaml : | grep rancher/b:", Expected: "rancher/b:v2"},
	}}

	j, err := target.Job(cfg, "mower", "qa-infra-RC-1")
	if err != nil {
		t.Fatal(err)
	}
	want := `-tags=versionbump -cmd="sudo grep -h chart-url /x/a.yaml, kubectl get node -o yaml : | grep rancher/b:" ` +
		`-expectedValue="a-1.tgz,rancher/b:v2" -destroy true`
	if j.Params["TEST_ARGS"] != want || j.Params["INSTALL_MODE"] != "INSTALL_RKE2_COMMIT" ||
		j.Params["HOSTNAME_PREFIX"] != "rbr137b" || j.Path != cfg.Jobs["rke2"] || j.Params["REPORT_TO_QASE"] != "" {
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
	issues   map[string][]projectIssue // "org/number"
	prs      map[int][]BumpPR          // by issue
	files    map[int][]PRFile          // by pull request
	tags     []string
	contains map[string]bool // "tag|commit"
	head     string
}

func (f *fakeBumpSource) ToTestIssues(_ context.Context, org string, number int) ([]projectIssue, error) {
	return f.issues[fmt.Sprintf("%s/%d", org, number)], nil
}

func (f *fakeBumpSource) MergedPRs(_ context.Context, _ string, number int) ([]BumpPR, error) {
	return f.prs[number], nil
}

func (*fakeBumpSource) ClosingIssue(_ context.Context, _ string, pull int) (int, error) {
	return pull - 1, nil // the tests' main pull requests close the issue just before them
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

// A plan validates only clean backports: the RC when it holds the merge, the branch head when not;
// a non-bump, a PR that changes more than versions and a main issue without backports are listed.
func TestPlanBumps(t *testing.T) {
	src := &fakeBumpSource{
		issues: map[string][]projectIssue{"rancher/31": {
			{Repo: "rancher/rke2", Number: 10, Title: "Bump gateway-api to 1.7.0"},
			{Repo: "rancher/rke2", Number: 11, Title: "[Release 1.37] Bump gateway API to 1.7.0", Body: "Backport: #10"},
			{Repo: "rancher/rke2", Number: 12, Title: "[Release 1.36] Bump gateway API to 1.7.0", Body: ".../pull/11"},
			{Repo: "rancher/rke2", Number: 13, Title: "[Release 1.35] Bump gateway API to 1.7.0", Body: "Backport: #10"},
			{Repo: "rancher/rke2", Number: 20, Title: "Update CoreDNS chart to 1.50.801"},
			{Repo: "rancher/rke2", Number: 30, Title: "FIPS provider missing in NGINX"},
		}},
		prs: map[int][]BumpPR{
			11: {{Number: 111, Base: "release-1.37", MergeCommit: "c137"}},
			12: {{Number: 112, Base: "release-1.36", MergeCommit: "c136"}},
			13: {{Number: 113, Base: "release-1.35", MergeCommit: "c135"}},
		},
		files: map[int][]PRFile{
			111: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			112: {{Name: "charts/chart_versions.yaml", Patch: gatewayAPIPatch}},
			113: {{Name: "charts/chart_versions.yaml", Patch: "-  bootstrap: false\n+  bootstrap: true"}},
		},
		tags:     []string{"v1.37.2-rc1+rke2r1", "v1.37.2-rc2+rke2r1", "v1.36.6-rc2+rke2r1"},
		contains: map[string]bool{"v1.37.2-rc2+rke2r1|c137": true},
		head:     "deadbeef",
	}
	cfg := &BumpConfig{PrefixBase: "rb", Jobs: map[string]string{"k3s": "k", "rke2": "r"}}

	plan, err := PlanBumps(testContext(t), src, cfg, "2026-11")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Targets) != 2 {
		t.Fatalf("targets %+v", plan.Targets)
	}
	r136, r137 := plan.Targets[0], plan.Targets[1]
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
		!strings.Contains(reasons[20], "master is never validated") {
		t.Fatalf("person: %+v", plan.Person)
	}
	if len(plan.NotBumps) != 1 || plan.NotBumps[0].Number != 30 {
		t.Fatalf("not bumps: %+v", plan.NotBumps)
	}
}
