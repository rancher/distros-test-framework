package releasebot

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Bump mode: the boards' version-bump issues, validated by version-bump jobs and reported in one MD
// per product and month (docs/releasebot.md, "Bump issues").

// BumpConfig is config/releasebot/bumps.yaml; the CLI sets PrefixBase (PrefixFromLogin) and Products
// (-bumps-product, empty for both boards).
type BumpConfig struct {
	Controller    string              `yaml:"controller"`
	Jobs          map[string]string   `yaml:"jobs"`
	CNIJobs       map[string][]CNIJob `yaml:"cniJobs"`
	DefaultImages map[string][]string `yaml:"defaultImages"`
	PrefixBase    string              `yaml:"-"`
	Products      []string            `yaml:"-"`
}

// CNIJob is a version-bump job that installs a CNI, and the components (charts, images) that need it.
type CNIJob struct {
	Name       string   `yaml:"name"`
	Job        string   `yaml:"job"`
	Components []string `yaml:"components"`
}

// JobPaths are every bump job, the CNI ones included (BusyLimit tells our builds by them).
func (c *BumpConfig) JobPaths() []string {
	paths := slices.Sorted(maps.Values(c.Jobs))
	for _, jobs := range c.CNIJobs {
		for _, j := range jobs {
			if !slices.Contains(paths, j.Job) {
				paths = append(paths, j.Job)
			}
		}
	}

	return paths
}

