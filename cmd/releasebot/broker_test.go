package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -triage-build refuses a Baler build, quick or full, before reading its log or starting the agent,
// whose tools and prompt are jenkins-mower's; a Mower build passes on to the next check.
func TestTriageBuildRefusesOtherControllers(t *testing.T) {
	dir := t.TempDir()
	matrix := filepath.Join(dir, "matrix.yaml")
	body := "controllers:\n  mower: {url: https://mower.example, maxConcurrent: 1}\n" +
		"  baler: {url: https://baler.example, maxConcurrent: 1}\njobs: []\n"
	if err := os.WriteFile(matrix, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// Credentials for both: the refusal is about the agent's tools, not a missing token.
	t.Setenv("JENKINS_MOWER_AUTH", "u:t")
	t.Setenv("JENKINS_BALER_AUTH", "u:t")

	for _, full := range []bool{false, true} {
		o := &options{
			matrixPath: matrix, triageWorkspace: dir, triageFull: full,
			triageBuild: "https://baler.example/job/k3s-tests/job/k3s_tarball/7/",
		}
		triageErr := runTriageBuild(o)
		if triageErr == nil || !strings.Contains(triageErr.Error(), "mower builds only") {
			t.Fatalf("full=%v: a baler build must be refused before triage, got %v", full, triageErr)
		}
	}

	o := &options{matrixPath: matrix, triageWorkspace: dir, triageBuild: "https://mower.example/job/x/1/"}
	triageErr := runTriageBuild(o)
	if triageErr == nil || !strings.Contains(triageErr.Error(), "-triage-manifest") {
		t.Fatalf("a mower build must pass the controller check (and stop at the manifest), got %v", triageErr)
	}
}
