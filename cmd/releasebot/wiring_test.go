package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
)

func TestWiringAppliesDTFRefEnvironment(t *testing.T) {
	t.Setenv(releasebot.DTFRefEnv, "env-ref")
	path := filepath.Join(t.TempDir(), "matrix.yaml")
	raw := []byte("dtfRef: matrix-ref\ndefaultParams:\n  BRANCH: '{{DTF_REF}}'\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	app := newApp(&options{matrixPath: path, skipTagCheck: true})
	p, err := app.Prepare(t.Context(), releasebot.Request{RKE2: []string{"v1.37.1-rc2+rke2r1"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Matrix.DTFRef != "env-ref" || p.Matrix.Defaults["BRANCH"] != "env-ref" {
		t.Fatalf("override not applied: %+v", p.Matrix)
	}
}
