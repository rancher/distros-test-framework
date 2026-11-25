package releasebot

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// bumpProjects are the boards whose "To Test" column the bump mode reads.
var bumpProjects = []struct {
	product, org string
	number       int
}{{"k3s", "k3s-io", 6}, {"rke2", "rancher", 31}}

// bumpSource is what planning needs from GitHub (gitHub; a fake in tests).
type bumpSource interface {
	ToTestIssues(ctx context.Context, org string, number int) ([]projectIssue, error)
	MergedPRs(ctx context.Context, repo string, number int) ([]BumpPR, error)
	ClosingIssue(ctx context.Context, repo string, pull int) (int, error)
	PRFiles(ctx context.Context, repo string, number int) ([]PRFile, error)
	TagContains(ctx context.Context, repo, tag, commit string) (bool, error)
	BranchHead(ctx context.Context, repo, branch string) (string, error)
	matchingTags(ctx context.Context, product, prefix string) ([]string, error)
}

// BumpProblem is an issue the bot does not validate, and why.
type BumpProblem struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// BumpPlan is what a bump run validates and reports.
type BumpPlan struct {
	Month   string        `json:"month"`
	Groups  []*BumpGroup  `json:"groups"`
	Targets []*BumpTarget `json:"targets"`

	// Person: bumps the bot cannot validate (not only a version change, no check for a file, no PR);
	// NotBumps: other "To Test" issues, listed so nothing is silently dropped.
	Person   []BumpProblem `json:"person,omitempty"`
	NotBumps []BumpProblem `json:"not_bumps,omitempty"`
}

// PlanBumps reads both boards and plans one version-bump job per product and release branch.
func PlanBumps(ctx context.Context, src bumpSource, cfg *BumpConfig, month string) (*BumpPlan, error) {
	plan := &BumpPlan{Month: month}
	var bumps []*BumpIssue
	for _, p := range bumpProjects {
		issues, listErr := src.ToTestIssues(ctx, p.org, p.number)
		if listErr != nil {
			return nil, listErr
		}

		for _, pi := range issues {
			issue, problem, checkErr := classifyBump(ctx, src, cfg, p.product, &pi)
			switch {
			case checkErr != nil:
				return nil, checkErr
			case problem != nil && issue == nil:
				plan.NotBumps = append(plan.NotBumps, *problem)
			case problem != nil:
				plan.Person = append(plan.Person, *problem)
				bumps = append(bumps, issue)
			default:
				bumps = append(bumps, issue)
			}
		}
	}

	for _, g := range GroupBumps(bumps) {
		switch {
		case g.Main.Number == 0:
			for _, bp := range g.Backports {
				plan.Person = append(plan.Person, BumpProblem{Repo: bp.Repo, Number: bp.Number, Title: bp.Title,
					Reason: "no main issue: its body names none and no To Test issue has its title"})
			}
			continue
		case len(g.Backports) == 0:
			plan.Person = append(plan.Person, BumpProblem{Repo: g.Main.Repo, Number: g.Main.Number, Title: g.Main.Title,
				Reason: "no backport in To Test: master is never validated"})
		}
		plan.Groups = append(plan.Groups, g)
	}
	plan.Person = append(plan.Person, conflictingBumps(bumps, plan.Person)...)

	targets, targetErr := bumpTargets(ctx, src, bumps, plan.Person)
	if targetErr != nil {
		return nil, targetErr
	}
	plan.Targets = targets

	return plan, nil
}

