package releasebot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A console as the version-bump job prints it (timestamps, colors, the kubeconfig the framework adds).
const bumpConsole = "[2026-11-25T04:38:52.413Z] rke2 version v1.37.2+dev.4b027024 (4b0270244ca6d1a2)\n" +
	"[2026-11-25T04:39:57.678Z] IP Address: 203.0.113.10\n" +
	"[2026-11-25T04:39:57.678Z] Command to Execute: " +
	"sudo grep -h chart-url /var/lib/rancher/rke2/data/*/charts/rke2-canal.yaml\n" +
	"[2026-11-25T04:39:57.678Z] Execution Location: Node\n" +
	"[2026-11-25T04:39:57.678Z] Expected Value: rke2-canal-3.33.101-build20261120.tgz\n" +
	"[2026-11-25T04:40:02.949Z] Command:\n" +
	"[2026-11-25T04:40:02.949Z] sudo grep -h chart-url /var/lib/rancher/rke2/data/*/charts/rke2-canal.yaml\n" +
	"[2026-11-25T04:40:02.949Z] ----------------------\n" +
	"[2026-11-25T04:40:02.949Z] Matched with result:\n" +
	"[2026-11-25T04:40:02.949Z]     helm.cattle.io/chart-url: " +
	"\"https://rke2-charts.rancher.io/assets/rke2-canal/rke2-canal-3.33.101-build20261120.tgz\"\n" +
	"[2026-11-25T04:40:02.949Z]\n" +
	"[2026-11-25T04:40:03.000Z] Command to Execute: kubectl get node -o yaml : | grep rancher/hardened-calico\n" +
	"[2026-11-25T04:40:03.000Z] Expected Value: rancher/hardened-calico:v3.33.1-build20261120\n" +
	"[2026-11-25T04:40:05.000Z] Matched with result:\n" +
	"[2026-11-25T04:40:05.000Z] - docker.io/rancher/hardened-calico:v3.33.1-build20261120 on ip-172-31-4-9 (10.0.0.4)\n" +
	"[2026-11-25T04:40:05.000Z] \x1b[32mPASSED!\x1b[0m Test Bump version\n"

func canalValidated() (group *BumpGroup, validated map[string]*BranchResult) {
	target := &BumpTarget{Product: "rke2", Branch: "release-1.37", Version: "4b0270244ca6d1a2", Commit: true}
	target.Checks = []BumpCheck{
		{What: "rke2-canal chart 3.33.101-build20261120", Issue: 11921,
			Cmd:      "sudo grep -h chart-url /var/lib/rancher/rke2/data/*/charts/rke2-canal.yaml",
			Expected: "rke2-canal-3.33.101-build20261120.tgz"},
		{What: "hardened-calico image v3.33.1-build20261120", Issue: 11921,
			Cmd:      "kubectl get node -o yaml : | grep rancher/hardened-calico",
			Expected: "rancher/hardened-calico:v3.33.1-build20261120"},
	}
	version, results := ParseBumpConsole(bumpConsole)
	group = &BumpGroup{Product: "rke2", Main: BumpIssue{Repo: "rancher/rke2", Number: 11882, Title: "Update Canal"},
		Backports: []*BumpIssue{{Repo: "rancher/rke2", Number: 11921, Branch: "release-1.37"}}}

	return group, map[string]*BranchResult{"release-1.37": {
		Target: target, Version: version, NodeOS: "sles15", Arch: "amd64", Results: results,
	}}
}

func TestParseBumpConsole(t *testing.T) {
	version, results := ParseBumpConsole(bumpConsole)
	if version != "rke2 version v1.37.2+dev.4b027024 (4b0270244ca6d1a2)" || len(results) != 2 {
		t.Fatalf("version %q, results %+v", version, results)
	}
	if results[0].Expected != "rke2-canal-3.33.101-build20261120.tgz" || len(results[0].Output) != 1 ||
		!strings.Contains(results[0].Output[0], "rke2-canal-3.33.101-build20261120.tgz") || len(results[1].Output) != 1 {
		t.Fatalf("results %+v", results)
	}
}

