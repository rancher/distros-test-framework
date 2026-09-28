package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
	"github.com/rancher/distros-test-framework/internal/resources"
)

type options struct {
	message       string
	messageFile   string
	matrixPath    string
	dryRun        bool
	skipTagCheck  bool
	skipWorkflows bool
	skipJobs      bool
	poll          time.Duration
	qaseTimeout   time.Duration
}

func main() {
	var o options
	flag.StringVar(&o.message, "message", "", "release request text (as posted in Slack)")
	flag.StringVar(&o.messageFile, "message-file", "", "read the release request text from a file")
	flag.StringVar(&o.matrixPath, "matrix", "config/releasebot/matrix.yaml", "job matrix file")
	flag.BoolVar(&o.dryRun, "dry-run", true, "print the plan without dispatching or triggering anything")
	flag.BoolVar(&o.skipTagCheck, "skip-tag-check", false, "do not verify that the RC tags exist on GitHub")
	flag.BoolVar(&o.skipWorkflows, "skip-workflows", false, "do not dispatch the GitHub workflows")
	flag.BoolVar(&o.skipJobs, "skip-jobs", false, "do not trigger the Jenkins jobs")
	flag.DurationVar(&o.poll, "poll", 60*time.Second, "Jenkins polling interval")
	flag.DurationVar(&o.qaseTimeout, "qase-timeout", 15*time.Minute, "how long to wait for the Qase runs to appear")
	flag.Parse()

	if err := run(&o); err != nil {
		resources.LogLevel("error", "releasebot: %v", err)
		os.Exit(1)
	}
}

func run(o *options) error {
	if o.messageFile != "" {
		raw, err := os.ReadFile(o.messageFile)
		if err != nil {
			return err
		}
		o.message = string(raw)
	}

	matrix, err := releasebot.LoadMatrix(o.matrixPath)
	if err != nil {
		return err
	}

	plan, err := releasebot.BuildPlan(releasebot.ParseRequest(o.message), matrix, time.Now(),
		releasebot.NewRequestID(time.Now()))
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gh := releasebot.NewGitHub(os.Getenv("GITHUB_TOKEN"))
	if !o.skipTagCheck {
		if tagErr := checkTags(ctx, gh, plan.Request); tagErr != nil {
			return tagErr
		}
	}

	printPlan(plan, o.dryRun)
	if o.dryRun {
		return nil
	}

	return execute(ctx, o, gh, matrix, plan)
}

func execute(
	ctx context.Context,
	o *options,
	gh *releasebot.GitHub,
	matrix *releasebot.Matrix,
	plan *releasebot.Plan,
) error {
	// Check the Qase client before dispatching anything, so a missing token fails early.
	var qase *releasebot.Qase
	needQase := !o.skipJobs && jobsNeedQaseRun(plan.Jobs)
	if needQase {
		var qErr error
		if qase, qErr = releasebot.NewQase(); qErr != nil {
			return fmt.Errorf("jobs use {{QASE_RUN_ID}}: %w", qErr)
		}
	}

	if !o.skipWorkflows {
		for _, w := range plan.Workflows {
			if dErr := gh.Dispatch(ctx, w); dErr != nil {
				return dErr
			}
			resources.LogLevel("info", "dispatched %s on %s@%s", w.Workflow, w.Repo, w.Ref)
		}
	}

	if o.skipJobs || len(plan.Jobs) == 0 {
		return nil
	}

	if needQase {
		// Runs created by this dispatch carry the request id; with -skip-workflows (runs created
		// by hand) there is none, and the newest run with each title is used.
		requestID := plan.RequestID
		if o.skipWorkflows {
			requestID = ""
		}
		if qErr := fillQaseRunIDs(ctx, qase, plan, requestID, o.qaseTimeout); qErr != nil {
			return qErr
		}
	}

	return runJobs(ctx, matrix, plan.Jobs, o.poll)
}

func fillQaseRunIDs(
	ctx context.Context,
	qase *releasebot.Qase,
	plan *releasebot.Plan,
	requestID string,
	timeout time.Duration,
) error {
	resources.LogLevel("info", "waiting for the Qase runs of request %q", requestID)
	ids, err := releasebot.WaitQaseRuns(ctx, qase, plan.QaseTitles, requestID, 20*time.Second, timeout)
	if err != nil {
		return err
	}
	for _, t := range plan.QaseTitles {
		resources.LogLevel("info", "Qase run %d: %s", ids[t], t)
	}

	return releasebot.ApplyQaseRunIDs(plan.Jobs, ids)
}

