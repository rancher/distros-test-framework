package releasebot

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The bump MD: one section per main issue, in the team's bump template, posted by -bumps-post; it is
// public, so nothing in it may point at jenkins or show an infrastructure detail.

// BumpResult is one check as the job ran it.
type BumpResult struct {
	Cmd, Expected string
	Output        []string
}

// BranchResult is a passed version-bump job: what it installed and what its checks printed.
type BranchResult struct {
	Target *BumpTarget
	// "rke2 version v1.37.2+dev.4b027024 (4b02...)", from the console when it says
	Version string
	NodeOS  string
	Arch    string
	Results []BumpResult

	// Nodes and Flags are the cluster the job built, from its TFVARS ("1 node: etcd, cp, worker").
	Nodes, Flags string
}

// issueResult is the passed job that validated the backport (a branch can have one per CNI install).
func issueResult(validated map[string]*BranchResult, bp *BumpIssue) *BranchResult {
	for _, r := range validated {
		if r.Target.Branch == bp.Branch && slices.ContainsFunc(r.Target.Checks,
			func(c BumpCheck) bool { return c.Issue == bp.Number }) {
			return r
		}
	}

	return nil
}

var (
	consoleStamp   = regexp.MustCompile(`^\[\d{4}-\d\d-\d\dT[^\]]*\]\s?`)
	versionLine    = regexp.MustCompile(`\b(?:rke2|k3s) version v\S+ \([0-9a-f]{7,}\)`)
	jenkinsPointer = regexp.MustCompile(`(?i)https?://[^\s)"]*jenkins[^\s)"]*|/job/[^\s)"]+/\d+/?`)
)

// private is what a public issue must not show, with its placeholder; an IPv6 is eight groups or has a
// "::", so times and image tags ("04:40:05", "x:v1") are left alone.
var private = []struct {
	re          *regexp.Regexp
	placeholder string
}{
	{regexp.MustCompile(`(?i)(?:https?://)?prime\.ribs\.rancher\.io[^\s)"'<>]*`), "<REDACTED_PRIME_ARTIFACT_URL>"},
	{regexp.MustCompile(`(?i)\bstgregistry\.suse\.com\b`), "<REDACTED_REGISTRY>"},
	{regexp.MustCompile(`(?i)\bami-[0-9a-f]{8,17}\b`), "<REDACTED_AMI>"},
	{regexp.MustCompile(`\bip(?:-\d{1,3}){4}(?:\.[a-z0-9-]+)*`), "<REDACTED_HOST>"},
	{regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`), "<REDACTED_IP>"},
	{regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){7}[0-9a-f]{1,4}\b|(?:\b[0-9a-f]{1,4}:)+:(?:[0-9a-f]{1,4}` +
		`(?::[0-9a-f]{1,4})*\b)?|::[0-9a-f]{1,4}(?::[0-9a-f]{1,4})*\b`), "<REDACTED_IP>"},
}

// ParseBumpConsole reads the version-bump job's console: the installed version and, for every
// "Command to Execute:" block, its "Expected Value:" and the lines after "Matched with result:".
func ParseBumpConsole(console string) (version string, results []BumpResult) {
	var cur *BumpResult
	matching := false
	for _, raw := range strings.Split(console, "\n") {
		line := strings.TrimRight(consoleStamp.ReplaceAllString(ansiCode.ReplaceAllString(raw, ""), ""), "\r")
		if version == "" {
			version = versionLine.FindString(line)
		}

		switch {
		case strings.HasPrefix(line, "Command to Execute:"):
			results = append(results, BumpResult{Cmd: strings.TrimSpace(strings.TrimPrefix(line, "Command to Execute:"))})
			cur, matching = &results[len(results)-1], false
		case cur == nil:
		case strings.HasPrefix(line, "Expected Value:"):
			cur.Expected = strings.TrimSpace(strings.TrimPrefix(line, "Expected Value:"))
		case strings.HasPrefix(line, "Matched with result:"):
			matching = true
		case matching && (strings.TrimSpace(line) == "" || strings.HasPrefix(line, "PASSED!") ||
			strings.HasPrefix(line, "FAILED!") || strings.HasPrefix(line, "IP Address:") || strings.HasPrefix(line, "----")):
			matching = false
		case matching:
			cur.Output = append(cur.Output, line)
		}
	}

	return version, results
}

// displayCmd is the command as a person runs it: the framework's " : " kubeconfig marker goes.
func displayCmd(cmd string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(cmd, " : ", " ")), " ")
}