// The MD follows the bump template, holds each check's command and output with addresses redacted,
// and its section is what -bumps-post takes, with the backports it validated.
func TestRenderBumpMD(t *testing.T) {
	group, validated := canalValidated()
	md, err := RenderBumpMD("rke2", "2026-11", []*BumpGroup{group}, validated)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"## Validated on Version", "# release-1.37\n-$ rke2 version v1.37.2+dev.4b027024 (4b0270244ca6d1a2)",
		"sles15, amd64", "(the latest commit of a release branch whose RC does not hold the bump yet)",
		"Confirm the rke2-canal chart is 3.33.101-build20261120.",
		"$ kubectl get node -o yaml | grep rancher/hardened-calico\n",
		"hardened-calico:v3.33.1-build20261120 on <REDACTED_HOST> (<REDACTED_IP>)",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("MD lacks %q:\n%s", want, md)
		}
	}

	sections, parseErr := ParseBumpMD(md)
	if parseErr != nil || len(sections) != 1 || sections[0].Issue != 11882 || sections[0].Repo != "rancher/rke2" ||
		len(sections[0].Backports) != 1 || sections[0].Backports[0] != 11921 ||
		!strings.HasPrefix(sections[0].Body, "## Validated on Version") ||
		strings.Contains(sections[0].Body, "# rancher/rke2#") {
		t.Fatalf("sections %+v, %v", sections, parseErr)
	}

	// Nothing validated for the product: no file.
	if empty, _ := RenderBumpMD("k3s", "2026-11", []*BumpGroup{group}, validated); empty != "" {
		t.Fatalf("an MD with no section: %q", empty)
	}
}

// A file that would show a jenkins link or an address is refused, also after a person edits it.
func TestBumpMDRefusesJenkinsAndAddresses(t *testing.T) {
	for _, bad := range []string{
		"see https://mower.jenkins.qa.rancher.space/job/x/1/",
		"build at /job/distros_qa/job/rke2-tests/job/x/47/",
		"node 203.0.113.10 answered",
		"node ip-172-31-4-9.us-east-2.compute.internal",
		"aws_ami: ami-0abcdef1234567890",
		"curl -sfL https://prime.ribs.rancher.io/rke2/v1.37.2/rke2.linux-amd64.tar.gz",
		"image: stgregistry.suse.com/rancher/hardened-calico:v3.33.1",
		"InternalIP: 2001:db8:abc:1234::5",
		"pod 2001:0db8:0000:0000:0000:ff00:0042:8329",
		"cluster-cidr: fd00:42::/56",
	} {
		if err := CheckBumpMD(bad); !errors.Is(err, ErrBumpMDUnsafe) {
			t.Fatalf("%q passed: %v", bad, err)
		}
		if _, err := ParseBumpMD(bad); !errors.Is(err, ErrBumpMDUnsafe) {
			t.Fatalf("ParseBumpMD(%q): %v", bad, err)
		}
		if err := CheckBumpMD(redact(bad)); err != nil && !strings.Contains(bad, "/job/") {
			t.Fatalf("redact(%q) = %q still shows it: %v", bad, redact(bad), err)
		}
	}

	for _, fine := range []string{
		"rke2 version v1.37.2+rke2r1 (abc1234)", "creationTimestamp: 2026-11-25T04:40:05Z",
		"- docker.io/rancher/hardened-calico:v3.33.1-build20261120", "sha256:4b0270244ca60523102a06b20c85c6a2f9e09f11",
		"$ kubectl get node -o yaml | grep rancher/hardened-calico", "https://rke2-charts.rancher.io/assets/x-1.0.tgz",
	} {
		if err := CheckBumpMD(fine); err != nil {
			t.Fatalf("%q is no private detail: %v", fine, err)
		}
	}
}

// The whole MD is redacted, the command and the output alike, keeping the text around each detail.
func TestRenderBumpMDRedactsEverywhere(t *testing.T) {
	group, validated := canalValidated()
	r := validated["release-1.37"]
	r.Target.Checks[0].Cmd = "sudo grep -h stgregistry.suse.com /etc/rancher/rke2/registries.yaml"
	r.Results[0].Output = append(r.Results[0].Output, "ami-0abcdef1234567890 fd00:42::1 https://prime.ribs.rancher.io/x.tgz")

	md, err := RenderBumpMD("rke2", "2026-11", []*BumpGroup{group}, validated)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"$ sudo grep -h <REDACTED_REGISTRY> /etc/rancher/rke2/registries.yaml",
		"<REDACTED_AMI> <REDACTED_IP> <REDACTED_PRIME_ARTIFACT_URL>",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("MD lacks %q:\n%s", want, md)
		}
	}
}

