package qainfra

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func (*Provisioner) provisionInfrastructure(cfg *driver.InfraConfig) (*driver.Cluster, error) {
	resources.LogLevel("info", "Start provisioning with qainfra infrastructure for %s", cfg.Product)

	// Add qainfra env config to start the provisioning process.
	infraCfg, err := addQAInfraEnv(cfg)
	if err != nil {
		return nil, fmt.Errorf("error loading qainfra cluster config: %w", err)
	}

	pipeline := []ProvisioningStep{
		setupDirectories,
		writeRunManifest,
		prepareTerraformFiles,
		executeInfraProvisioner,
		buildClusterConfig,
		setupAnsibleEnvironment,
		executeAnsiblePlaybook,
		addTofuOutputsToConfig,
		collectAirgapFacts,
	}

	for i, step := range pipeline {
		if err := step(infraCfg); err != nil {
			handoffOnFailure(infraCfg)

			return nil, fmt.Errorf("provisioning step %d failed: %w", i+1, err)
		}
	}

	if err := updateManifestStatus(infraCfg.InfraProvisioner.RunDir, runStatusProvisioned, nil); err != nil {
		return nil, err
	}

	resources.LogLevel("info", "Infrastructure provisioned successfully with qainfra remote"+
		" module and Ansible playbooks downloaded to: %s", infraCfg.InfraProvisioner.TempDir)

	return infraCfg.Cluster, nil
}

// handoffOnFailure tells the operator where this run's state is; the caller decides on destroy.
func handoffOnFailure(cfg *driver.InfraConfig) {
	dir := cfg.InfraProvisioner.RunDir
	m, err := readManifest(dir)
	if err != nil {
		resources.LogLevel("warn", "qainfra run %s failed before its manifest was written (%v)",
			cfg.InfraProvisioner.RunID, err)

		return
	}

	logRunHandoff(dir, m)
}

// destroyInfrastructure removes exactly the resources of the current run, located
// through QA_INFRA_RUN_ID and its persisted manifest; never by name pattern.
func (*Provisioner) destroyInfrastructure(product, module string) (string, error) {
	resources.LogLevel("info", "Start destroying qainfra infrastructure for %s with module %s",
		product, module)

	runID := strings.TrimSpace(os.Getenv(runIDEnv))
	if runID == "" {
		return "", errors.New("no " + runIDEnv + " set for qainfra destroy")
	}

	dir := runDir(runStateRoot(), runID)
	m, err := readManifest(dir)
	if err != nil {
		return "", fmt.Errorf("locate run %s: %w", runID, err)
	}

	switch m.Status {
	case runStatusDestroyed:
		resources.LogLevel("info", "run %s already destroyed", runID)

		return "cluster destroyed", nil
	case runStatusCreated:
		if !workspaceStateExists(m) {
			resources.LogLevel("info", "run %s never applied any resources; nothing to destroy", runID)
			m.Status = runStatusDestroyed

			return "cluster destroyed", writeManifest(dir, m)
		}
	}

	m.Status = runStatusDestroying
	if err := writeManifest(dir, m); err != nil {
		return "", err
	}

	// RDS deletion alone can take 10-15 minutes; a short window here kills the
	// destroy mid-flight and orphans the database (seen on extdb jobs).
	args := append([]string{"destroy", "-auto-approve"}, m.ApplyArgs...)
	if err := runCmdWithTimeout(m.TofuDir, 30*time.Minute, "tofu", args...); err != nil {
		m.Status = runStatusDestroyFailed
		_ = writeManifest(dir, m)
		logRunHandoff(dir, m)

		return "", fmt.Errorf("tofu destroy failed for run %s: %w", runID, err)
	}

	m.Status = runStatusDestroyed
	if err := writeManifest(dir, m); err != nil {
		return "", err
	}
	resources.LogLevel("info", "Infrastructure destroyed successfully (run %s)", runID)

	return "cluster destroyed", nil
}

// workspaceStateExists reports whether tofu ever wrote state for the run's workspace.
func workspaceStateExists(m *runManifest) bool {
	_, err := os.Stat(filepath.Join(m.TofuDir, "terraform.tfstate.d", m.Workspace, "terraform.tfstate"))

	return err == nil
}
