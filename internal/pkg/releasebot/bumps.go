package releasebot

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Bump mode: the "To Test" issues of the K3s and RKE2 projects that are only version bumps are
// validated with one version-bump job per product and release branch, and reported in one MD per
// product and month (bumpsreport.go); bumpsgithub.go reads the issues and their PRs.

// BumpConfig is config/releasebot/bumps.yaml.
type BumpConfig struct {
	Controller string            `yaml:"controller"`
	PrefixBase string            `yaml:"prefixBase"`
	Jobs       map[string]string `yaml:"jobs"`

	// DefaultImages run on a default install, so their tag shows on the node; a bump of any other
	// image is left to a person.
	DefaultImages map[string][]string `yaml:"defaultImages"`
}

// LoadBumpConfig reads and checks the bump mode's config.
func LoadBumpConfig(path string) (*BumpConfig, error) {
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}

	var c BumpConfig
	if decodeErr := yaml.Unmarshal(raw, &c); decodeErr != nil {
		return nil, fmt.Errorf("bump config %s: %w", path, decodeErr)
	}
	for _, product := range []string{"k3s", "rke2"} {
		if c.Jobs[product] == "" {
			return nil, fmt.Errorf("bump config %s: no job for %s", path, product)
		}
	}
	if c.PrefixBase == "" {
		c.PrefixBase = "rb"
	}
	if c.Controller == "" {
		c.Controller = "mower"
	}

	return &c, nil
}

