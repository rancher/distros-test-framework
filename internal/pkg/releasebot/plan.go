package releasebot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkflowDispatch is one GitHub Actions workflow_dispatch call.
type WorkflowDispatch struct {
	Repo     string
	Workflow string
	Ref      string
	Inputs   map[string]string
}

// JenkinsJob is one build to trigger, already expanded for a single version.
type JenkinsJob struct {
	Name string
	// Dependencies refer to jobs of the same product and version.
	DependsOn  []string
	Controller string
	Path       string
	Product    string
	Version    string
	Phase      int
	Priority   int
	Params     map[string]string
	// Zero for the initial build; incremented on reruns.
	Attempt   int
	QaseTitle string
}

// qaseRunPlaceholder is replaced by the run id once the Qase workflow has created the runs.
const qaseRunPlaceholder = "{{QASE_RUN_ID}}"

// Plan is everything a release request would do, in order.
type Plan struct {
	Request Request

	RequestID string
	Workflows []WorkflowDispatch
	// The workflow creates runs for both products for every version.
	QaseTitles []string
	Jobs       []JenkinsJob
	// Missing optional jobs do not block phase progression.
	Skipped []SkippedJob
	// Missing required jobs prevent execution of the plan.
	Blocked []SkippedJob
	// Execution is refused when Jenkins parameters could not be verified.
	Unchecked   []string
	checkErr    error
	UpgradeFrom map[string]string
	Warnings    []string
}

// SkippedJob is a matrix job that does not run for one RC.
type SkippedJob struct {
	Path, Version, Reason string
}

// RunnableError says why the plan's jobs must not be triggered: a required job that cannot run,
// or jobs whose parameters were never verified (Jenkins would silently ignore unknown ones).
func (p *Plan) RunnableError() error {
	var problems []string
	if len(p.Blocked) > 0 {
		problems = append(problems, fmt.Sprintf("%d required jobs cannot run (first: %s %s: %s)",
			len(p.Blocked), p.Blocked[0].Path, p.Blocked[0].Version, p.Blocked[0].Reason))
	}
	if len(p.Unchecked) > 0 {
		problems = append(problems, fmt.Sprintf("%d jobs could not be checked on Jenkins (%v)",
			len(p.Unchecked), p.checkErr))
	}
	if len(problems) == 0 {
		return nil
	}

	return errors.New(strings.Join(problems, "; ") + "; fix them and ask again")
}

// Resolver answers what a plan needs from outside. buildPlan works without one (tests, no
// credentials): jobs are then not checked and upgrade jobs are Blocked (no start version).
type Resolver interface {
	// JobParams returns the job's parameter names; exists is false when Jenkins has no such job.
	JobParams(ctx context.Context, controller, path string) (params []string, exists bool, err error)
	// LatestGA returns the release an upgrade to rc starts from, with a note when it is unusual.
	LatestGA(ctx context.Context, product, rc string) (version, note string, err error)
}

// Matrix is the versioned job list (config/releasebot/matrix.yaml).
type Matrix struct {
	DTFRepo    string            `yaml:"dtfRepo"`
	DTFRef     string            `yaml:"dtfRef"`
	PrefixBase string            `yaml:"prefixBase"`
	Controller map[string]Limits `yaml:"controllers"`
	Jobs       []MatrixJob       `yaml:"jobs"`
	Defaults   map[string]string `yaml:"defaultParams"`
	// OptionalParams are one all-or-none group (Qase reporting): a job lacking any of them runs without
	// all of them, with a plan warning. A job missing any other parameter the bot sets is refused.
	OptionalParams []string `yaml:"optionalParams"`
}

// Limits bounds how many builds may run at once on a controller.
type Limits struct {
	URL           string `yaml:"url"`
	MaxConcurrent int    `yaml:"maxConcurrent"`
}

