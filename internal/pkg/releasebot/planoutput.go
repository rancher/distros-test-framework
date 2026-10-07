package releasebot

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

type planLine struct {
	level string
	text  string
}

// planLines renders a plan once, for the log (PrintPlan) and for Slack (planSummary); mode
// ("DRY-RUN", "EXECUTE") labels the header when set.
func planLines(p *Plan, mode string) []planLine {
	var out []planLine
	add := func(level, format string, args ...any) {
		out = append(out, planLine{level, fmt.Sprintf(format, args...)})
	}

	if mode != "" {
		add("info", "Release plan (%s), request %s", mode, p.RequestID)
	} else {
		add("info", "Release plan, request %s", p.RequestID)
	}
	add("info", "  k3s: %s | rke2: %s", list(p.Request.K3s), list(p.Request.RKE2))

	add("info", "gitHub workflows (%d):", len(p.Workflows))
	for _, w := range p.Workflows {
		var inputs []string
		for k, v := range w.Inputs {
			if v != "" {
				inputs = append(inputs, k+"="+v)
			}
		}
		sort.Strings(inputs)
		add("info", "  %s@%s %s %s", w.Repo, w.Ref, w.Workflow, strings.Join(inputs, " "))
	}

	if len(p.QaseTitles) > 0 {
		add("info", "Qase runs the workflow creates (ids fill {{QASE_RUN_ID}} once they exist):")
		for _, t := range p.QaseTitles {
			add("info", "  %s", t)
		}
	}

	jobLines(p, add)

	for _, w := range p.Warnings {
		add("warn", "WARNING: %s", w)
	}

	return out
}

// jobLines lists the jobs by phase and RC (the log also gets each job's path and parameters),
// then the upgrade starting versions and the skipped jobs grouped by job.
func jobLines(p *Plan, add func(level, format string, args ...any)) {
	add("info", "jenkins jobs (%d); each RC starts a phase once all its jobs of the previous phase passed:",
		len(p.Jobs))
	byPhase := map[int]map[string][]string{}
	var phases []int
	for i := range p.Jobs {
		j := &p.Jobs[i]
		if byPhase[j.Phase] == nil {
			byPhase[j.Phase] = map[string][]string{}
			phases = append(phases, j.Phase)
		}
		rc := j.Product + " " + j.Version
		byPhase[j.Phase][rc] = append(byPhase[j.Phase][rc], j.Name)
		add("debug", "  phase %d %s %s prefix=%s params: %s", j.Phase, j.Path, j.Version,
			j.Params["HOSTNAME_PREFIX"], sortedParams(j.Params))
	}

	sort.Ints(phases)
	for _, ph := range phases {
		add("info", "  Phase %d:", ph)
		rcs := make([]string, 0, len(byPhase[ph]))
		for rc := range byPhase[ph] {
			rcs = append(rcs, rc)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(rcs)))
		for _, rc := range rcs {
			names := byPhase[ph][rc]
			sort.Strings(names)
			add("info", "    %s: %s", rc, strings.Join(names, ", "))
		}
	}

	if len(p.UpgradeFrom) > 0 {
		add("info", "Upgrades start from:")
		for _, rc := range slices.Sorted(maps.Keys(p.UpgradeFrom)) {
			add("info", "  %s <- %s", rc, p.UpgradeFrom[rc])
		}
	}

	skippedLines(add, "Cannot run, so the plan will not start (%d):", p.Blocked)
	skippedLines(add, "Skipped, optional jobs not on jenkins yet (%d):", p.Skipped)
}

// skippedLines groups jobs by path and reason, listing the RCs each applies to.
func skippedLines(add func(level, format string, args ...any), title string, jobs []SkippedJob) {
	if len(jobs) == 0 {
		return
	}
	add("info", title, len(jobs))
	rcs := map[string][]string{}
	var keys []string
	for _, s := range jobs {
		k := s.Path + ": " + s.Reason
		if rcs[k] == nil {
			keys = append(keys, k)
		}
		rcs[k] = append(rcs[k], s.Version)
	}
	for _, k := range keys {
		add("info", "  %s (%s)", k, strings.Join(rcs[k], " "))
	}
}

func sortedParams(params map[string]string) string {
	out := make([]string, 0, len(params))
	for k, v := range params {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)

	return strings.Join(out, " ")
}

func (a *App) PrintPlan(p *Plan, mode string) {
	for _, l := range planLines(p, mode) {
		a.cfg.Log(l.level, "%s", l.text)
	}
}

func planSummary(p *Plan) string {
	var b strings.Builder
	b.WriteString("```\n")

	// No mode: the message posted with it says whether the plan started or is dry-run only.
	for _, l := range planLines(p, "") {
		if l.level != "debug" {
			b.WriteString(l.text + "\n")
		}
	}
	b.WriteString("```")

	return b.String()
}

func list(v []string) string {
	if len(v) == 0 {
		return "-"
	}

	return strings.Join(v, " ")
}