// BumpIssue is one "To Test" issue and the merged PRs that fixed it.
type BumpIssue struct {
	Product string `json:"product"`
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`

	// Branch is the backport's release branch ("release-1.37"); "" is the main issue (master).
	Branch string `json:"branch,omitempty"`

	// Parent is the main issue a backport's body points at (0 when none).
	Parent int         `json:"parent,omitempty"`
	PRs    []BumpPR    `json:"prs,omitempty"`
	Checks []BumpCheck `json:"checks,omitempty"`
}

type BumpPR struct {
	Number      int      `json:"number"`
	Base        string   `json:"base"`
	MergeCommit string   `json:"merge_commit"`
	Files       []PRFile `json:"-"`
}

type PRFile struct {
	Name, Patch string
}

// BumpCheck is one command the version-bump job runs and the text its output must contain.
type BumpCheck struct {
	What     string `json:"what"`
	Cmd      string `json:"cmd"`
	Expected string `json:"expected"`
	Issue    int    `json:"issue"`
}

var (
	releasePrefix = regexp.MustCompile(`(?i)^\s*\[release[- ]?(1\.\d+)\]\s*-?\s*`)
	bumpVerb      = regexp.MustCompile(`(?i)^(bump|update|upgrade)\b`)
	parentRef     = regexp.MustCompile(`(?:/issues/|#)(\d+)\b`)
	parentPull    = regexp.MustCompile(`/pull/(\d+)\b`)

	// versionToken covers semver, chart versions with +up or -build suffixes, and digests.
	versionToken = regexp.MustCompile(`v?\d+(?:\.\d+)+(?:[-+][0-9A-Za-z.+_-]*)?|\b[0-9a-f]{40,64}\b`)
	imageRef     = regexp.MustCompile(`(?:^|[\s"/%])rancher/([a-z0-9][a-z0-9._-]*):([0-9A-Za-z][0-9A-Za-z._+-]*)`)
	chartVersion = regexp.MustCompile(`^\s*-\s*version:\s*"?([^"\s]+)"?\s*$`)
	chartFile    = regexp.MustCompile(`^\s*filename:\s*/charts/([a-z0-9-]+)\.yaml\s*$`)
	k3sChartURL  = regexp.MustCompile(`static/charts/([a-z0-9-]+?)-(\d[0-9A-Za-z.+_-]*)\.tgz`)
)

// IsBumpTitle: "Bump X", "Update X chart to Y", "[Release-1.37] Bump X" and the like.
func IsBumpTitle(title string) bool {
	return bumpVerb.MatchString(releasePrefix.ReplaceAllString(title, ""))
}

// BranchOf is the release branch a backport's title names, or "" for the main issue.
func BranchOf(title string) string {
	m := releasePrefix.FindStringSubmatch(title)
	if m == nil {
		return ""
	}

	return "release-" + m[1]
}

// ParentOf is what a backport's body points at: an issue ("Backport for #11882", an issue URL) or,
// when it names the pull request instead (".../pull/11776"), that pull request.
func ParentOf(body string) (issue, pull int) {
	if m := parentRef.FindStringSubmatch(body); m != nil {
		issue, _ = strconv.Atoi(m[1])
	}
	if m := parentPull.FindStringSubmatch(body); m != nil {
		pull, _ = strconv.Atoi(m[1])
	}

	return issue, pull
}

// sameBump is a title without its release prefix, case and dashes: a backport whose body names no
// issue still finds its main issue by title.
func sameBump(title string) string {
	t := strings.ToLower(releasePrefix.ReplaceAllString(title, ""))

	return strings.Join(strings.Fields(strings.ReplaceAll(t, "-", " ")), " ")
}

// PureBump reports whether every changed line only changes versions: removed and added lines match
// once their versions and image tags are masked, the rest of an image line included. An added line
// that is only an image (a new entry of an image list) passes too; go.sum is versions by nature.
func PureBump(files []PRFile) (pure bool, why string) {
	for _, f := range files {
		if strings.HasSuffix(f.Name, "go.sum") {
			continue
		}

		removed, added := changedLines(f.Patch)
		left := map[string]int{}
		for _, l := range removed {
			left[maskVersions(l)]++
		}
		for _, l := range added {
			k := maskVersions(l)
			switch {
			case left[k] > 0:
				left[k]--
			case imageOnly(l):
			default:
				return false, fmt.Sprintf("%s: %q changes more than a version", f.Name, strings.TrimSpace(l))
			}
		}
		for k, n := range left {
			if n > 0 {
				return false, fmt.Sprintf("%s: %q is removed, not bumped", f.Name, strings.TrimSpace(k))
			}
		}
	}

	return true, ""
}

// maskVersions masks image tags (any tag, "latest" too) and then version tokens.
func maskVersions(line string) string {
	tags := imageRef.ReplaceAllStringFunc(line, func(m string) string { return m[:strings.LastIndex(m, ":")+1] + "<v>" })

	return strings.TrimSpace(versionToken.ReplaceAllString(tags, "<v>"))
}

// imageListRest is what an image-list line holds besides its image: a list marker, quotes, a comma or
// a line continuation, and a registry ("${REGISTRY}", "docker.io").
var imageListRest = regexp.MustCompile(`^[-\s"'\\,]*(?:\$\{?\w+\}?|[\w.-]+\.[a-z]{2,})?[\s"'\\,]*$`)

// imageOnly: the line is only an image reference (no key, no setting next to it).
func imageOnly(line string) bool {
	return imageRef.MatchString(line) && imageListRest.MatchString(imageRef.ReplaceAllString(line, ""))
}

// changedLines splits a unified diff into its removed and added lines (headers excluded).
func changedLines(patch string) (removed, added []string) {
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "-"):
			removed = append(removed, l[1:])
		case strings.HasPrefix(l, "+"):
			added = append(added, l[1:])
		}
	}

	return removed, added
}

// BumpChecks derives what the job checks from the PR's changes; a change no rule covers is returned
// as unsupported, for a person: a file, or a bumped image a default install does not run. RKE2 charts
// are checked on the chart file every install bundles, K3s charts on the server manifest, and images
// (default ones only) on the node.
func BumpChecks(product string, files []PRFile, defaultImages []string) (checks []BumpCheck, unsupported []string) {
	for _, f := range files {
		var got []BumpCheck
		switch {
		case product == "rke2" && f.Name == "charts/chart_versions.yaml":
			got = rke2ChartChecks(f.Patch)
		case product == "k3s" && strings.HasPrefix(f.Name, "manifests/") && strings.HasSuffix(f.Name, ".yaml"):
			got = k3sChartChecks(f.Name, f.Patch)
		}
		images, untested := imageChecks(f.Patch, defaultImages)
		got = append(got, images...)

		for _, u := range untested {
			if !slices.Contains(unsupported, u) {
				unsupported = append(unsupported, u)
			}
		}
		if len(got) == 0 && len(untested) == 0 && !strings.HasSuffix(f.Name, "go.sum") &&
			f.Name != "scripts/build-images" && f.Name != "scripts/airgap/image-list.txt" {
			unsupported = append(unsupported, f.Name)
		}
		checks = append(checks, got...)
	}

	return dedupChecks(checks), unsupported
}