// MatrixJob is a job template; {{VERSION}}, {{PREFIX}} and {{UPGRADE_FROM}} are replaced per
// RC, {{QASE_RUN_ID}} once the Qase runs exist.
type MatrixJob struct {
	Name       string            `yaml:"name"`
	Product    string            `yaml:"product"`
	Controller string            `yaml:"controller"`
	Path       string            `yaml:"path"`
	Params     map[string]string `yaml:"params"`
	// Phase gates the job per product+RC: it starts once every job of the previous phase passed.
	Phase int `yaml:"phase"`
	// Split shares the product's RCs among the jobs of the same group, newest RCs to the first job.
	Split string `yaml:"split"`
	Code  string `yaml:"code"`
	// Optional jobs may be missing on the controller (not created yet): they are skipped, and the
	// next phase does not wait for them. A missing required job refuses the plan.
	Optional bool `yaml:"optional"`
	// Priority orders jobs that can start (default: the phase).
	Priority int `yaml:"priority"`
	// DependsOn lists job names of the same product; they must succeed for the same RC first.
	DependsOn []string `yaml:"dependsOn"`
}

const (
	upgradeFromPlaceholder = "{{UPGRADE_FROM}}"
	// qainfra names resources dsf-<HOSTNAME_PREFIX>-<product>-<5-char id> within 24 chars.
	hostnameBudget = 24 - len("dsf-") - len("--") - 5
)

var jobCode = regexp.MustCompile(`^[a-z0-9]{1,2}$`)

// loadMatrix reads and validates the matrix file.
func loadMatrix(path string) (*Matrix, error) {
	return LoadMatrixWithRef(path, "")
}

// LoadMatrixWithRef applies an explicit branch override before expanding {{DTF_REF}}.
func LoadMatrixWithRef(path, dtfRef string) (*Matrix, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read matrix: %w", err)
	}

	// Unknown keys are errors: a typo such as depends_on would otherwise drop dependencies silently.
	var m Matrix
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err = dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse matrix: %w", err)
	}

	if m.PrefixBase == "" {
		m.PrefixBase = "rb"
	}
	if jobsErr := validateJobs(&m); jobsErr != nil {
		return nil, jobsErr
	}

	if m.DTFRepo == "" {
		m.DTFRepo = "rancher/distros-test-framework"
	}
	if m.DTFRef == "" {
		m.DTFRef = "main"
	}
	if dtfRef != "" {
		m.DTFRef = dtfRef
	}
	for k, v := range m.Defaults {
		m.Defaults[k] = strings.ReplaceAll(v, dtfRefPlaceholder, m.DTFRef)
	}
	for i := range m.Jobs {
		for k, v := range m.Jobs[i].Params {
			m.Jobs[i].Params[k] = strings.ReplaceAll(v, dtfRefPlaceholder, m.DTFRef)
		}
	}

	return &m, nil
}

// DTFRefEnv is read by the CLI, not by matrix loading.
const DTFRefEnv = "RELEASEBOT_DTF_REF"

const dtfRefPlaceholder = "{{DTF_REF}}"

// validateJobs fills phase/priority defaults and rejects bad products, controllers, names,
// prefixes and dependencies.
func validateJobs(m *Matrix) error {
	seen := map[string]bool{}
	codes := map[string]string{}
	probe := make([]JenkinsJob, 0, len(m.Jobs))
	for i := range m.Jobs {
		j := &m.Jobs[i]
		if j.Product != "k3s" && j.Product != "rke2" {
			return fmt.Errorf("job %q: product must be k3s or rke2", j.Name)
		}
		if j.Phase == 0 {
			j.Phase = 1
		}
		if j.Priority == 0 {
			j.Priority = j.Phase
		}
		if err := checkPrefix(m, j, codes); err != nil {
			return err
		}
		lim, ok := m.Controller[j.Controller]
		if !ok || lim.URL == "" || lim.MaxConcurrent < 1 {
			return fmt.Errorf("job %q: controller %q missing url or maxConcurrent", j.Name, j.Controller)
		}
		if j.Name == "" || seen[j.Product+"|"+j.Name] {
			return fmt.Errorf("job %q (%s): names must be set and unique per product", j.Name, j.Product)
		}
		seen[j.Product+"|"+j.Name] = true
		probe = append(probe, JenkinsJob{Name: j.Name, Product: j.Product, DependsOn: j.DependsOn})
	}
	// Dependencies resolve within one product/RC, so one probe version covers every expansion.
	if err := validateDependencies(probe); err != nil {
		return fmt.Errorf("matrix dependencies: %w", err)
	}

	return checkPhaseOrder(m.Jobs)
}