// redact replaces every private detail with its placeholder, keeping the text around it.
func redact(s string) string {
	for _, p := range private {
		s = p.re.ReplaceAllString(s, p.placeholder)
	}

	return s
}

// ErrBumpMDUnsafe: the MD would show a jenkins link or a private detail, so it is not written (or posted).
var ErrBumpMDUnsafe = errors.New("bump MD would show a jenkins link or a private infrastructure detail")

// CheckBumpMD refuses text that points at jenkins or shows a private detail (the issues are public).
func CheckBumpMD(md string) error {
	if m := jenkinsPointer.FindString(md); m != "" {
		return fmt.Errorf("%w: %q", ErrBumpMDUnsafe, m)
	}
	for _, p := range private {
		if m := p.re.FindString(md); m != "" {
			return fmt.Errorf("%w: %q", ErrBumpMDUnsafe, m)
		}
	}

	return nil
}

const (
	sectionStart = "<!-- bump-section "
	sectionEnd   = "<!-- /bump-section -->"
)

// RenderBumpMD writes the product's MD: a section per main issue with a validated backport; validated
// is target name → result, and groups without one are left out (the run lists them).
func RenderBumpMD(product, month string, groups []*BumpGroup, validated map[string]*BranchResult) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- %s version bumps, %s release: one section per issue; releasebot -bumps-post posts each "+
		"section on its issue and links the backports to it. -->\n", product, month)

	sections := 0
	for _, g := range groups {
		if g.Product != product {
			continue
		}

		var done []*BumpIssue
		for _, bp := range g.Backports {
			if issueResult(validated, bp) != nil {
				done = append(done, bp)
			}
		}
		if len(done) == 0 {
			continue
		}

		sections++
		renderSection(&b, g, done, validated)
	}
	if sections == 0 {
		return "", nil
	}

	// commands, titles and outputs alike
	md := redact(b.String())
	if unsafeErr := CheckBumpMD(md); unsafeErr != nil {
		return "", unsafeErr
	}

	return md, nil
}

// writeSectionHeader writes the section's title and marker: the backports it validates, those it does
// not (open: they keep the main issue open) and those validated on a commit build only.
func writeSectionHeader(b *strings.Builder, g *BumpGroup, done []*BumpIssue, validated map[string]*BranchResult) {
	var numbers, open, commit []string
	for _, bp := range g.Backports {
		if !slices.Contains(done, bp) {
			open = append(open, strconv.Itoa(bp.Number))
			continue
		}
		numbers = append(numbers, strconv.Itoa(bp.Number))
		if issueResult(validated, bp).Target.Commit {
			commit = append(commit, strconv.Itoa(bp.Number))
		}
	}

	fmt.Fprintf(b, "\n# %s#%d %s\n\n", g.Main.Repo, g.Main.Number, g.Main.Title)
	fmt.Fprintf(b, "%s%s#%d backports=%s open=%s commit=%s -->\n", sectionStart, g.Main.Repo, g.Main.Number,
		strings.Join(numbers, ","), strings.Join(open, ","), strings.Join(commit, ","))
}

