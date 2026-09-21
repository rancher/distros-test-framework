package qainfra

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/resources"
)

// Run identity and persisted state. Every provisioning run owns exactly one
// directory under the state root; cleanup only ever targets that directory.
const (
	runIDEnv    = "QA_INFRA_RUN_ID"
	stateDirEnv = "QA_INFRA_STATE_DIR"

	defaultStateRoot = "/tmp/qainfra-runs"
	runIDMaxLen      = 60
	runManifestName  = "run.json"
	runDestroyScript = "destroy.sh"
	manifestVersion  = 1

	runStatusCreated       = "created"
	runStatusApplying      = "applying"
	runStatusApplied       = "applied"
	runStatusProvisioned   = "provisioned"
	runStatusDestroying    = "destroying"
	runStatusDestroyed     = "destroyed"
	runStatusDestroyFailed = "destroy_failed"
)

var runIDInvalidRE = regexp.MustCompile(`[^a-z0-9-]+`)

// runManifest is the durable record a run leaves behind so an independent
// process (Jenkins `finally`, an operator) can destroy exactly this run.
type runManifest struct {
	SchemaVersion  int      `json:"schema_version"`
	RunID          string   `json:"run_id"`
	Workspace      string   `json:"workspace"`
	Product        string   `json:"product"`
	Module         string   `json:"module"`
	ResourceName   string   `json:"resource_name"`
	HostnamePrefix string   `json:"hostname_prefix,omitempty"`
	Region         string   `json:"region,omitempty"`
	QAInfraRepo    string   `json:"qa_infra_repo"`
	QAInfraRef     string   `json:"qa_infra_ref"`
	QAInfraSHA     string   `json:"qa_infra_sha"`
	TofuDir        string   `json:"tofu_dir"`
	AnsibleDir     string   `json:"ansible_dir"`
	ApplyArgs      []string `json:"apply_args,omitempty"`
	DestroyPolicy  bool     `json:"destroy_policy"`
	Status         string   `json:"status"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// resolveRunID returns the run identity: QA_INFRA_RUN_ID when the caller (Jenkins)
// minted one, otherwise a local id that is still unique per process.
func resolveRunID(resourceName string) (string, error) {
	if raw := strings.TrimSpace(os.Getenv(runIDEnv)); raw != "" {
		id, err := sanitizeRunID(raw)
		if err != nil {
			return "", fmt.Errorf("%s: %w", runIDEnv, err)
		}
		// Normalizing the charset is idempotent with Jenkins; truncating is not and
		// would break the caller's cleanup path.
		if len(strings.Trim(runIDInvalidRE.ReplaceAllString(strings.ToLower(raw), "-"), "-")) > runIDMaxLen {
			return "", fmt.Errorf("%s=%q is longer than %d chars; the caller keys cleanup on this exact value",
				runIDEnv, raw, runIDMaxLen)
		}

		return id, nil
	}

	base := strings.TrimSpace(resourceName)
	if base == "" {
		base = "local"
	}
	id, err := sanitizeRunID(fmt.Sprintf("%s-%s-%s", base, time.Now().UTC().Format("20060102t150405"), randomSuffix(5)))
	if err != nil {
		return "", err
	}

	return id, nil
}

// sanitizeRunID lowercases and restricts to [a-z0-9-] so the id is safe as a
// tofu workspace, a directory name and an AWS tag value.
func sanitizeRunID(raw string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(raw))
	id = runIDInvalidRE.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")
	if len(id) > runIDMaxLen {
		id = strings.Trim(id[:runIDMaxLen], "-")
	}
	if id == "" {
		return "", fmt.Errorf("run id %q has no usable characters", raw)
	}

	return id, nil
}

// uniqueIDFromRunID derives the 5-char suffix used in AWS names from the run id,
// so a retry of the same run reuses the same resource names.
func uniqueIDFromRunID(runID string) string {
	sum := sha256.Sum256([]byte(runID))

	return hex.EncodeToString(sum[:])[:5]
}

// runStateRoot is where run directories live; Jenkins mounts a workspace path
// here so state survives the test container.
func runStateRoot() string {
	if dir := strings.TrimSpace(os.Getenv(stateDirEnv)); dir != "" {
		return dir
	}

	return defaultStateRoot
}

func runDir(root, runID string) string { return filepath.Join(root, runID) }

func manifestPath(dir string) string { return filepath.Join(dir, runManifestName) }

func destroyScriptPath(dir string) string { return filepath.Join(dir, runDestroyScript) }

func writeManifest(dir string, m *runManifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create run dir %s: %w", dir, err)
	}
	m.SchemaVersion = manifestVersion
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if m.CreatedAt == "" {
		m.CreatedAt = m.UpdatedAt
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run manifest: %w", err)
	}
	tmp := manifestPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write run manifest: %w", err)
	}

	return os.Rename(tmp, manifestPath(dir))
}

func readManifest(dir string) (*runManifest, error) {
	data, err := os.ReadFile(manifestPath(dir))
	if err != nil {
		return nil, fmt.Errorf("read run manifest: %w", err)
	}
	var m runManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse run manifest %s: %w", manifestPath(dir), err)
	}
	if m.SchemaVersion != manifestVersion {
		return nil, fmt.Errorf("run manifest %s has schema_version %d, want %d",
			manifestPath(dir), m.SchemaVersion, manifestVersion)
	}
	if m.RunID == "" || m.TofuDir == "" || m.Workspace == "" {
		return nil, errors.New("run manifest is missing run_id, workspace or tofu_dir")
	}

	return &m, nil
}

// updateManifestStatus rewrites only the status (and apply args when given).
func updateManifestStatus(dir, status string, applyArgs []string) error {
	m, err := readManifest(dir)
	if err != nil {
		return err
	}
	m.Status = status
	if applyArgs != nil {
		m.ApplyArgs = applyArgs
	}

	return writeManifest(dir, m)
}

// writeDestroyScript renders a self-contained cleanup script next to the manifest.
// It re-runs the exact apply arguments, honors destroy_policy unless FORCE=1 and
// is idempotent on an already destroyed run.
func writeDestroyScript(dir string, m *runManifest) error {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n# Generated by distros-test-framework; destroys ONLY run ")
	b.WriteString(m.RunID)
	b.WriteString("\nset -eu\n")
	b.WriteString("RUN_DIR=\"$(cd \"$(dirname \"$0\")\" && pwd -P)\"\n")
	b.WriteString("MANIFEST=\"$RUN_DIR/" + runManifestName + "\"\n")
	// The playbook's credentials file must not outlive an aborted run, whatever happens below.
	b.WriteString("rm -f \"$RUN_DIR/" + airgapSecretsName + "\"\n")
	b.WriteString("if grep -q '\"status\": \"" + runStatusDestroyed + "\"' \"$MANIFEST\"; then\n")
	b.WriteString("  echo \"run " + m.RunID + " already destroyed\"; exit 0\nfi\n")
	b.WriteString("if [ \"${FORCE:-0}\" != \"1\" ] && ! grep -q '\"destroy_policy\": true' \"$MANIFEST\"; then\n")
	b.WriteString("  echo \"run " + m.RunID + " was kept on purpose (destroy=false); ")
	b.WriteString("rerun with FORCE=1 to remove\"; exit 0\nfi\n")
	b.WriteString("cd " + shellQuote(m.TofuDir) + "\n")
	b.WriteString("tofu workspace select " + shellQuote(m.Workspace) + "\n")
	b.WriteString("tofu destroy -auto-approve")
	for _, a := range m.ApplyArgs {
		b.WriteString(" " + shellQuote(a))
	}
	b.WriteString("\n")
	b.WriteString("sed -i.bak 's/\"status\": \"[a-z_]*\"/\"status\": \"" + runStatusDestroyed + "\"/' \"$MANIFEST\"")
	b.WriteString(" && rm -f \"$MANIFEST.bak\"\n")
	b.WriteString("echo \"run " + m.RunID + " destroyed\"\n")

	if err := os.WriteFile(destroyScriptPath(dir), []byte(b.String()), 0o755); err != nil {
		return fmt.Errorf("write destroy script: %w", err)
	}

	return nil
}

// shellQuote single-quotes a value for POSIX sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// logRunHandoff prints what an operator needs to recover or clean a run by hand.
func logRunHandoff(dir string, m *runManifest) {
	resources.LogLevel("info", "qainfra run %s: state in %s (status=%s, destroy_policy=%t); cleanup: sh %s",
		m.RunID, dir, m.Status, m.DestroyPolicy, destroyScriptPath(dir))
}