// checkPhaseOrder rejects a dependency on a job of a later phase: that phase already waits for
// this one, so the plan would deadlock.
func checkPhaseOrder(jobs []MatrixJob) error {
	phase := map[string]int{}
	for i := range jobs {
		phase[jobs[i].Product+"|"+jobs[i].Name] = jobs[i].Phase
	}
	for i := range jobs {
		j := &jobs[i]
		for _, d := range j.DependsOn {
			if dp := phase[j.Product+"|"+d]; dp > j.Phase {
				return fmt.Errorf("job %q (phase %d) depends on %q of the later phase %d", j.Name, j.Phase, d, dp)
			}
		}
	}

	return nil
}

// checkPrefix rejects duplicate codes and hostname prefixes that could exceed qainfra's budget.
func checkPrefix(m *Matrix, j *MatrixJob, codes map[string]string) error {
	if !jobCode.MatchString(j.Code) {
		return fmt.Errorf("job %q: code must be 1 or 2 lowercase letters or digits", j.Name)
	}
	if other, dup := codes[j.Product+"|"+j.Code]; dup {
		return fmt.Errorf("jobs %q and %q (%s) share code %q", other, j.Name, j.Product, j.Code)
	}
	codes[j.Product+"|"+j.Code] = j.Name

	tmpl, ok := j.Params["HOSTNAME_PREFIX"]
	if !ok {
		tmpl = m.Defaults["HOSTNAME_PREFIX"]
	}
	// Budget for a two-digit minor (every current one: v1.37 -> "137").
	longest := strings.ReplaceAll(tmpl, "{{PREFIX}}", m.PrefixBase+j.Product[:1]+"110"+j.Code)
	if budget := hostnameBudget - len(j.Product); len(longest) > budget {
		return fmt.Errorf("job %q: HOSTNAME_PREFIX %q can reach %d chars; qainfra allows %d for %s",
			j.Name, tmpl, len(longest), budget, j.Product)
	}

	return nil
}

// buildPlan expands the request against the matrix; jobs are ordered by priority, then version.
// res (optional) checks the jobs on Jenkins and resolves where upgrades start.
func buildPlan(ctx context.Context, req Request, m *Matrix, now time.Time, requestID string, res Resolver) (
	*Plan, error,
) {
	if req.Empty() {
		return nil, errors.New("no release candidate tags found in the message")
	}

	p := &Plan{Request: req, RequestID: requestID}
	p.Workflows = append(p.Workflows, WorkflowDispatch{
		Repo:     m.DTFRepo,
		Workflow: "release-checks.yaml",
		Ref:      m.DTFRef,
		Inputs: map[string]string{
			"k3s_versions":  strings.Join(req.K3s, ","),
			"rke2_versions": strings.Join(req.RKE2, ","),
		},
	})

	rcs, warn := qaseRCs(req)
	p.Warnings = append(p.Warnings, warn...)
	if len(rcs) > 0 {
		p.Workflows = append(p.Workflows, WorkflowDispatch{
			Repo:     m.DTFRepo,
			Workflow: "qase-patch-validation-create.yaml",
			Ref:      m.DTFRef,
			Inputs:   map[string]string{"rcs": strings.Join(rcs, ","), "request_id": requestID},
		})
		for _, product := range []string{"rke2", "k3s"} {
			for _, rc := range rcs {
				p.QaseTitles = append(p.QaseTitles, qaseRunTitle(product, rc, now))
			}
		}
	}

	x := &expander{
		ctx: ctx, m: m, res: res, plan: p,
		params: map[string]jobCheck{}, from: map[string]string{}, failed: map[string]bool{},
	}
	x.expandAll(req, now)
	// Check the expanded graph (phases included) before anything external happens.
	if err := validateDependencies(p.Jobs); err != nil {
		return nil, fmt.Errorf("plan dependencies: %w", err)
	}

	sort.SliceStable(p.Jobs, func(i, j int) bool {
		if p.Jobs[i].Priority != p.Jobs[j].Priority {
			return p.Jobs[i].Priority < p.Jobs[j].Priority
		}

		return newerTag(p.Jobs[i].Version, p.Jobs[j].Version)
	})

	return p, nil
}