// classifyBump returns the bump (with its PRs and checks), or a problem: a non-bump (no issue) or a
// bump a person has to validate (issue and problem).
func classifyBump(
	ctx context.Context, src bumpSource, cfg *BumpConfig, product string, pi *projectIssue,
) (*BumpIssue, *BumpProblem, error) {
	problem := func(reason string) *BumpProblem {
		return &BumpProblem{Repo: pi.Repo, Number: pi.Number, Title: pi.Title, Reason: reason}
	}
	if pi.Repo != productRepo[product] || !IsBumpTitle(pi.Title) {
		return nil, problem("not a bump"), nil
	}

	issue := &BumpIssue{Product: product, Repo: pi.Repo, Number: pi.Number, Title: pi.Title, URL: pi.URL,
		Branch: BranchOf(pi.Title)}
	if issue.Branch == "" {
		return issue, nil, nil // the main issue: reported with its backports, never validated itself
	}
	parent, pull := ParentOf(pi.Body)
	if parent == 0 && pull > 0 {
		closed, closingErr := src.ClosingIssue(ctx, pi.Repo, pull)
		if closingErr != nil {
			return nil, nil, closingErr
		}
		parent = closed
	}
	issue.Parent = parent

	prs, prsErr := src.MergedPRs(ctx, pi.Repo, pi.Number)
	if prsErr != nil {
		return nil, nil, prsErr
	}
	for _, pr := range prs {
		if pr.Base == issue.Branch {
			issue.PRs = append(issue.PRs, pr)
		}
	}
	if len(issue.PRs) == 0 {
		return issue, problem("no merged pull request into " + issue.Branch), nil
	}

	reason, checksErr := addBumpChecks(ctx, src, cfg, issue)
	if checksErr != nil {
		return nil, nil, checksErr
	}
	if reason != "" {
		return issue, problem(reason), nil
	}
	if len(issue.Checks) == 0 {
		return issue, problem("its pull requests change nothing a job can check (a non-default image?)"), nil
	}

	return issue, nil, nil
}

// addBumpChecks reads the files of each of the issue's PRs and adds their checks to the issue, or
// returns why a person has to validate it instead.
func addBumpChecks(ctx context.Context, src bumpSource, cfg *BumpConfig, issue *BumpIssue) (string, error) {
	for i := range issue.PRs {
		files, filesErr := src.PRFiles(ctx, issue.Repo, issue.PRs[i].Number)
		if filesErr != nil {
			return "", filesErr
		}
		issue.PRs[i].Files = files

		if pure, why := PureBump(files); !pure {
			return fmt.Sprintf("#%d is not only a version bump: %s", issue.PRs[i].Number, why), nil
		}

		checks, unsupported := BumpChecks(issue.Product, files, cfg.DefaultImages[issue.Product])
		if len(unsupported) > 0 {
			return fmt.Sprintf("#%d: no check yet for %s", issue.PRs[i].Number, strings.Join(unsupported, ", ")), nil
		}
		for _, c := range checks {
			c.Issue = issue.Number
			issue.Checks = append(issue.Checks, c)
		}
	}

	return "", nil
}

func personSet(person []BumpProblem) map[string]bool {
	skip := map[string]bool{}
	for _, p := range person {
		skip[fmt.Sprintf("%s#%d", p.Repo, p.Number)] = true
	}

	return skip
}

// conflictingBumps: checks of one branch that want the same chart or image at different versions (two
// issues, or two PRs of one) cannot all pass and would fail the branch's job, so a person takes them.
func conflictingBumps(bumps []*BumpIssue, person []BumpProblem) []BumpProblem {
	skip := personSet(person)
	versions := map[string][]string{}
	issues := map[string][]*BumpIssue{}
	var keys []string
	for _, b := range bumps {
		if b.Branch == "" || skip[fmt.Sprintf("%s#%d", b.Repo, b.Number)] {
			continue
		}

		for _, c := range b.Checks {
			f := strings.Fields(c.What) // "<component> <chart|image> <version>"
			k := b.Product + " " + b.Branch + ": " + strings.Join(f[:min(2, len(f))], " ")
			if versions[k] == nil {
				keys = append(keys, k)
			}
			if !slices.Contains(versions[k], c.Expected) {
				versions[k] = append(versions[k], c.Expected)
			}
			if !slices.Contains(issues[k], b) {
				issues[k] = append(issues[k], b)
			}
		}
	}

	var problems []BumpProblem
	for _, k := range keys {
		if len(versions[k]) < 2 {
			continue
		}
		for _, b := range issues[k] {
			if skip[fmt.Sprintf("%s#%d", b.Repo, b.Number)] {
				continue // listed for another conflict already
			}
			skip[fmt.Sprintf("%s#%d", b.Repo, b.Number)] = true
			problems = append(problems, BumpProblem{Repo: b.Repo, Number: b.Number, Title: b.Title,
				Reason: fmt.Sprintf("%s is bumped to %d versions (%s): one job cannot check them all",
					k, len(versions[k]), strings.Join(versions[k], ", "))})
		}
	}

	return problems
}