// rke2ChartChecks: a "- version: X" line followed by "filename: /charts/<chart>.yaml" bumps that chart.
func rke2ChartChecks(patch string) []BumpCheck {
	lines := strings.Split(patch, "\n")
	var checks []BumpCheck
	for i, l := range lines {
		if !strings.HasPrefix(l, "+") {
			continue
		}
		v := chartVersion.FindStringSubmatch(l[1:])
		if v == nil {
			continue
		}

		for _, next := range lines[i+1:] {
			if next != "" && (next[0] == '-' || strings.Contains(next, "version:")) {
				break
			}
			if f := chartFile.FindStringSubmatch(strings.TrimLeft(next, "+ ")); f != nil {
				checks = append(checks, BumpCheck{
					What:     fmt.Sprintf("%s chart %s", f[1], v[1]),
					Cmd:      "sudo grep -h chart-url /var/lib/rancher/rke2/data/*/charts/" + f[1] + ".yaml",
					Expected: f[1] + "-" + v[1] + ".tgz",
				})

				break
			}
		}
	}

	return checks
}

// k3sChartChecks: an added chart URL in a server manifest (traefik.yaml: traefik, traefik-crd).
func k3sChartChecks(name, patch string) []BumpCheck {
	_, added := changedLines(patch)
	var checks []BumpCheck
	for _, l := range added {
		m := k3sChartURL.FindStringSubmatch(l)
		if m == nil {
			continue
		}

		checks = append(checks, BumpCheck{
			What:     fmt.Sprintf("%s chart %s", m[1], m[2]),
			Cmd:      "sudo grep -h static/charts/ /var/lib/rancher/k3s/server/" + name,
			Expected: m[1] + "-" + m[2] + ".tgz",
		})
	}

	return checks
}

// imageChecks: an added image tag of an image a default install runs, seen in the node's images; any
// other added image is untested, so its issue goes to a person instead of passing on its other checks.
func imageChecks(patch string, defaultImages []string) (checks []BumpCheck, untested []string) {
	_, added := changedLines(patch)
	for _, l := range added {
		for _, m := range imageRef.FindAllStringSubmatch(l, -1) {
			if !slices.Contains(defaultImages, m[1]) {
				untested = append(untested, fmt.Sprintf("rancher/%s:%s (not a default image)", m[1], m[2]))
				continue
			}

			// No ":" after the image: the job's resources.JoinCommands turns every ":" of a kubectl
			// command into ",", and the expected "rancher/<img>:<tag>" keeps the check exact.
			checks = append(checks, BumpCheck{
				What:     fmt.Sprintf("%s image %s", m[1], m[2]),
				Cmd:      "kubectl get node -o yaml : | grep rancher/" + m[1],
				Expected: "rancher/" + m[1] + ":" + m[2],
			})
		}
	}

	return checks, untested
}

func dedupChecks(in []BumpCheck) []BumpCheck {
	seen := map[string]bool{}
	out := in[:0]
	for _, c := range in {
		if k := c.Cmd + "|" + c.Expected; !seen[k] {
			seen[k] = true
			out = append(out, c)
		}
	}

	return out
}

// BumpTarget is one version-bump job: a product's release branch, the version it installs and every
// bump check of that branch.
type BumpTarget struct {
	Product string      `json:"product"`
	Branch  string      `json:"branch"`
	Version string      `json:"version"` // an RC tag, or the branch's latest commit
	Commit  bool        `json:"commit,omitempty"`
	Checks  []BumpCheck `json:"checks"`
}