// fakePoster records what was posted and fails the first call when failOnce is set; open are the
// open backports GitHub knows of.
type fakePoster struct {
	comments []string // "repo#n: body"
	closed   []int
	open     []int
	failOnce bool
}

func (f *fakePoster) OpenBackports(context.Context, string, int) ([]int, error) {
	return f.open, nil
}

func (f *fakePoster) Comment(_ context.Context, repo string, issue int, body string) (string, error) {
	if f.failOnce {
		f.failOnce = false
		return "", errors.New("github is down")
	}
	f.comments = append(f.comments, fmt.Sprintf("%s#%d: %s", repo, issue, strings.SplitN(body, "\n", 2)[0]))

	return fmt.Sprintf("https://github.com/%s/issues/%d#issuecomment-%d", repo, issue, len(f.comments)), nil
}

func (f *fakePoster) CloseIssue(_ context.Context, _ string, issue int) error {
	f.closed = append(f.closed, issue)
	return nil
}

// The section goes on the main issue, a link on each backport, and closing only when asked; a run
// that failed half-way is carried on, never posting twice.
func TestPostBumpMD(t *testing.T) {
	group, validated := canalValidated()
	md, err := RenderBumpMD("rke2", "2026-11", []*BumpGroup{group}, validated)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "rke2-bumps-release-2026-11.md")
	if err = os.WriteFile(file, []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}

	gh := &fakePoster{failOnce: true}
	if err = PostBumpMD(testContext(t), gh, file, true, func(string, string, ...any) {}); err == nil {
		t.Fatal("the failed comment was not reported")
	}
	if err = PostBumpMD(testContext(t), gh, file, true, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if len(gh.comments) != 2 || gh.comments[0] != "rancher/rke2#11882: ## Validated on Version" ||
		gh.comments[1] != "rancher/rke2#11921: Validated here: "+
			"https://github.com/rancher/rke2/issues/11882#issuecomment-1" ||
		len(gh.closed) != 2 || gh.closed[0] != 11921 || gh.closed[1] != 11882 {
		t.Fatalf("comments %q, closed %v", gh.comments, gh.closed)
	}

	// Posted already: a third run does nothing again.
	err = PostBumpMD(testContext(t), gh, file, true, func(string, string, ...any) {})
	if err != nil || len(gh.comments) != 2 {
		t.Fatalf("a rerun posted again: %v %q", err, gh.comments)
	}
}

