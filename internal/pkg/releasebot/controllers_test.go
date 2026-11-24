package releasebot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only the controllers the jobs use need credentials: a mower-only run must not ask for a Baler
// token, while a run with a Baler job without one must stop before anything starts.
func TestBuildersForNeedsOnlyUsedControllers(t *testing.T) {
	m := &Matrix{Controller: map[string]Limits{"mower": {}, "baler": {}}}
	clients := map[string]*jenkins{"mower": NewJenkins("https://mower.example", "u", "t")}

	mowerOnly := []JenkinsJob{{Name: "a", Controller: "mower"}}
	builders, err := buildersFor(m, clients, mowerOnly)
	if err != nil || builders["mower"] == nil || len(builders) != 1 {
		t.Fatalf("mower-only run: builders %v, err %v", builders, err)
	}

	withBaler := []JenkinsJob{mowerOnly[0], {Name: "b", Controller: "baler"}}
	if _, err = buildersFor(m, clients, withBaler); err == nil || !strings.Contains(err.Error(), "JENKINS_BALER_AUTH") {
		t.Fatalf("baler job without credentials: expected JENKINS_BALER_AUTH error, got %v", err)
	}
}

// JENKINS_<NAME>_URL replaces the matrix URL for plans and build lookups; anything but a plain
// https origin is refused, since the URL is logged and receives the credentials.
func TestLoadMatrixAppliesControllerURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.yaml")
	body := "controllers:\n  mower: {url: https://mower.example, maxConcurrent: 1}\njobs:\n" +
		"  - {name: a, product: k3s, controller: mower, path: p, code: a}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := func(string) (string, string, bool) { return "u", "t", true }

	override := func(string) string { return "https://other.example" }
	app := New(&Config{MatrixPath: path, JenkinsAuth: auth, ControllerURL: override})
	m, err := app.loadMatrix()
	if err != nil || m.Controller["mower"].URL != "https://other.example" {
		t.Fatalf("override not applied: %+v, err %v", m, err)
	}
	if _, lookupErr := app.JenkinsForBuild("https://other.example/job/p/1/"); lookupErr != nil {
		t.Fatalf("build on the overriding URL not served: %v", lookupErr)
	}

	for _, bad := range []string{"http://other.example", "https://u:p@other.example", "https://x?y=1", "other"} {
		app = New(&Config{MatrixPath: path, ControllerURL: func(string) string { return bad }})
		if _, err = app.loadMatrix(); err == nil || !strings.Contains(err.Error(), "JENKINS_MOWER_URL") {
			t.Fatalf("%q: expected a JENKINS_MOWER_URL error, got %v", bad, err)
		}
	}
}