// jobFor is the job an issue's checks run on: the CNI job of its components, else the product's.
// An issue that needs two CNIs cannot run on one job (why).
func (c *BumpConfig) jobFor(issue *BumpIssue) (path, name, why string) {
	var matched []CNIJob
	for _, j := range c.CNIJobs[issue.Product] {
		if slices.ContainsFunc(issue.Checks, func(ch BumpCheck) bool {
			return slices.Contains(j.Components, strings.Fields(ch.What)[0])
		}) && !slices.ContainsFunc(matched, func(m CNIJob) bool { return m.Job == j.Job }) {
			matched = append(matched, j)
		}
	}

	switch {
	case len(matched) == 0:
		return c.Jobs[issue.Product], "", ""
	case len(matched) > 1:
		return "", "", fmt.Sprintf("its checks need both the %s and the %s install", matched[0].Name, matched[1].Name)
	case matched[0].Job == c.Jobs[issue.Product]:
		return matched[0].Job, "", ""
	}

	return matched[0].Job, matched[0].Name, ""
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
	Parent int `json:"parent,omitempty"`

	// OffBoard: a backport of a To Test main issue that sits in another column of the board.
	OffBoard bool        `json:"off_board,omitempty"`
	PRs      []BumpPR    `json:"prs,omitempty"`
	Checks   []BumpCheck `json:"checks,omitempty"`

	// JobPath and JobName are the version-bump job its checks run on (BumpConfig.jobFor).
	JobPath string `json:"job,omitempty"`
	JobName string `json:"job_name,omitempty"`
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

	// Replaced is the issue's version when a later change (ReplacedBy, "rancher/rke2#11949") bumped it
	// again in the version installed: the check expects that newer version.
	Replaced   string `json:"replaced,omitempty"`
	ReplacedBy string `json:"replaced_by,omitempty"`
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

// PureBump: removed and added lines match once versions and image tags are masked; an added image-only
// line (a new image-list entry) and go.sum pass too.
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

// BumpChecks derives the job's checks from the PR's changes (charts, node images, chart values); what
// no rule covers is returned as unsupported, for a person.
func BumpChecks(product string, files []PRFile, defaultImages []string) (checks []BumpCheck, unsupported []string) {
	var untested []bumpedImage
	for _, f := range files {
		got := chartChecks(product, f.Name, f.Patch)
		images, others := imageChecks(f.Patch, defaultImages)
		got = append(got, images...)
		untested = append(untested, others...)

		if len(got) == 0 && len(others) == 0 && !strings.HasSuffix(f.Name, "go.sum") &&
			f.Name != "scripts/build-images" && f.Name != "scripts/airgap/image-list.txt" {
			unsupported = append(unsupported, f.Name)
		}
		checks = append(checks, got...)
	}

	values, left := valuesChecks(product, checks, untested)
	checks = append(checks, values...)
	for _, u := range left {
		if !slices.Contains(unsupported, u) {
			unsupported = append(unsupported, u)
		}
	}

	return dedupChecks(checks), unsupported
}

// chartChecks are the chart bumps of one changed file: RKE2's chart_versions.yaml, a K3s manifest.
func chartChecks(product, name, patch string) []BumpCheck {
	switch {
	case product == "rke2" && name == "charts/chart_versions.yaml":
		return rke2ChartChecks(patch)
	case product == "k3s" && strings.HasPrefix(name, "manifests/") && strings.HasSuffix(name, ".yaml"):
		return k3sChartChecks(name, patch)
	}

	return nil
}

// versionsIn are the versions a whole file sets: "<chart> chart" and "<image> image|<section>" (an
// image list repeats an image per CNI, at different tags), read with the same rules as a bump's patch.
func versionsIn(product, name, content string) map[string][]string {
	versions := map[string][]string{}
	add := func(key, version string) {
		if !slices.Contains(versions[key], version) {
			versions[key] = append(versions[key], version)
		}
	}

	for _, c := range chartChecks(product, name, "+"+strings.ReplaceAll(content, "\n", "\n+")) {
		f := strings.Fields(c.What)
		add(f[0]+" chart", f[2])
	}

	section := ""
	for _, l := range strings.Split(content, "\n") {
		for _, m := range imageRef.FindAllStringSubmatch(l, -1) {
			add(m[1]+" image|"+section, m[2])
		}
		if sectionLine.MatchString(l) {
			section = sectionOf(l)
		}
	}

	return versions
}

// setsVersion: the patch adds version for one of keys, versionsIn keys ("rke2-canal chart", "<image>
// image|<section>"), read as BumpChecks reads it: a chart with its filename line, an image in its section.
func setsVersion(product, path, patch string, keys []string, version string) bool {
	for _, key := range keys {
		name, kind, _ := strings.Cut(key, " ")
		switch {
		case kind == "chart":
			if slices.ContainsFunc(chartChecks(product, path, patch),
				func(c BumpCheck) bool { return c.What == name+" chart "+version }) {
				return true
			}
		case slices.Contains(imageSections(patch)["rancher/"+name+":"+version], strings.TrimPrefix(kind, "image|")):
			return true
		}
	}

	return false
}

// sectionLine is a line git names in a hunk header ("@@ -72,7 +72,7 @@ xargs ... images-cilium.txt").
var sectionLine = regexp.MustCompile(`^[A-Za-z$_]`)

// sectionOf is a section line as both a patch's hunk header and the whole file show it.
func sectionOf(line string) string {
	return maskVersions(strings.TrimRight(line[:min(72, len(line))], " \t"))
}

// imageSections maps each image an added line sets ("rancher/x:tag") to its sections (a PR can set one
// tag for two CNIs), each the last section line before it in the new file.
func imageSections(patch string) map[string][]string {
	sections := map[string][]string{}
	section := ""
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			if _, name, ok := strings.Cut(strings.TrimPrefix(l, "@@"), "@@"); ok {
				section = sectionOf(strings.TrimSpace(name))
			}
			continue
		case l == "" || l[0] == '-' || l[0] == '\\':
			continue
		case l[0] == '+':
			for _, m := range imageRef.FindAllStringSubmatch(l[1:], -1) {
				if image := "rancher/" + m[1] + ":" + m[2]; !slices.Contains(sections[image], section) {
					sections[image] = append(sections[image], section)
				}
			}
		}
		if sectionLine.MatchString(l[1:]) {
			section = sectionOf(l[1:])
		}
	}

	return sections
}

var digitRun = regexp.MustCompile(`\d+`)

// newerVersion: b is a later release of a, the same shape once numbers are masked ("v" aside) and
// higher at its first different number (3.33.103-build20261120 after 3.33.101-build20261120).
func newerVersion(a, b string) bool {
	a, b = strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v")
	if digitRun.ReplaceAllString(a, "0") != digitRun.ReplaceAllString(b, "0") {
		return false
	}

	na, nb := digitRun.FindAllString(a, -1), digitRun.FindAllString(b, -1)
	for i := range na {
		x, y := strings.TrimLeft(na[i], "0"), strings.TrimLeft(nb[i], "0")
		if x != y {
			return len(y) > len(x) || (len(y) == len(x) && y > x)
		}
	}

	return false
}

// bumpedImage is an image tag a pull request adds.
type bumpedImage struct{ name, tag string }

// operatorVariant: the cilium chart names one operator image; its template adds the cloud suffix.
var operatorVariant = regexp.MustCompile(`-(?:aws|azure|alibabacloud|generic)$`)