func renderSection(b *strings.Builder, g *BumpGroup, done []*BumpIssue, validated map[string]*BranchResult) {
	writeSectionHeader(b, g, done, validated)

	b.WriteString("## Validated on Version\n\n```console\n")
	for _, bp := range done {
		r := issueResult(validated, bp)
		fmt.Fprintf(b, "# %s\n-$ %s\n", bp.Branch, r.Version)
	}
	b.WriteString("```\n\n")

	writeEnvironment(b, issueResult(validated, done[0]))

	b.WriteString("## Steps to validate the fix\n\n<details>\n<summary>Steps</summary>\n\n")
	installs := "Install each version listed above"
	if slices.ContainsFunc(done, func(bp *BumpIssue) bool { return issueResult(validated, bp).Target.Commit }) {
		installs += " (the latest commit of a release branch whose RC does not hold the bump yet)"
	}
	fmt.Fprintf(b, "1. %s.\n", installs)
	for i, w := range sectionWhats(done, validated) {
		fmt.Fprintf(b, "%d. Confirm %s.\n", i+2, w)
	}
	b.WriteString("\n</details>\n\n## **Validation Results:**\n\n<details>\n<summary>Validation Results</summary>\n\n")

	for _, bp := range done {
		r := issueResult(validated, bp)
		version := r.Target.Version
		fmt.Fprintf(b, "<details>\n<summary>%s (%s)</summary>\n\n```\n", version, bp.Branch)
		for _, c := range r.Target.Checks {
			if c.Issue != bp.Number {
				continue
			}
			res := resultFor(r.Results, c)
			fmt.Fprintf(b, "$ %s\n", displayCmd(c.Cmd))
			for _, out := range res.Output {
				b.WriteString(out + "\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("```\n\n</details>\n\n")
	}
	b.WriteString("</details>\n" + sectionEnd + "\n")
}

// writeEnvironment is the Environment Details of the job that ran the checks: its node and the cluster
// its TFVARS built.
func writeEnvironment(b *strings.Builder, r *BranchResult) {
	b.WriteString("## Environment Details\n\n*Infrastructure*\nAWS EC2 instances\n\n")
	fmt.Fprintf(b, "*Node(s) CPU architecture, OS, and Version:*\n%s, %s\n\n", r.NodeOS, r.Arch)

	b.WriteString("*Cluster Configuration:*\n")
	switch {
	case r.Nodes == "" && r.Flags == "":
		b.WriteString("Not recorded by the job\n\n")
	case r.Flags == "":
		fmt.Fprintf(b, "%s, no server flags\n\n", r.Nodes)
	default:
		fmt.Fprintf(b, "%s\n\n```yaml\n%s\n```\n\n", r.Nodes, r.Flags)
	}
}

// sectionWhats are the steps' "the rke2-canal chart is 3.33.101-build20261120", once each; a values
// check says the chart carries the image, which is what it proves.
func sectionWhats(done []*BumpIssue, validated map[string]*BranchResult) []string {
	var whats []string
	for _, bp := range done {
		for _, c := range issueResult(validated, bp).Target.Checks {
			// "<component> <chart|image> <version> [in <chart> values]"
			f := strings.Fields(c.What)
			if c.Issue != bp.Number || len(f) < 3 {
				continue
			}

			w := fmt.Sprintf("the %s %s is %s", f[0], f[1], f[2])
			if len(f) == 6 && f[3] == "in" {
				w = fmt.Sprintf("the %s values carry the %s image %s", f[4], f[0], f[2])
			}
			if !slices.Contains(whats, w) {
				whats = append(whats, w)
			}
		}
	}

	return whats
}

// resultFor is the check's own console block: its command and its value (two checks can share either).
func resultFor(results []BumpResult, c BumpCheck) BumpResult {
	for _, r := range results {
		if r.Cmd == c.Cmd && r.Expected == c.Expected {
			return r
		}
	}

	return BumpResult{Cmd: c.Cmd, Expected: c.Expected}
}

// BumpSection is one section of a bump MD: its issue, the backports linked to it, those it does not
// validate (Open, keeping the issue open) and those validated on a commit build only (Commit).
type BumpSection struct {
	Repo      string
	Issue     int
	Backports []int
	Open      []int
	Commit    []int
	Body      string
}

var sectionHeader = regexp.MustCompile(
	`^<!-- bump-section ([\w.-]+/[\w.-]+)#(\d+) backports=([\d,]*) open=([\d,]*)(?: commit=([\d,]*))? -->$`)

// issueNumbers reads the header's "1,2" lists.
func issueNumbers(list string) []int {
	var numbers []int
	for _, s := range strings.Split(list, ",") {
		if n, convErr := strconv.Atoi(s); convErr == nil {
			numbers = append(numbers, n)
		}
	}

	return numbers
}

// ParseBumpMD splits a bump MD into its sections (what -bumps-post posts), refusing an unsafe file.
func ParseBumpMD(md string) ([]BumpSection, error) {
	if unsafeErr := CheckBumpMD(md); unsafeErr != nil {
		return nil, unsafeErr
	}

	var sections []BumpSection
	var cur *BumpSection
	var body []string
	for _, line := range strings.Split(md, "\n") {
		if m := sectionHeader.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			cur = &BumpSection{Repo: m[1], Issue: n, Backports: issueNumbers(m[3]), Open: issueNumbers(m[4]),
				Commit: issueNumbers(m[5])}
			body = nil
			continue
		}
		if cur == nil {
			continue
		}
		if line == sectionEnd {
			cur.Body = strings.TrimSpace(strings.Join(body, "\n")) + "\n"
			sections = append(sections, *cur)
			cur = nil
			continue
		}
		body = append(body, line)
	}
	if cur != nil {
		return nil, fmt.Errorf("section of %s#%d has no end marker", cur.Repo, cur.Issue)
	}

	return sections, nil
}