func jobsNeedQaseRun(jobs []releasebot.JenkinsJob) bool {
	for i := range jobs {
		for _, v := range jobs[i].Params {
			if strings.Contains(v, "{{QASE_RUN_ID}}") {
				return true
			}
		}
	}

	return false
}

func runJobs(
	ctx context.Context,
	matrix *releasebot.Matrix,
	jobs []releasebot.JenkinsJob,
	poll time.Duration,
) error {
	builders := map[string]releasebot.Builder{}
	for name, lim := range matrix.Controller {
		user, token, ok := jenkinsAuth(name)
		if !ok {
			return fmt.Errorf("set JENKINS_%s_AUTH=user:apitoken", strings.ToUpper(name))
		}
		builders[name] = releasebot.NewJenkins(lim.URL, user, token)
	}

	s := &releasebot.Scheduler{
		Builders: builders,
		Limits:   matrix.Controller,
		Poll:     poll,
		Notify:   resources.LogLevel,
	}

	outcomes := s.Run(ctx, jobs)
	failed := 0
	resources.LogLevel("info", "Summary:")
	for i := range outcomes {
		o := &outcomes[i]
		status := o.Result
		if o.Err != nil {
			status = "ERROR: " + o.Err.Error()
		}
		if status != "SUCCESS" {
			failed++
		}
		level := "info"
		if status != resultSuccess {
			level = "warn"
		}
		resources.LogLevel(level, "  %-8s %-55s %s %s", status, o.Job.Path, o.Job.Version, o.BuildURL)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d jobs did not succeed", failed, len(jobs))
	}

	return nil
}

func checkTags(ctx context.Context, gh *releasebot.GitHub, req releasebot.Request) error {
	var missing []string
	check := func(product string, tags []string) error {
		for _, t := range tags {
			ok, err := gh.TagExists(ctx, product, t)
			if err != nil {
				return err
			}
			if !ok {
				missing = append(missing, t)
			}
		}

		return nil
	}
	if err := check("k3s", req.K3s); err != nil {
		return err
	}
	if err := check("rke2", append(append([]string{}, req.RKE2...), req.RKE2LTS...)); err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("tags not found on GitHub: %s", strings.Join(missing, ", "))
	}

	return nil
}

// jenkinsAuth reads JENKINS_<CONTROLLER>_AUTH as "user:apitoken".
func jenkinsAuth(controller string) (user, token string, ok bool) {
	return strings.Cut(os.Getenv("JENKINS_"+strings.ToUpper(controller)+"_AUTH"), ":")
}

func printPlan(p *releasebot.Plan, dryRun bool) {
	mode := "EXECUTE"
	if dryRun {
		mode = "DRY-RUN"
	}
	resources.LogLevel("info", "Release plan (%s), request %s", mode, p.RequestID)
	resources.LogLevel("info", "  k3s: %s | rke2: %s | rke2 LTS: %s",
		list(p.Request.K3s), list(p.Request.RKE2), list(p.Request.RKE2LTS))

	resources.LogLevel("info", "GitHub workflows (%d):", len(p.Workflows))
	for _, w := range p.Workflows {
		var inputs []string
		for k, v := range w.Inputs {
			if v != "" {
				inputs = append(inputs, k+"="+v)
			}
		}
		sort.Strings(inputs)
		resources.LogLevel("info", "  %s@%s %s %s", w.Repo, w.Ref, w.Workflow, strings.Join(inputs, " "))
	}

	if len(p.QaseTitles) > 0 {
		resources.LogLevel("info", "Qase runs the workflow creates (ids fill {{QASE_RUN_ID}} once they exist):")
		for _, t := range p.QaseTitles {
			resources.LogLevel("info", "  %s", t)
		}
	}

	resources.LogLevel("info", "Jenkins jobs (%d):", len(p.Jobs))
	for i := range p.Jobs {
		j := &p.Jobs[i]
		after := ""
		if len(j.DependsOn) > 0 {
			after = " after " + strings.Join(j.DependsOn, ",")
		}
		resources.LogLevel("info", "  P%d %-6s %-55s %s prefix=%s%s",
			j.Priority, j.Controller, j.Path, j.Version, j.Params["HOSTNAME_PREFIX"], after)

		params := make([]string, 0, len(j.Params))
		for k, v := range j.Params {
			params = append(params, k+"="+v)
		}
		sort.Strings(params)
		resources.LogLevel("debug", "      params: %s", strings.Join(params, " "))
	}

	for _, w := range p.Warnings {
		resources.LogLevel("warn", "%s", w)
	}
}

const resultSuccess = "SUCCESS"

func list(v []string) string {
	if len(v) == 0 {
		return "-"
	}

	return strings.Join(v, " ")
}
