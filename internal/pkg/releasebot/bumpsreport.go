package releasebot

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The bump MD: one file per product and month, one section per main issue (with the backports it
// validates), each shaped like the team's bump template; the section between its markers is what
// -bumps-post puts on the issue. Nothing in it may point at jenkins or show an address.

// BumpResult is one check as the job ran it.
type BumpResult struct {
	Cmd, Expected string
	Output        []string
}

// BranchResult is a passed version-bump job: what it installed and what its checks printed.
type BranchResult struct {
	Target  *BumpTarget
	Version string // "rke2 version v1.37.2+dev.4b027024 (4b02...)", from the console when it says
	NodeOS  string
	Arch    string
	Results []BumpResult
}

var (
	consoleStamp   = regexp.MustCompile(`^\[\d{4}-\d\d-\d\dT[^\]]*\]\s?`)
	versionLine    = regexp.MustCompile(`\b(?:rke2|k3s) version v\S+ \([0-9a-f]{7,}\)`)
	ipv4           = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	awsHostname    = regexp.MustCompile(`\bip-\d{1,3}-\d{1,3}-\d{1,3}-\d{1,3}\b`)
	jenkinsPointer = regexp.MustCompile(`(?i)https?://[^\s)"]*jenkins[^\s)"]*|/job/[^\s)"]+/\d+/?`)
)

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

func redact(s string) string {
	return awsHostname.ReplaceAllString(ipv4.ReplaceAllString(s, "<REDACTED_IP>"), "<REDACTED_HOST>")
}

// ErrBumpMDUnsafe: the MD would show a jenkins link or an address, so it is not written (or posted).
var ErrBumpMDUnsafe = errors.New("bump MD would show a jenkins link or an address")

// CheckBumpMD refuses text that points at jenkins or shows an address (the issues are public).
func CheckBumpMD(md string) error {
	if m := jenkinsPointer.FindString(md); m != "" {
		return fmt.Errorf("%w: %q", ErrBumpMDUnsafe, m)
	}
	if m := ipv4.FindString(md); m != "" {
		return fmt.Errorf("%w: %q", ErrBumpMDUnsafe, m)
	}

	return nil
}

const (
	sectionStart = "<!-- bump-section "
	sectionEnd   = "<!-- /bump-section -->"
)

// RenderBumpMD writes the product's MD: a section per main issue with at least one validated
// backport. Validated is "branch" → result; groups without one are left out (the run lists them).
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
			if validated[bp.Branch] != nil && slices.ContainsFunc(validated[bp.Branch].Target.Checks,
				func(c BumpCheck) bool { return c.Issue == bp.Number }) {
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

	md := b.String()
	if unsafeErr := CheckBumpMD(md); unsafeErr != nil {
		return "", unsafeErr
	}

	return md, nil
}

func renderSection(b *strings.Builder, g *BumpGroup, done []*BumpIssue, validated map[string]*BranchResult) {
	numbers := make([]string, 0, len(done))
	for _, bp := range done {
		numbers = append(numbers, strconv.Itoa(bp.Number))
	}
	fmt.Fprintf(b, "\n# %s#%d %s\n\n", g.Main.Repo, g.Main.Number, g.Main.Title)
	fmt.Fprintf(b, "%s%s#%d backports=%s -->\n", sectionStart, g.Main.Repo, g.Main.Number, strings.Join(numbers, ","))

	b.WriteString("## Validated on Version\n\n```console\n")
	for _, bp := range done {
		r := validated[bp.Branch]
		version := r.Version
		if version == "" {
			version = fmt.Sprintf("%s version %s", r.Target.Product, r.Target.Version)
		}
		fmt.Fprintf(b, "# %s\n-$ %s\n", bp.Branch, version)
	}
	b.WriteString("```\n\n")

	first := validated[done[0].Branch]
	b.WriteString("## Environment Details\n\n*Infrastructure*\nAWS EC2 instances\n\n")
	fmt.Fprintf(b, "*Node(s) CPU architecture, OS, and Version:*\n%s, %s\n\n", first.NodeOS, first.Arch)
	b.WriteString("*Cluster Configuration:*\nDefault configuration\n\n")

	b.WriteString("## Steps to validate the fix\n\n<details>\n<summary>Steps</summary>\n\n")
	installs := "Install each version listed above"
	if slices.ContainsFunc(done, func(bp *BumpIssue) bool { return validated[bp.Branch].Target.Commit }) {
		installs += " (the latest commit of a release branch whose RC does not hold the bump yet)"
	}
	fmt.Fprintf(b, "1. %s.\n", installs)
	for i, w := range sectionWhats(done, validated) {
		fmt.Fprintf(b, "%d. Confirm %s.\n", i+2, w)
	}
	b.WriteString("\n</details>\n\n## **Validation Results:**\n\n<details>\n<summary>Validation Results</summary>\n\n")

	for _, bp := range done {
		r := validated[bp.Branch]
		version := r.Target.Version
		fmt.Fprintf(b, "<details>\n<summary>%s (%s)</summary>\n\n```\n", version, bp.Branch)
		for _, c := range r.Target.Checks {
			if c.Issue != bp.Number {
				continue
			}
			res := resultFor(r.Results, c.Expected)
			fmt.Fprintf(b, "$ %s\n", displayCmd(c.Cmd))
			for _, out := range res.Output {
				b.WriteString(redact(out) + "\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("```\n\n</details>\n\n")
	}
	b.WriteString("</details>\n" + sectionEnd + "\n")
}

// sectionWhats are the steps' "the rke2-canal chart is 3.33.101-build20261120", once each.
func sectionWhats(done []*BumpIssue, validated map[string]*BranchResult) []string {
	var whats []string
	for _, bp := range done {
		for _, c := range validated[bp.Branch].Target.Checks {
			f := strings.Fields(c.What) // "<component> <chart|image> <version>"
			if c.Issue != bp.Number || len(f) < 3 {
				continue
			}
			if w := fmt.Sprintf("the %s %s is %s", f[0], f[1], f[2]); !slices.Contains(whats, w) {
				whats = append(whats, w)
			}
		}
	}

	return whats
}

// resultFor is the console's result of the check expecting this value (the job runs them in order).
func resultFor(results []BumpResult, expected string) BumpResult {
	for _, r := range results {
		if r.Expected == expected {
			return r
		}
	}

	return BumpResult{Expected: expected}
}

// BumpSection is one section of a bump MD: the issue it goes on and the backports linked to it.
type BumpSection struct {
	Repo      string
	Issue     int
	Backports []int
	Body      string
}

var sectionHeader = regexp.MustCompile(`^<!-- bump-section ([\w.-]+/[\w.-]+)#(\d+) backports=([\d,]*) -->$`)

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
			cur, body = &BumpSection{Repo: m[1], Issue: n}, nil
			for _, s := range strings.Split(m[3], ",") {
				if bp, convErr := strconv.Atoi(s); convErr == nil {
					cur.Backports = append(cur.Backports, bp)
				}
			}
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