// valuesChecks reads the images no default install runs from the values of the RKE2 chart the PR bumps
// for them (chartFor), in its bundled chart file on the node; with no such chart, a person checks them.
func valuesChecks(product string, checks []BumpCheck, images []bumpedImage) (values []BumpCheck, left []string) {
	var charts []string
	for _, c := range checks {
		if f := strings.Fields(c.What); f[1] == "chart" && !slices.Contains(charts, f[0]) {
			charts = append(charts, f[0])
		}
	}

	for _, img := range images {
		chart := chartFor(img.name, charts)
		if product != "rke2" || chart == "" {
			left = append(left, fmt.Sprintf("rancher/%s:%s (not a default image, and no single chart, or one named "+
				"after it, to read it from)", img.name, img.tag))
			continue
		}

		// comment lines go first: rke2-traefik documents its image tag between repository and tag
		values = append(values, BumpCheck{
			What: fmt.Sprintf("%s image %s in %s values", img.name, img.tag, chart),
			Cmd: "sudo grep -h chartContent /var/lib/rancher/rke2/data/*/charts/" + chart + ".yaml" +
				" | sed s/.*chartContent.// | base64 -di | tar xzOf - " + chart + "/values.yaml" +
				" | grep -v -E ^[[:space:]]*# | grep -A1 rancher/" + operatorVariant.ReplaceAllString(img.name, ""),
			Expected: img.tag,
		})
	}

	return values, left
}

// chartFor is the bumped chart whose values carry the image: the only one, or the one named after it
// (hardened-traefik in rke2-traefik, not rke2-traefik-crd); "" when that is not one chart.
func chartFor(image string, charts []string) string {
	if len(charts) == 1 {
		return charts[0]
	}

	base := strings.TrimPrefix(strings.TrimPrefix(image, "hardened-"), "mirrored-")
	var named []string
	for _, c := range charts {
		if strings.TrimPrefix(c, "rke2-") == base {
			named = append(named, c)
		}
	}
	if len(named) != 1 {
		return ""
	}

	return named[0]
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

// imageChecks: an added image tag of an image a default install runs, seen in the node's images; the
// others are returned, to be checked in their chart (valuesChecks) or left to a person.
func imageChecks(patch string, defaultImages []string) (checks []BumpCheck, others []bumpedImage) {
	_, added := changedLines(patch)
	for _, l := range added {
		for _, m := range imageRef.FindAllStringSubmatch(l, -1) {
			if !slices.Contains(defaultImages, m[1]) {
				if img := (bumpedImage{m[1], m[2]}); !slices.Contains(others, img) {
					others = append(others, img)
				}
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

	return checks, others
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

// BumpTarget is one version-bump job: a branch, the RC tag or latest commit it installs, its checks and
// its job (JobName names a CNI job, "canal"; empty for the product's).
type BumpTarget struct {
	Product string      `json:"product"`
	Branch  string      `json:"branch"`
	Version string      `json:"version"`
	Commit  bool        `json:"commit,omitempty"`
	Checks  []BumpCheck `json:"checks"`
	JobPath string      `json:"job,omitempty"`
	JobName string      `json:"job_name,omitempty"`
}

// Name is the run's name for the target's job: "rke2-bumps-137", "rke2-bumps-137-canal".
func (t *BumpTarget) Name() string {
	name := t.Product + "-bumps-" + t.Minor()
	if t.JobName != "" {
		name += "-" + t.JobName
	}

	return name
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

	// <base><minor><product letter>, e.g. fmor137r: within qainfra's 9 characters for rke2.
	if c.PrefixBase == "" {
		return JenkinsJob{}, fmt.Errorf("no HOSTNAME_PREFIX base: set %s to your SUSE login", SUSELoginEnv)
	}
	prefix := c.PrefixBase + t.Minor() + t.Product[:1]
	if t.JobName != "" {
		// fmor137rc: tells the canal job's nodes from the other job's
		prefix += t.JobName[:1]
	}
	if budget := hostnameBudget - len(t.Product); len(prefix) > budget {
		return JenkinsJob{}, fmt.Errorf("hostname prefix %q is longer than %d characters", prefix, budget)
	}

	mode := "INSTALL_" + strings.ToUpper(t.Product) + "_VERSION"
	if t.Commit {
		mode = "INSTALL_" + strings.ToUpper(t.Product) + "_COMMIT"
	}

	path := t.JobPath
	if path == "" {
		path = c.Jobs[t.Product]
	}

	return JenkinsJob{
		Name: t.Name(), Product: t.Product, Version: t.Version, Controller: controller, Path: path, Phase: 1,
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