// qaseRCs returns one "vX.Y.Z-rcN" per version; the Qase script creates runs for both products from each.
func qaseRCs(req Request) (rcs, warnings []string) {
	byVersion := map[string]string{}
	for _, tag := range append(append([]string{}, req.K3s...), req.RKE2...) {
		v, rc := baseVersion(tag), baseRC(tag)
		prev, ok := byVersion[v]
		if ok && prev != rc {
			warnings = append(warnings, fmt.Sprintf("Qase: %s has different RCs (%s, %s); using %s in the run description",
				v, prev, rc, maxRC(prev, rc)))
			rc = maxRC(prev, rc)
		}
		byVersion[v] = rc
	}

	for _, rc := range byVersion {
		rcs = append(rcs, rc)
	}
	// Oldest first by release (v1.36.9 before v1.36.10); base RCs carry no "+rke2rN", so one is
	// added only to compare them.
	sort.Slice(rcs, func(i, j int) bool { return newerTag(rcs[j]+"+rke2r1", rcs[i]+"+rke2r1") })

	return rcs, warnings
}

func maxRC(a, b string) string {
	if rcNumber(a) >= rcNumber(b) {
		return a
	}

	return b
}

func rcNumber(rc string) int {
	i := strings.LastIndex(rc, "-rc")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(rc[i+3:])

	return n
}

// splitShare returns, for each split group of the product, the one job name that runs tag:
// the product's RCs, newest first, are shared out in matrix order (4 RCs, 2 jobs: 2 each).
func splitShare(m *Matrix, product string, tags []string, tag string) map[string]string {
	groups := map[string][]string{}
	for i := range m.Jobs {
		if j := &m.Jobs[i]; j.Product == product && j.Split != "" {
			groups[j.Split] = append(groups[j.Split], j.Name)
		}
	}

	ordered := newestFirst(tags)
	idx := 0
	for i, t := range ordered {
		if t == tag {
			idx = i
		}
	}
	share := map[string]string{}
	for g, names := range groups {
		share[g] = names[idx*len(names)/len(ordered)]
	}

	return share
}

type jobCheck struct {
	params  []string
	exists  bool
	checked bool
}

// expander builds one RC's jobs, checking each Jenkins job once per plan.
type expander struct {
	ctx    context.Context
	m      *Matrix
	res    Resolver
	plan   *Plan
	params map[string]jobCheck // by controller|path
	from   map[string]string   // upgrade start by product|tag; "" = unknown
	// failed: sources (a controller, "github") that already failed a check are not asked again,
	// so an unreachable Jenkins costs one timeout per plan, not one per job.
	failed map[string]bool
}

