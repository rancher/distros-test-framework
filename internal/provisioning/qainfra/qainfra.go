package qainfra

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rancher/distros-test-framework/internal/resources"
)

// Provision provisions infrastructure using qa-infra remote modules
func (p *QAInfraProvider) Provision(config Config, c *resources.Cluster) (*resources.Cluster, error) {
	cfg := addQAInfraEnv(config, c)

	resources.LogLevel("info", "Starting qa-infra provisioning with workspace: %s", cfg.Workspace)

	// Execute provisioning pipeline
	pipeline := []ProvisioningStep{
		setupDirectories,
		prepareTerraformFiles,
		executeOpenTofuOperations,
		setupAnsibleEnvironment,
		generateInventory,
		ApplySystemBypasses,
		executeAnsiblePlaybook,
	}

	for i, step := range pipeline {
		if err := step(cfg); err != nil {
			return nil, fmt.Errorf("provisioning step %d failed: %w", i+1, err)
		}
	}

	outputs, err := getOpenTofuOutputs(config.NodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster outputs: %w", err)
	}

	ccc := QAInfraClusterConfig(cfg, outputs.FQDN)

	resources.LogLevel("info", "Infrastructure provisioned successfully with qa-infra remote modules")
	resources.LogLevel("info", "Kubeconfig available at: %s", cfg.KubeconfigPath)
	resources.LogLevel("info", "Ansible playbooks downloaded to: %s", cfg.TempDir)

	return ccc, nil
}

// destroyQAInfra destroys qa-infra infrastructure
func destroyQAInfra() (string, error) {
	workspace := os.Getenv("TF_WORKSPACE")
	if workspace == "" {
		resources.LogLevel("warn", "No workspace specified for qa-infra destroy")
		return "", nil
	}

	var rootDir string
	if isRunningInContainer() {
		resources.LogLevel("info", "Detected container environment for qa-infra destroy")
		rootDir = "/go/src/github.com/rancher/distros-test-framework"
	} else {
		_, callerFilePath, _, _ := runtime.Caller(0)
		rootDir = filepath.Join(filepath.Dir(callerFilePath), "..")
	}

	nodeSource := os.Getenv("TERRAFORM_NODE_SOURCE")
	if nodeSource == "" {
		nodeSource = filepath.Join(rootDir, "infrastructure/qa-infra")
	}

	resources.LogLevel("info", "Destroying qa-infra infrastructure...")
	if err := runCmd(nodeSource, "tofu", "workspace", "select", workspace); err != nil {
		return "", fmt.Errorf("tofu workspace select failed: %w", err)
	}

	if err := runCmd(nodeSource, "tofu", "destroy", "-auto-approve"); err != nil {
		return "", fmt.Errorf("tofu destroy failed: %w", err)
	}

	return "cluster destroyed", nil
}
