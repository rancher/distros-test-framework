package qainfra

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/resources"
)

const (
	qaInfraRepoEnv     = "QA_INFRA_REPO"
	qaInfraRefEnv      = "QA_INFRA_REF"
	defaultQAInfraRepo = "github.com/rancher/qa-infra-automation"
	qaInfraRefDef      = "main"

	lsRemoteTimeout  = 60 * time.Second
	lsRemoteAttempts = 3
)

var (
	shaRE      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	repoPathRE = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+){2}$`)
)

// qaInfraSource pins rancher/qa-infra-automation (or a fork) to ONE commit so
// the Tofu module and the Ansible checkout can never run different revisions.
type qaInfraSource struct {
	// ModuleBase is the go-getter/tofu form: host/owner/repo (no scheme, no .git).
	ModuleBase string
	CloneURL   string
	Ref        string
	SHA        string
}

// moduleSource returns the tofu source address for a module path under tofu/<provider>/modules.
func (s qaInfraSource) moduleSource(provider, module string) string {
	return fmt.Sprintf("%s//tofu/%s/modules/%s?ref=%s", s.ModuleBase, provider, module, s.SHA)
}

// resolveQAInfraSource reads QA_INFRA_REPO/QA_INFRA_REF and resolves the ref to a
// commit SHA before any resource is created.
func resolveQAInfraSource(ctx context.Context) (qaInfraSource, error) {
	base, err := normalizeQAInfraRepo(os.Getenv(qaInfraRepoEnv))
	if err != nil {
		return qaInfraSource{}, err
	}

	ref := strings.TrimSpace(os.Getenv(qaInfraRefEnv))
	if ref == "" {
		ref = qaInfraRefDef
	}

	src := qaInfraSource{
		ModuleBase: base,
		CloneURL:   "https://" + base + ".git",
		Ref:        ref,
	}

	if shaRE.MatchString(ref) {
		src.SHA = ref
		resources.LogLevel("info", "qa-infra pinned to commit %s (%s)", ref, base)

		return src, nil
	}

	sha, err := lsRemoteSHA(ctx, src.CloneURL, ref)
	if err != nil {
		return qaInfraSource{}, err
	}
	src.SHA = sha
	resources.LogLevel("info", "qa-infra %s ref %q resolved to commit %s", base, ref, sha)

	return src, nil
}

// normalizeQAInfraRepo accepts owner/repo, host/owner/repo or an https URL and
// returns host/owner/repo. Empty input selects the upstream repository.
func normalizeQAInfraRepo(raw string) (string, error) {
	repo := strings.TrimSpace(raw)
	if repo == "" {
		return defaultQAInfraRepo, nil
	}

	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimPrefix(repo, "http://")
	repo = strings.TrimSuffix(repo, "/")
	repo = strings.TrimSuffix(repo, ".git")

	if strings.Count(repo, "/") == 1 {
		repo = "github.com/" + repo
	}
	if !repoPathRE.MatchString(repo) {
		return "", fmt.Errorf("%s=%q must look like owner/repo or host/owner/repo", qaInfraRepoEnv, raw)
	}

	return repo, nil
}

// lsRemoteSHA resolves a branch or tag to its commit via `git ls-remote`,
// preferring the peeled tag object so annotated tags yield the commit.
func lsRemoteSHA(ctx context.Context, cloneURL, ref string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= lsRemoteAttempts; attempt++ {
		out, err := runLsRemote(ctx, cloneURL, ref)
		if err == nil {
			sha, parseErr := parseLsRemote(out, ref)
			if parseErr == nil {
				return sha, nil
			}

			return "", parseErr
		}
		lastErr = err
		resources.LogLevel("warn", "git ls-remote %s (attempt %d/%d) failed: %v", ref, attempt, lsRemoteAttempts, err)
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}

	return "", fmt.Errorf("resolve qa-infra ref %q at %s: %w", ref, cloneURL, lastErr)
}

func runLsRemote(ctx context.Context, cloneURL, ref string) ([]byte, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, lsRemoteTimeout)
	defer cancel()

	// cloneURL is derived from the validated repo path; ref is only ever a ls-remote pattern.
	cmd := exec.CommandContext(cmdCtx, "git", "ls-remote", "--heads", "--tags", cloneURL, //nolint:gosec // validated inputs
		"refs/heads/"+ref, "refs/tags/"+ref, "refs/tags/"+ref+"^{}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// parseLsRemote picks refs/heads/<ref>, else the peeled refs/tags/<ref>^{}, else refs/tags/<ref>.
func parseLsRemote(out []byte, ref string) (string, error) {
	want := map[string]int{
		"refs/heads/" + ref:        0,
		"refs/tags/" + ref + "^{}": 1,
		"refs/tags/" + ref:         2,
	}
	best, bestRank := "", len(want)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !shaRE.MatchString(fields[0]) {
			continue
		}
		if rank, ok := want[fields[1]]; ok && rank < bestRank {
			best, bestRank = fields[0], rank
		}
	}
	if best == "" {
		return "", fmt.Errorf("qa-infra ref %q not found as a branch or tag", ref)
	}

	return best, nil
}