// expandRC returns the jobs that run tag: split groups applied, jobs missing on Jenkins or
// missing a parameter left out, and each phase waiting for the previous one.
func (x *expander) expandRC(product, tag string, share map[string]string) []JenkinsJob {
	var jobs []JenkinsJob
	for i := range x.m.Jobs {
		mj := &x.m.Jobs[i]
		if mj.Product != product || (mj.Split != "" && share[mj.Split] != mj.Name) {
			continue
		}
		job := expandJob(mj, tag, x.m.PrefixBase, x.m.Defaults)
		if reason := x.fillUpgradeFrom(&job); reason != "" {
			x.block(&job, reason)
			continue
		}
		switch reason, absent := x.checkJob(&job); {
		case absent && mj.Optional:
			x.skip(&job, reason)
		case reason != "":
			x.block(&job, reason)
		default:
			jobs = append(jobs, job)
		}
	}

	return gatePhases(jobs, x.note)
}

// expandAll adds every product's RCs to the plan, newest RC first.
func (x *expander) expandAll(req Request, now time.Time) {
	for _, product := range []string{"rke2", "k3s"} {
		tags := req.K3s
		if product == "rke2" {
			tags = req.RKE2
		}
		for _, tag := range newestFirst(tags) {
			jobs := x.expandRC(product, tag, splitShare(x.m, product, tags, tag))
			for i := range jobs {
				jobs[i].QaseTitle = qaseRunTitle(product, tag, now)
			}
			x.plan.Jobs = append(x.plan.Jobs, jobs...)
		}
	}
	x.warnUnchecked()
}

func (x *expander) skip(j *JenkinsJob, reason string) {
	x.plan.Skipped = append(x.plan.Skipped, SkippedJob{Path: j.Path, Version: j.Version, Reason: reason})
}

func (x *expander) block(j *JenkinsJob, reason string) {
	x.plan.Blocked = append(x.plan.Blocked, SkippedJob{Path: j.Path, Version: j.Version, Reason: reason})
}

func (x *expander) fillUpgradeFrom(j *JenkinsJob) string {
	uses := false
	for _, v := range j.Params {
		uses = uses || strings.Contains(v, upgradeFromPlaceholder)
	}
	if !uses {
		return ""
	}

	key := j.Product + "|" + j.Version
	from, done := x.from[key]
	if !done {
		if x.res == nil || x.failed["github"] {
			from = ""
		} else if v, note, err := x.res.LatestGA(x.ctx, j.Product, j.Version); err != nil {
			x.failed["github"] = true
			x.plan.Warnings = append(x.plan.Warnings, "upgrade start for "+j.Version+": "+err.Error())
		} else {
			from = v
			if x.plan.UpgradeFrom == nil {
				x.plan.UpgradeFrom = map[string]string{}
			}
			x.plan.UpgradeFrom[j.Version] = v
			if note != "" {
				x.plan.Warnings = append(x.plan.Warnings, note)
			}
		}
		x.from[key] = from
	}
	if from == "" {
		return "upgrade start version unknown"
	}
	for k, v := range j.Params {
		j.Params[k] = strings.ReplaceAll(v, upgradeFromPlaceholder, from)
	}

	return ""
}