// A later run of the month validates a new backport (its 1.36 job still failing): the main issue gets
// the new results, the new backport links to them, and the main issue stays open.
func TestPostBumpMDAgainInTheMonth(t *testing.T) {
	group, validated := canalValidated()
	file := filepath.Join(t.TempDir(), "rke2-bumps-release-2026-11.md")
	gh := &fakePoster{}
	post := func() {
		t.Helper()
		md, err := RenderBumpMD("rke2", "2026-11", []*BumpGroup{group}, validated)
		if err == nil {
			err = os.WriteFile(file, []byte(md), 0o600)
		}
		if err == nil {
			err = PostBumpMD(testContext(t), gh, file, false, func(string, string, ...any) {})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	post()

	group.Backports = append(group.Backports,
		&BumpIssue{Repo: "rancher/rke2", Number: 11922, Branch: "release-1.37"},
		&BumpIssue{Repo: "rancher/rke2", Number: 11923, Branch: "release-1.36"})
	validated["release-1.37"].Target.Checks = append(validated["release-1.37"].Target.Checks,
		BumpCheck{What: "rke2-canal chart 3.33.101-build20261120", Issue: 11922,
			Cmd:      "sudo grep -h chart-url /var/lib/rancher/rke2/data/*/charts/rke2-canal.yaml",
			Expected: "rke2-canal-3.33.101-build20261120.tgz"})
	post()

	md, _ := os.ReadFile(file)
	if !strings.Contains(string(md), "backports=11921,11922 open=11923 -->") {
		t.Fatalf("section header:\n%s", md)
	}
	if len(gh.comments) != 4 || gh.comments[2] != "rancher/rke2#11882: ## Validated on Version" ||
		gh.comments[3] != "rancher/rke2#11922: Validated here: "+
			"https://github.com/rancher/rke2/issues/11882#issuecomment-3" {
		t.Fatalf("comments %q", gh.comments)
	}
}

// Closing: the main issue only once none of its backports is open.
func TestPostBumpMDKeepsMainOpen(t *testing.T) {
	file := filepath.Join(t.TempDir(), "open.md")
	section := "<!-- bump-section rancher/rke2#1 backports=2 open=3 -->\nok\n" + sectionEnd + "\n"
	if err := os.WriteFile(file, []byte(section), 0o600); err != nil {
		t.Fatal(err)
	}

	gh := &fakePoster{}
	if err := PostBumpMD(testContext(t), gh, file, true, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if len(gh.closed) != 1 || gh.closed[0] != 2 {
		t.Fatalf("closed %v: the main issue has an open backport", gh.closed)
	}

	// Every backport of the board validated, but GitHub knows one more (still in progress): it stays open.
	offBoard := filepath.Join(t.TempDir(), "off-board.md")
	if err := os.WriteFile(offBoard, []byte(strings.Replace(section, " open=3 ", " open= ", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	gh = &fakePoster{open: []int{2, 4}}
	if err := PostBumpMD(testContext(t), gh, offBoard, true, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if len(gh.closed) != 1 || gh.closed[0] != 2 {
		t.Fatalf("closed %v: #4 is open and not on the board", gh.closed)
	}
}

// A passed job whose console lacks a check's output is no evidence: the branch stays out of the MD.
func TestNoOutputIsNoEvidence(t *testing.T) {
	_, validated := canalValidated()
	r := validated["release-1.37"]
	if missing := noOutput(r.Target.Checks, r.Results); missing != "" {
		t.Fatalf("complete console: %q missing", missing)
	}

	before, _, _ := strings.Cut(bumpConsole, "[2026-11-25T04:40:05.000Z] Matched")
	_, cut := ParseBumpConsole(before)
	if missing := noOutput(r.Target.Checks, cut); !strings.HasPrefix(missing, "hardened-calico image") {
		t.Fatalf("console without the image's output: %q", missing)
	}
}

// Builds other than ours on the controller lower the limit to busy, ours told by their job; the count
// is cached, and one that cannot be read counts as busy.
func TestBusyLimit(t *testing.T) {
	ourJobs := []string{"/job/qa/job/rke2-version-bump/"}
	ours := func(n int) string { return fmt.Sprintf("https://mower/job/qa/job/rke2-version-bump/%d/", n) }
	other := "https://mower/job/qa/job/rke2-validate/9/"
	limitWith := func(builds ...string) func(string, int, int) int {
		return BusyLimit(func(context.Context) ([]string, error) { return builds, nil }, ourJobs, 4, time.Hour)
	}

	calls := 0
	builds := []string{ours(1), ours(2)}
	limit := BusyLimit(func(context.Context) ([]string, error) { calls++; return builds, nil }, ourJobs, 4, time.Hour)
	if got := limit("mower", 6, 2); got != 6 || calls != 1 {
		t.Fatalf("only our 2 builds run: limit %d, calls %d", got, calls)
	}
	builds = append(builds, other)
	if got := limit("mower", 6, 2); got != 6 || calls != 1 {
		t.Fatalf("cached for the ttl: limit %d, calls %d", got, calls)
	}

	if got := limitWith(ours(1), other)("mower", 6, 2); got != 4 {
		t.Fatalf("one of our 2 is still queued and another build runs: limit %d", got)
	}
	if got := limitWith(ours(1), ours(2), ours(3))("mower", 6, 2); got != 4 {
		t.Fatalf("someone else runs our job: limit %d", got)
	}
	unknown := BusyLimit(func(context.Context) ([]string, error) { return nil, errors.New("down") }, ourJobs, 4, time.Hour)
	if got := unknown("mower", 6, 0); got != 4 {
		t.Fatalf("an unreadable count: limit %d", got)
	}
}

// Scheduler.LimitFor caps the controller below MaxConcurrent: with a limit of 1, jobs run one at a time.
func TestSchedulerLimitFor(t *testing.T) {
	b := &scriptBuilder{}
	jobs := []JenkinsJob{
		{Name: "a", Controller: "mower", Path: "a", Phase: 1}, {Name: "b", Controller: "mower", Path: "b", Phase: 1},
		{Name: "c", Controller: "mower", Path: "c", Phase: 1},
	}
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 3}},
		Poll: time.Millisecond, LimitFor: func(string, int, int) int { return 1 },
	}

	out := s.Run(testContext(t), jobs)
	if len(out) != 3 || b.peak != 1 {
		t.Fatalf("outcomes %d, peak %d (want 1)", len(out), b.peak)
	}
}