// bumpTargets groups the valid backports by product and branch, with the version to install: the
// branch's newest RC when it holds every merge, otherwise the branch's latest commit.
func bumpTargets(ctx context.Context, src bumpSource, bumps []*BumpIssue, person []BumpProblem) ([]*BumpTarget, error) {
	skip := personSet(person)

	byBranch := map[string]*BumpTarget{}
	commits := map[string][]string{}
	var keys []string
	for _, b := range bumps {
		if b.Branch == "" || skip[fmt.Sprintf("%s#%d", b.Repo, b.Number)] {
			continue
		}

		k := b.Product + "|" + b.Branch
		t := byBranch[k]
		if t == nil {
			t = &BumpTarget{Product: b.Product, Branch: b.Branch}
			byBranch[k] = t
			keys = append(keys, k)
		}
		t.Checks = append(t.Checks, b.Checks...)
		for _, pr := range b.PRs {
			commits[k] = append(commits[k], pr.MergeCommit)
		}
	}
	sort.Strings(keys)

	targets := make([]*BumpTarget, 0, len(keys))
	for _, k := range keys {
		t := byBranch[k]
		version, isCommit, versionErr := bumpVersion(ctx, src, t.Product, t.Branch, commits[k])
		if versionErr != nil {
			return nil, versionErr
		}
		t.Version, t.Commit = version, isCommit
		targets = append(targets, t)
	}

	return targets, nil
}

// bumpVersion: the newest RC of the branch's minor if it holds every commit, else the branch head.
func bumpVersion(
	ctx context.Context, src bumpSource, product, branch string, commits []string,
) (version string, commit bool, err error) {
	repo := productRepo[product]
	minor := strings.TrimPrefix(branch, "release-")
	tags, tagsErr := src.matchingTags(ctx, product, "v"+minor+".")
	if tagsErr != nil {
		return "", false, tagsErr
	}

	var rcs []string
	for _, tag := range tags {
		if strings.Contains(tag, "-rc") {
			rcs = append(rcs, tag)
		}
	}
	if newest := newestFirst(rcs); len(newest) > 0 {
		holdsAll := true
		for _, c := range commits {
			holds, containsErr := src.TagContains(ctx, repo, newest[0], c)
			if containsErr != nil {
				return "", false, containsErr
			}
			holdsAll = holdsAll && holds
		}
		if holdsAll {
			return newest[0], false, nil
		}
	}

	head, headErr := src.BranchHead(ctx, repo, branch)
	if headErr != nil {
		return "", false, headErr
	}

	return head, true, nil
}

// PlanBumps reads the boards with the app's GitHub token (it needs read:project).
func (a *App) PlanBumps(ctx context.Context, cfg *BumpConfig, month string) (*BumpPlan, error) {
	return PlanBumps(ctx, a.cfg.NewGitHub(a.cfg.GitHubToken), cfg, month)
}

// PrintBumpPlan logs what the run validates, with each branch's job parameters, and what it leaves out.
func (a *App) PrintBumpPlan(plan *BumpPlan, cfg *BumpConfig, dtfRef, mode string) {
	log := a.cfg.Log
	log("info", "Bump plan for %s (%s): %d jobs, %d bumps for a person, %d other To Test issues",
		plan.Month, mode, len(plan.Targets), len(plan.Person), len(plan.NotBumps))

	for _, t := range plan.Targets {
		what := "RC"
		if t.Commit {
			what = "latest commit: no RC holds every merge yet, so validate again on the RC"
		}
		log("info", "  %s %s on %s (%s)", t.Product, t.Branch, t.Version, what)
		for _, c := range t.Checks {
			log("info", "    #%d %s: %s -> %s", c.Issue, c.What, c.Cmd, c.Expected)
		}

		job, jobErr := t.Job(cfg, cfg.Controller, dtfRef)
		if jobErr != nil {
			log("warn", "    WARNING: %v", jobErr)
			continue
		}
		log("debug", "    %s:%s params: %s", job.Controller, job.Path, sortedParams(job.Params))
	}

	if len(plan.Person) > 0 {
		log("info", "Needs a person (%d):", len(plan.Person))
		for _, p := range plan.Person {
			log("info", "  %s#%d %s: %s", p.Repo, p.Number, p.Title, p.Reason)
		}
	}
	if len(plan.NotBumps) > 0 {
		log("info", "Other To Test issues, not bumps (%d):", len(plan.NotBumps))
		for _, p := range plan.NotBumps {
			log("info", "  %s#%d %s", p.Repo, p.Number, p.Title)
		}
	}
}