// checkJob returns why the job cannot run ("" to keep it) and whether the job is absent from the
// controller. An unanswered check keeps the job but marks the plan unchecked.
func (x *expander) checkJob(j *JenkinsJob) (reason string, absent bool) {
	if x.res == nil {
		return "", false
	}

	key := j.Controller + "|" + j.Path
	c, done := x.params[key]
	if !done && !x.failed[j.Controller] {
		params, exists, err := x.res.JobParams(x.ctx, j.Controller, j.Path)
		c = jobCheck{params: params, exists: exists, checked: err == nil}
		if err != nil {
			x.plan.checkErr = err
			x.failed[j.Controller] = true
		}
		x.params[key] = c
	}
	switch {
	case !c.checked:
		x.plan.Unchecked = append(x.plan.Unchecked, j.Path+" "+j.Version)
		return "", false
	case !c.exists:
		return "not on " + j.Controller + " yet", true
	}

	var missing, dropped []string
	for k := range j.Params {
		switch {
		case slices.Contains(c.params, k):
		case slices.Contains(x.m.OptionalParams, k):
			dropped = append(dropped, k)
		default:
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "job has no parameter " + strings.Join(missing, ", "), false
	}
	if len(dropped) > 0 {
		// The optional params go together (REPORT_TO_QASE without QASE_RUN_ID would report to the
		// wrong run): one missing drops them all.
		sort.Strings(dropped)
		all := slices.DeleteFunc(slices.Clone(x.m.OptionalParams), func(k string) bool { _, ok := j.Params[k]; return !ok })
		sort.Strings(all)
		for _, k := range all {
			delete(j.Params, k)
		}
		x.note(fmt.Sprintf("%s runs without %s (it does not define %s)", j.Path, strings.Join(all, ", "),
			strings.Join(dropped, ", ")))
	}

	return "", false
}

// note adds a warning once per plan, however many RCs hit it.
func (x *expander) note(w string) {
	if !slices.Contains(x.plan.Warnings, w) {
		x.plan.Warnings = append(x.plan.Warnings, w)
	}
}

func (x *expander) warnUnchecked() {
	if n := len(x.plan.Unchecked); n > 0 {
		x.plan.Warnings = append(x.plan.Warnings, fmt.Sprintf(
			"%d jobs not checked on Jenkins (existence and parameters), so the plan cannot run: %v",
			n, x.plan.checkErr))
	}
}

// gatePhases makes each job wait for every job of the previous phase that runs for the same RC;
// explicit dependencies on jobs that do not run for it are dropped, with a note.
func gatePhases(jobs []JenkinsJob, note func(string)) []JenkinsJob {
	byPhase := map[int][]string{}
	var phases []int
	for i := range jobs {
		if _, ok := byPhase[jobs[i].Phase]; !ok {
			phases = append(phases, jobs[i].Phase)
		}
		byPhase[jobs[i].Phase] = append(byPhase[jobs[i].Phase], jobs[i].Name)
	}
	sort.Ints(phases)

	present := map[string]bool{}
	for i := range jobs {
		present[jobs[i].Name] = true
	}
	for i := range jobs {
		j := &jobs[i]
		var deps []string
		for _, d := range j.DependsOn {
			if present[d] {
				deps = append(deps, d)
			} else {
				note(fmt.Sprintf("%s %s does not wait for %s, which does not run for that RC", j.Name, j.Version, d))
			}
		}
		if n := slices.Index(phases, j.Phase); n > 0 {
			for _, d := range byPhase[phases[n-1]] {
				if !slices.Contains(deps, d) {
					deps = append(deps, d)
				}
			}
		}
		j.DependsOn = deps
	}

	return jobs
}

// expandJob keeps HOSTNAME_PREFIX short: qainfra caps dsf-<prefix>-<product>-<id> at 24 chars.
// The prefix is <base><product letter><major><minor><code>, e.g. rbr137vc.
func expandJob(mj *MatrixJob, tag, prefixBase string, defaults map[string]string) JenkinsJob {
	r, _ := parseRelease(tag)
	prefix := fmt.Sprintf("%s%s%d%d%s", prefixBase, mj.Product[:1], r.major, r.minor, mj.Code)

	params := map[string]string{}
	for k, v := range defaults {
		params[k] = v
	}
	for k, v := range mj.Params {
		params[k] = v
	}
	for k, v := range params {
		v = strings.ReplaceAll(v, "{{VERSION}}", tag)
		params[k] = strings.ReplaceAll(v, "{{PREFIX}}", prefix)
	}

	return JenkinsJob{
		Name:       mj.Name,
		DependsOn:  append([]string{}, mj.DependsOn...),
		Controller: mj.Controller,
		Path:       mj.Path,
		Product:    mj.Product,
		Version:    tag,
		Phase:      mj.Phase,
		Priority:   mj.Priority,
		Params:     params,
	}
}
