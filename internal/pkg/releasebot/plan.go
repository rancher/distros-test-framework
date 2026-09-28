package releasebot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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
	// Name is the matrix job name; DependsOn names jobs of the same product and version.
	Name       string
	DependsOn  []string
	Controller string
	Path       string
	Product    string
	Version    string
	Priority   int
	Params     map[string]string
	// QaseTitle is the patch-validation run this job reports to (see QaseRunTitle).
	QaseTitle string
}

// qaseRunPlaceholder is replaced by the run id once the Qase workflow has created the runs.
const qaseRunPlaceholder = "{{QASE_RUN_ID}}"

// Plan is everything a release request would do, in order.
type Plan struct {
	Request Request
	// RequestID ties the Qase runs to this dispatch (see MatchQaseRun).
	RequestID string
	Workflows []WorkflowDispatch
	// QaseTitles are the runs the Qase workflow creates: both products for every version.
	QaseTitles []string
	Jobs       []JenkinsJob
	Warnings   []string
}

// Matrix is the versioned job list (config/releasebot/matrix.yaml).
type Matrix struct {
	DTFRepo    string            `yaml:"dtfRepo"`
	DTFRef     string            `yaml:"dtfRef"`
	PrefixBase string            `yaml:"prefixBase"`
	Controller map[string]Limits `yaml:"controllers"`
	Jobs       []MatrixJob       `yaml:"jobs"`
	Defaults   map[string]string `yaml:"defaultParams"`
}

// Limits bounds how many builds may run at once on a controller.
type Limits struct {
	URL           string `yaml:"url"`
	MaxConcurrent int    `yaml:"maxConcurrent"`
}

// MatrixJob is a job template; {{VERSION}} and {{PREFIX}} are replaced per RC,
// {{QASE_RUN_ID}} once the Qase runs exist.
type MatrixJob struct {
	Name       string            `yaml:"name"`
	Product    string            `yaml:"product"`
	Controller string            `yaml:"controller"`
	Path       string            `yaml:"path"`
	Priority   int               `yaml:"priority"`
	Params     map[string]string `yaml:"params"`
	// DependsOn lists job names of the same product; they must succeed for the same RC first.
	DependsOn []string `yaml:"dependsOn"`
}

// LoadMatrix reads and validates the matrix file.
func LoadMatrix(path string) (*Matrix, error) {
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

	seen := map[string]bool{}
	probe := make([]JenkinsJob, 0, len(m.Jobs))
	for _, j := range m.Jobs {
		if j.Product != "k3s" && j.Product != "rke2" {
			return nil, fmt.Errorf("job %q: product must be k3s or rke2", j.Name)
		}
		lim, ok := m.Controller[j.Controller]
		if !ok || lim.URL == "" || lim.MaxConcurrent < 1 {
			return nil, fmt.Errorf("job %q: controller %q missing url or maxConcurrent", j.Name, j.Controller)
		}
		if j.Name == "" || seen[j.Product+"|"+j.Name] {
			return nil, fmt.Errorf("job %q (%s): names must be set and unique per product", j.Name, j.Product)
		}
		seen[j.Product+"|"+j.Name] = true
		probe = append(probe, JenkinsJob{Name: j.Name, Product: j.Product, DependsOn: j.DependsOn})
	}
	// Dependencies resolve within one product/RC, so one probe version covers every expansion.
	if err = ValidateDependencies(probe); err != nil {
		return nil, fmt.Errorf("matrix dependencies: %w", err)
	}

	if m.DTFRepo == "" {
		m.DTFRepo = "rancher/distros-test-framework"
	}
	if m.DTFRef == "" {
		m.DTFRef = "main"
	}
	if m.PrefixBase == "" {
		m.PrefixBase = "rb"
	}

	return &m, nil
}

// BuildPlan expands the request against the matrix; jobs are ordered by priority, then version.
func BuildPlan(req Request, m *Matrix, now time.Time, requestID string) (*Plan, error) {
	if req.Empty() {
		return nil, errors.New("no release candidate tags found in the message")
	}

	p := &Plan{Request: req, RequestID: requestID}
	p.Workflows = append(p.Workflows, WorkflowDispatch{
		Repo:     m.DTFRepo,
		Workflow: "release-checks.yaml",
		Ref:      m.DTFRef,
		Inputs: map[string]string{
			"k3s_versions":      strings.Join(req.K3s, ","),
			"rke2_versions":     strings.Join(req.RKE2, ","),
			"rke2_lts_versions": strings.Join(req.RKE2LTS, ","),
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
				p.QaseTitles = append(p.QaseTitles, QaseRunTitle(product, rc, now))
			}
		}
	}

	for i := range m.Jobs {
		tags := req.K3s
		if m.Jobs[i].Product == "rke2" {
			tags = req.RKE2
		}
		for _, tag := range tags {
			job := expandJob(&m.Jobs[i], tag, m.PrefixBase, m.Defaults)
			job.QaseTitle = QaseRunTitle(job.Product, tag, now)
			p.Jobs = append(p.Jobs, job)
		}
	}

	sort.SliceStable(p.Jobs, func(i, j int) bool {
		if p.Jobs[i].Priority != p.Jobs[j].Priority {
			return p.Jobs[i].Priority < p.Jobs[j].Priority
		}

		return p.Jobs[i].Version > p.Jobs[j].Version
	})

	return p, nil
}

// qaseRCs returns one "vX.Y.Z-rcN" per version; the Qase script creates runs for both products from each.
func qaseRCs(req Request) (rcs, warnings []string) {
	byVersion := map[string]string{}
	for _, tag := range append(append([]string{}, req.K3s...), req.RKE2...) {
		v, rc := BaseVersion(tag), BaseRC(tag)
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
	sort.Strings(rcs)

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

// expandJob keeps HOSTNAME_PREFIX short: qainfra caps dsf-<prefix>-<product>-<id> at 24 chars.
func expandJob(mj *MatrixJob, tag, prefixBase string, defaults map[string]string) JenkinsJob {
	minor := strings.ReplaceAll(strings.TrimPrefix(BaseVersion(tag), "v"), ".", "")
	prefix := fmt.Sprintf("%s%s%s", prefixBase, mj.Product[:1], minor)

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
		DependsOn:  mj.DependsOn,
		Controller: mj.Controller,
		Path:       mj.Path,
		Product:    mj.Product,
		Version:    tag,
		Priority:   mj.Priority,
		Params:     params,
	}
}
