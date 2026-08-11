package qainfra

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func (*Provisioner) provisionInfrastructure(cfg *driver.InfraConfig) (*driver.Cluster, error) {
	resources.LogLevel("info", "Start provisioning with qainfra infrastructure for %s", cfg.Product)

	// Add qainfra env config to start the provisioning process.
	infraCfg := addQAInfraEnv(cfg)

	pipeline := []ProvisioningStep{
		setupDirectories,
		prepareTerraformFiles,
		executeInfraProvisioner,
		buildClusterConfig,
		setupAnsibleEnvironment,
		executeAnsiblePlaybook,
		addTofuOutputsToConfig,
	}

	for i, step := range pipeline {
		if err := step(infraCfg); err != nil {
			return nil, fmt.Errorf("provisioning step %d failed: %w", i+1, err)
		}
	}

	resources.LogLevel("info", "Infrastructure provisioned successfully with qainfra remote"+
		" module and Ansible playbooks downloaded to: %s", infraCfg.InfraProvisioner.TempDir)

	return infraCfg.Cluster, nil
}

func (*Provisioner) destroyInfrastructure(product, module string) (string, error) {
	resources.LogLevel("info", "Start destroying qainfra infrastructure for %s with module %s",
		product, module)

	workspace := os.Getenv("TF_WORKSPACE")
	if workspace == "" {
		resources.LogLevel("warn", "No workspace specified for qainfra destroy")
		return "", errors.New("no workspace specified for qainfra destroy")
	}

	nodeSource := tofuWorkdir(workspace)
	// Destroy must receive the same nodes override apply used (split-role blobs
	// carry placeholder topologies that fail the module's cp-count validation).
	args, err := appendNodesVar([]string{"destroy", "-auto-approve", "-var-file=vars.tfvars"})
	if err != nil {
		return "", fmt.Errorf("build nodes topology for destroy: %w", err)
	}
	// RDS deletion alone can take 10-15 minutes; a short window here kills the
	// destroy mid-flight and orphans the database (seen on extdb jobs).
	if err := runCmdWithTimeout(nodeSource, 30*time.Minute, "tofu", args...); err != nil {
		return "", fmt.Errorf("tofu destroy failed: %w", err)
	}

	resources.LogLevel("info", "Infrastructure destroyed successfully")

	return "cluster destroyed", nil
}