// Minor is "137" for release-1.37, the part of the hostname prefix that names the branch.
func (t *BumpTarget) Minor() string {
	return strings.ReplaceAll(strings.TrimPrefix(t.Branch, "release-"), ".", "")
}

// TestArgs is the job's TEST_ARGS: the checks as -cmd and -expectedValue (comma-separated, in the
// same order). No check may hold a comma or a single quote: the job runs TEST_ARGS inside sh -c '...'.
func (t *BumpTarget) TestArgs() (string, error) {
	cmds := make([]string, 0, len(t.Checks))
	expected := make([]string, 0, len(t.Checks))
	for _, c := range t.Checks {
		if strings.ContainsAny(c.Cmd+c.Expected, ",'\"") {
			return "", fmt.Errorf("check %q cannot be passed to the job (comma or quote)", c.What)
		}
		cmds = append(cmds, c.Cmd)
		expected = append(expected, c.Expected)
	}

	return `-tags=versionbump -cmd="` + strings.Join(cmds, ", ") + `" -expectedValue="` +
		strings.Join(expected, ",") + `" -destroy true`, nil
}

// Job is the jenkins job that validates the target (never reported to Qase: bumps are reported on
// their own issues).
func (t *BumpTarget) Job(c *BumpConfig, controller, dtfRef string) (JenkinsJob, error) {
	args, argsErr := t.TestArgs()
	if argsErr != nil {
		return JenkinsJob{}, argsErr
	}

	prefix := c.PrefixBase + t.Product[:1] + t.Minor() + "b"
	if len(prefix) > 8 {
		return JenkinsJob{}, fmt.Errorf("hostname prefix %q is longer than 8 characters", prefix)
	}

	mode := "INSTALL_" + strings.ToUpper(t.Product) + "_VERSION"
	if t.Commit {
		mode = "INSTALL_" + strings.ToUpper(t.Product) + "_COMMIT"
	}

	return JenkinsJob{
		Name: t.Product + "-bumps-" + t.Minor(), Product: t.Product, Version: t.Version, Controller: controller,
		Path: c.Jobs[t.Product], Phase: 1,
		Params: map[string]string{
			"INSTALL_MODE": mode, "INSTALL_VERSION": t.Version, "TEST_ARGS": args,
			"HOSTNAME_PREFIX": prefix, "BRANCH": dtfRef,
		},
	}, nil
}

// BumpGroup is a main issue and its backports, reported together.
type BumpGroup struct {
	Product   string       `json:"product"`
	Main      BumpIssue    `json:"main"`
	Backports []*BumpIssue `json:"backports"`
}

// GroupBumps puts each backport under the main issue its body points at; a backport whose parent is
// unknown becomes its own group.
func GroupBumps(issues []*BumpIssue) []*BumpGroup {
	mains := map[string]*BumpGroup{}
	key := func(i *BumpIssue, n int) string { return i.Repo + "#" + strconv.Itoa(n) }

	var groups []*BumpGroup
	for _, i := range issues {
		if i.Branch == "" {
			g := &BumpGroup{Product: i.Product, Main: *i}
			mains[key(i, i.Number)] = g
			groups = append(groups, g)
		}
	}
	byTitle := map[string]int{}
	for _, i := range issues {
		if i.Branch == "" {
			byTitle[i.Repo+"|"+sameBump(i.Title)] = i.Number
		}
	}
	for _, i := range issues {
		if i.Branch == "" {
			continue
		}
		if i.Parent == 0 {
			i.Parent = byTitle[i.Repo+"|"+sameBump(i.Title)]
		}

		g := mains[key(i, i.Parent)]
		if g == nil {
			g = &BumpGroup{Product: i.Product, Main: BumpIssue{Product: i.Product, Repo: i.Repo, Number: i.Parent,
				Title: releasePrefix.ReplaceAllString(i.Title, "")}}
			mains[key(i, i.Parent)] = g
			groups = append(groups, g)
		}
		g.Backports = append(g.Backports, i)
	}

	for _, g := range groups {
		sort.Slice(g.Backports, func(a, b int) bool { return g.Backports[a].Branch > g.Backports[b].Branch })
	}

	return groups
}
