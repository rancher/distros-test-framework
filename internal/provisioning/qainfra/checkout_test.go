package qainfra

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	//nolint:gosec // G204: test helper, args are literals from this file and a temp dir
	cmd := exec.Command("git", append([]string{
		"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false",
	}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func TestCheckoutQAInfraIncludesRootRequirementsAndIsRetrySafe(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	for path, body := range map[string]string{
		"ansible/rke2/site.yml": "x", "ansible/roles/r/main.yml": "x", "requirements.yml": "collections: []",
		"unrelated/file.txt": "x",
	} {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-q", "-m", "init")
	src := qaInfraSource{CloneURL: "file://" + repo, SHA: gitIn(t, repo, "rev-parse", "HEAD")}

	dir := t.TempDir()
	for attempt := 1; attempt <= 2; attempt++ {
		if err := checkoutQAInfra(dir, src, "rke2"); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "requirements.yml")); err != nil {
			t.Fatalf("attempt %d: root requirements.yml missing: %v", attempt, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "ansible", "rke2", "site.yml")); err != nil {
		t.Errorf("ansible/rke2 missing: %v", err)
	}
}
