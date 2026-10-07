package releasebot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The listener's planner re-reads the matrix per request; without jenkins credentials the jobs
// cannot be checked, so the plan is refused, and it can be rebuilt without some tags.
func TestPlannerRefusesUncheckedPlan(t *testing.T) {
	matrix := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(matrix, []byte(`
controllers:
  mower: {url: https://mower.example, maxConcurrent: 1}
defaultParams:
  INSTALL_VERSION: "{{VERSION}}"
jobs:
  - {name: smoke, product: rke2, controller: mower, path: p/smoke, code: vc}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := New(&Config{MatrixPath: matrix, SkipTagCheck: true}).Planner(nil, nil)

	p, err := plan(context.Background(), "please test v1.37.1-rc1+rke2r1 v1.36.5-rc1+rke2r1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Refusal == "" || !strings.Contains(p.Summary, "Release plan, request") || len(p.Tags) != 2 {
		t.Fatalf("planned: refusal %q tags %v summary %q", p.Refusal, p.Tags, p.Summary)
	}
	rest, err := p.Without(context.Background(), []string{"v1.36.5-rc1+rke2r1"})
	if err != nil || len(rest.Tags) != 1 || rest.Tags[0] != "v1.37.1-rc1+rke2r1" {
		t.Fatalf("without: %v %v", rest, err)
	}
}
